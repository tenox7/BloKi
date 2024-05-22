package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

const adminPrefix = "admin:"

var bgf = map[bool]string{false: "#FFFFFF", true: "#E0E0E0"}

type AdminTemplate struct {
	SiteName  string
	AdminTab  string
	ActiveTab string
	AdminUrl  string
	UserName  string
	CharSet   string
}

type post struct{}
type media struct{}
type creds struct{}
type users struct{}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	var err error
	r.ParseMultipartForm(10 << 20)
	c := creds{}
	user, ok := c.user(w, r)
	if !ok {
		return
	}
	log.Printf("admin user=%q from=%q uri=%q url=%q", user, r.RemoteAddr, r.RequestURI, r.URL.Path)

	adm := AdminTemplate{
		SiteName: *siteName,
		AdminUrl: *adminUri,
		UserName: user,
		CharSet:  charset[strings.HasPrefix(r.UserAgent(), "Mozilla/5")],
	}

	switch r.FormValue("tab") {
	case "posts", "":
		m := post{}
		adm.ActiveTab = "posts"
		switch {
		case r.FormValue("edit") != "":
			adm.AdminTab, err = m.edit(r.FormValue("filename"))
		case r.FormValue("rename") != "":
			adm.AdminTab, err = m.rename(r.FormValue("filename"), r.FormValue("rename"))
		case r.FormValue("delete") == "true":
			adm.AdminTab, err = m.delete(r.FormValue("filename"))
		case r.FormValue("newpost") != "":
			adm.AdminTab, err = m.new(r.FormValue("newpost"), user)
		case r.FormValue("save") != "":
			adm.AdminTab, err = m.save(r.FormValue("filename"), r.FormValue("textdata"))
		case r.FormValue("search") != "":
			adm.AdminTab, err = m.list(r.FormValue("query"))
		default:
			adm.AdminTab, err = m.list("")
		}
	case "media":
		m := media{}
		adm.ActiveTab = "media"
		switch {
		case r.FormValue("rename") != "":
			adm.AdminTab, err = m.rename(r.FormValue("filename"), r.FormValue("rename"))
		case r.FormValue("delete") == "true":
			adm.AdminTab, err = m.delete(r.FormValue("filename"))
		case r.FormValue("upload") != "":
			adm.AdminTab, err = m.upload(r)
		default:
			adm.AdminTab, err = m.list()
		}
	case "users":
		m := users{}
		adm.ActiveTab = "users"
		switch {
		default:
			adm.AdminTab, err = m.list()
		}
	default:
		adm.AdminTab = "<H1>Not Implemented</H1><P>"
	}
	if err != nil {
		log.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates["admin"].Execute(w, adm)
}

func (m post) new(file, user string) (string, error) {
	file = unescapeOrEmpty(file)
	if file == "" || file == "null" {
		return m.list("")
	}
	file = path.Base(file)
	if !strings.HasSuffix(file, ".md") {
		file = file + ".md"
	}
	_, err := os.Stat(path.Join(*rootDir, *postsDir, file))
	if err == nil {
		return "", fmt.Errorf("new post file %q already exists", file)
	}
	_, err = m.save(file,
		"<!--not-published=\""+time.Now().Format(timeFormat)+"\"-->\n"+
			"<!--author=\""+user+"\"-->\n\n# New Post!\n\nHello world!\n\n")
	if err != nil {
		log.Printf("Unable to save post %q: %v", file, err)
		return "", err
	}
	log.Printf("Created new post %q", file)
	return m.edit(file)
}

func (m post) delete(file string) (string, error) {
	if file == "" {
		return m.list("")
	}
	file = path.Base(unescapeOrEmpty(file))
	err := os.Remove(path.Join(*rootDir, *postsDir, file))
	if err != nil {
		log.Printf("Unable to delete post %q: %v", file, err)
		return "", err
	}
	idx.delete(file)
	txt.delete(file)
	log.Printf("Deleted post %q", file)
	return m.list("")
}

func (m post) rename(old, new string) (string, error) {
	if old == "" || new == "" {
		return m.list("")
	}
	old = path.Base(unescapeOrEmpty(old))
	new = path.Base(unescapeOrEmpty(new))
	if !strings.HasSuffix(new, ".md") {
		new = new + ".md"
	}
	err := os.Rename(
		path.Join(*rootDir, *postsDir, old),
		path.Join(*rootDir, *postsDir, new),
	)
	if err != nil {
		log.Printf("Unable to rename post from %q to %q: %v", old, new, err)
		return "", err
	}
	idx.rename(old, new)
	txt.rename(old, new)
	log.Printf("Renamed post %v to %v", old, new)
	return m.list("")
}

// perhaps we should have update in place, save and reopen
func (m post) edit(file string) (string, error) {
	if file == "" {
		return m.list("")
	}
	data, err := m.load(file)
	if err != nil {
		return "", errors.New("Unable to open " + file)
	}
	buf := strings.Builder{}
	buf.WriteString("<H1>Editing - " + html.EscapeString(file) +
		"</H1>\n" +
		"<TEXTAREA NAME=\"textdata\" SPELLCHECK=\"true\" COLS=\"80\" ROWS=\"24\" WRAP=\"soft\" STYLE=\"width: 99%; height: 99%;\">\n" +
		data + "</TEXTAREA><P>\n" +
		"<INPUT TYPE=\"SUBMIT\" NAME=\"save\" VALUE=\"Save\"> <INPUT TYPE=\"SUBMIT\" NAME=\"cancel\" VALUE=\"Cancel\"><P>\n" +
		"<INPUT TYPE=\"HIDDEN\" NAME=\"filename\" VALUE=\"" + html.EscapeString(file) + "\">\n" +
		"<INPUT TYPE=\"HIDDEN\" NAME=\"tab\" VALUE=\"posts\">\n",
	)
	return buf.String(), nil
}

// TODO: I think that edit should be default action on a post and view could be in a secondary column in the table?
// or better no view rather preview from inside the post
func (post) list(query string) (string, error) {
	buf := strings.Builder{}
	buf.WriteString(`<H1>Posts</H1>
		<INPUT TYPE="HIDDEN" NAME="tab" VALUE="posts">
		<INPUT TYPE="TEXT" NAME="query">
		<INPUT TYPE="SUBMIT" NAME="search" VALUE="Search">
		<INPUT TYPE="SUBMIT" NAME="newpost" VALUE="New Post" ONCLICK="this.value=prompt('Name the new post:', 'new-post.md');">
		<INPUT TYPE="SUBMIT" NAME="edit" VALUE="Edit">
		<INPUT TYPE="SUBMIT" NAME="rename" VALUE="Rename" ONCLICK="this.value=prompt('Enter new name:', '');">
		<INPUT TYPE="SUBMIT" NAME="delete" VALUE="Delete" ONCLICK="this.value=confirm('Are you sure you want to delete this post?');">
		<P>
		<TABLE WIDTH="100%" BGCOLOR="#FFFFFF" CELLPADDING="10" CELLSPACING="0" BORDER="0">
		<TR ALIGN="LEFT"><TH>&nbsp;&nbsp;Article</TH><TH>&nbsp;</TH><TH>Author</TH><TH>&darr;Published</TH><TH>Modified</TH></TR>
	`)

	posts := []string{}
	if query != "" {
		posts = txt.search(query)
	}

	idx.RLock()
	defer idx.RUnlock()

	if len(posts) == 0 && query == "" {
		for a := range idx.metaData {
			posts = append(posts, a)
		}
		sort.SliceStable(posts, func(i, j int) bool {
			if idx.metaData[posts[i]].published.IsZero() && idx.metaData[posts[j]].published.IsZero() {
				return idx.metaData[posts[j]].modified.Before(idx.metaData[posts[i]].modified)
			}
			if idx.metaData[posts[i]].published.IsZero() {
				return true
			} else if idx.metaData[posts[j]].published.IsZero() {
				return false
			}
			return idx.metaData[posts[j]].published.Before(idx.metaData[posts[i]].published)
		})
	}

	i := 0
	for _, a := range posts {
		p := idx.metaData[a].published.Format(timeFormat)
		if idx.metaData[a].published.IsZero() {
			p = "draft"
		}
		buf.WriteString("<TR BGCOLOR=\"" + bgf[i%2 == 0] + "\">" +
			"<TD><INPUT TYPE=\"radio\" NAME=\"filename\" VALUE=\"" + a + "\">&nbsp;" +
			"<A HREF=\"/" + url.QueryEscape(idx.metaData[a].url) + "\" TARGET=\"_blank\">" + html.EscapeString(a) + "</A></TD>" +
			"<TD><A HREF=\"" + *adminUri + "/?tab=posts&edit=this&filename=" + url.QueryEscape(a) + "\">[Edit]</A></TD>" +
			"<TD>" + idx.metaData[a].author + "</TD>" +
			"<TD>" + p + "</TD>" +
			"<TD>" + idx.metaData[a].modified.Format(timeFormat) + "</TD></TR>\n")
		i++
	}

	buf.WriteString("</TABLE>\n")
	return buf.String(), nil
}

func (m post) save(file, postText string) (string, error) {
	file = unescapeOrEmpty(file)
	if file == "" {
		return m.list("")
	}
	fullFilename := path.Join(*rootDir, *postsDir, path.Base(file))
	log.Printf("Saving %q", fullFilename)
	err := os.WriteFile(fullFilename+".tmp", []byte(postText), 0644)
	if err != nil {
		return "", errors.New("unable to write temp file for %q: " + err.Error())
	}
	st, err := os.Stat(fullFilename + ".tmp")
	if err != nil {
		return "", errors.New("unable to stat temp file for %q: " + err.Error())
	}
	if st.Size() != int64(len(postText)) {
		return "", errors.New("temp file size != input size")
	}
	err = os.Rename(fullFilename+".tmp", fullFilename)
	if err != nil {
		return "", errors.New("unable to rename temp file to the target file: " + err.Error())
	}
	log.Printf("Saved post %q", file)
	idx.update(file)
	txt.update(file)
	return m.list("")
}

func (post) load(file string) (string, error) {
	f, err := os.ReadFile(path.Join(*rootDir, *postsDir, path.Base(unescapeOrEmpty(file))))
	if err != nil {
		return "", errors.New("unable to read " + file + " : " + err.Error())
	}
	return html.EscapeString(string(f)), nil
}

func (m media) upload(r *http.Request) (string, error) {
	i, h, err := r.FormFile("fileup")
	if err != nil {
		log.Printf("Unable to upload file %v", err)
		return "", err
	}
	if h.Filename == "" {
		return m.list()
	}
	o, err := os.OpenFile(path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(h.Filename))), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Unable to upload file %q: %v", h.Filename, err)
		return "", err
	}
	defer o.Close()
	oSz, err := io.Copy(o, i)
	if err != nil {
		log.Printf("Unable to upload file %q: %v", h.Filename, err)
		return "", err
	}
	if oSz != h.Size {
		log.Printf("Unable to upload file %q: %v", h.Filename, err)
		return "", err
	}
	log.Printf("Uploaded file %q, size: %v", h.Filename, h.Size)
	return m.list()
}

func (m media) rename(old, new string) (string, error) {
	if old == "" || new == "" {
		return m.list()
	}
	err := os.Rename(
		path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(old))),
		path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(new))),
	)
	if err != nil {
		log.Printf("Unable to rename media from %q to %q: %v", old, new, err)
		return "", err
	}
	log.Printf("Renamed media %q to %q", old, new)
	return m.list()
}

func (m media) delete(file string) (string, error) {
	if file == "" {
		return m.list()
	}
	err := os.Remove(path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(file))))
	if err != nil {
		log.Printf("Unable to delete media %q: %v", file, err)
		return "", err
	}
	log.Printf("Deleted media %q", file)
	return m.list()
}

func (media) list() (string, error) {
	buf := strings.Builder{}
	buf.WriteString(`<H1>Media</H1>
	<INPUT TYPE="HIDDEN" NAME="tab" VALUE="media">
	<INPUT TYPE="SUBMIT" NAME="rename" VALUE="Rename" ONCLICK="this.value=prompt('Enter new name:', '');">
	<INPUT TYPE="SUBMIT" NAME="delete" VALUE="Delete" ONCLICK="this.value=confirm('Are you sure you want to delete this image?');">
	<INPUT TYPE="FILE" NAME="fileup">
	<INPUT TYPE="SUBMIT" NAME="upload" VALUE="Upload">

	<TABLE BORDER="0" CELLSPACING="10"><TR>
	`)
	m, err := os.ReadDir(path.Join(*rootDir, *mediaDir))
	if err != nil {
		return "", err
	}
	sort.Slice(m, func(i, j int) bool {
		return m[i].Name() < m[j].Name()
	})
	for x, i := range m {
		if i.IsDir() || strings.HasPrefix(i.Name(), ".") ||
			!(strings.HasSuffix(i.Name(), ".jpg") ||
				strings.HasSuffix(i.Name(), ".png") ||
				strings.HasSuffix(i.Name(), ".gif")) {
			continue
		}
		if x%5 == 0 {
			buf.WriteString("</TR><TR>")
		}
		nm := html.EscapeString(i.Name())
		un := url.QueryEscape(i.Name())
		buf.WriteString(`
			<TD BGCOLOR="#D0D0D0" ALIGN="center" VALIGN="bottom">
			<A HREF="/media/` + un + `">
			<IMG SRC="/media/` + un + `" BORDER="0" TITLE="` + nm + `" ALT="` + nm + `" WIDTH="150"></A><BR>
			<INPUT TYPE="radio" NAME="filename" VALUE="` + un + `">
			<A HREF="/media/` + un + `">` + nm + `</A></TD>
		`)
	}
	buf.WriteString("</TR></TABLE>\n")
	return buf.String(), nil
}

func (users) list() (string, error) {
	buf := strings.Builder{}
	buf.WriteString(`<H1>Users</H1>
	<INPUT TYPE="HIDDEN" NAME="tab" VALUE="users">
	<INPUT TYPE="SUBMIT" NAME="newuser" VALUE="New User" ONCLICK="this.value=prompt('Name the new user:', '');">
	<INPUT TYPE="SUBMIT" NAME="passwd" VALUE="Reset Password" ONCLICK="this.value=prompt('Enter new password:', '');">
	<INPUT TYPE="SUBMIT" NAME="delete" VALUE="Delete" ONCLICK="this.value=confirm('Are you sure you want to delete this user?');">
	<P>
	<TABLE WIDTH="100%" BGCOLOR="#FFFFFF" CELLPADDING="10" CELLSPACING="0" BORDER="0">
	<TR ALIGN="LEFT"><TH>&nbsp;&nbsp;Username</TH><TH>Type</TH></TR>
	`)
	for i, u := range secretsStore.Keys() {
		if !strings.HasPrefix(u, adminPrefix) {
			continue
		}
		u = strings.Split(u, ":")[1]
		buf.WriteString("<TR BGCOLOR=\"" + bgf[i%2 == 0] + "\">" +
			"<TD><INPUT TYPE=\"radio\" NAME=\"username\" VALUE=\"" + u + "\">&nbsp;" + html.EscapeString(u) + "</TD>" +
			"<TD>admin</TD></TR>\n")
	}
	buf.WriteString("</TR></TABLE>\n")
	return buf.String(), nil
}

func (c creds) user(w http.ResponseWriter, r *http.Request) (string, bool) {
	if *secrets == "" || secretsStore == nil {
		http.Error(w, "unable to get user db", http.StatusUnauthorized)
		return "", false
	}
	u, p, ok := r.BasicAuth()
	if ok && c.auth(u, p) {
		return u, true
	}
	log.Printf("Unauthorized %q from %q", u, r.RemoteAddr)
	w.Header().Set("WWW-Authenticate", "Basic realm=\"BloKi "+*siteName+"\"")
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return "", false
}

func (creds) auth(user, pass string) bool {
	jpwd, err := secretsStore.Get(context.TODO(), adminPrefix+user)
	if err != nil {
		return false
	}
	spwd := struct{ Salt, Hash string }{}
	err = json.Unmarshal(jpwd, &spwd)
	if err != nil {
		return false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(spwd.Salt+pass)))
	return subtle.ConstantTimeCompare([]byte(hash), []byte(spwd.Hash)) == 1
}

func (creds) set(user, pass string) error {
	if *secrets == "" || secretsStore == nil {
		return errors.New("unable to access secret store")
	}
	buff := make([]byte, 8)
	_, err := rand.Read(buff)
	if err != nil {
		return err
	}
	salt := base64.StdEncoding.EncodeToString(buff)[:8]
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(salt+pass)))
	spwd, err := json.Marshal(struct{ Salt, Hash string }{Salt: salt, Hash: hash})
	if err != nil {
		return err
	}
	return secretsStore.Put(context.TODO(), adminPrefix+user, spwd)
}

func (creds) del(user string) error {
	if *secrets == "" || secretsStore == nil {
		return errors.New("unable to open user db")
	}
	return secretsStore.Delete(context.TODO(), adminPrefix+user)
}

func cliUserManager() {
	if secretsStore == nil {
		log.Fatal("The secrets file must be specified")
	}
	c := creds{}
	switch flag.Arg(1) {
	case "passwd":
		if flag.Arg(2) == "" {
			log.Fatal("usage: bloki user passwd <username>")
		}
		fmt.Print("New Password: ")
		pwd, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			log.Fatal(err)
		}
		err = c.set(flag.Arg(2), string(pwd))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("")
	case "delete":
		if flag.Arg(2) == "" {
			log.Fatal("usage: bloki user delete <username>")
		}
		err := c.del(flag.Arg(2))
		if err != nil {
			log.Fatal(err)
		}
	case "list":
		for _, u := range secretsStore.Keys() {
			if !strings.HasPrefix(u, adminPrefix) {
				continue
			}
			fmt.Println(strings.Split(u, ":")[1])
		}
	default:
		fmt.Println("usage: bloki user <passwd|delete|list> [username]")
	}
}

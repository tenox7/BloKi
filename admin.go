package main

import (
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
)

var bgf = map[bool]string{false: "#FFFFFF", true: "#E0E0E0"}

type AdminTemplate struct {
	SiteName  string
	AdminTab  string
	ActiveTab string
	AdminUrl  string
	User      string
	CharSet   string
}

type post struct{}
type media struct{}
type creds struct{}

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
		User:     user,
		CharSet:  charset[strings.HasPrefix(r.UserAgent(), "Mozilla/5")],
	}

out:
	switch r.FormValue("tab") {
	case "media":
		m := media{}
		adm.ActiveTab = "media"
		switch {
		case r.FormValue("rename") != "" && r.FormValue("filename") != "":
			adm.AdminTab, err = m.rename(r.FormValue("filename"), r.FormValue("rename"))
		case r.FormValue("delete") == "true" && r.FormValue("filename") != "":
			adm.AdminTab, err = m.delete(r.FormValue("filename"))
		case r.FormValue("upload") != "":
			adm.AdminTab, err = m.upload(r)
		default:
			adm.AdminTab, err = m.list()
		}
	default:
		m := post{}
		adm.ActiveTab = "posts"
		switch {
		case r.FormValue("edit") != "" && r.FormValue("filename") != "":
			adm.AdminTab, err = m.edit(r.FormValue("filename"))
		case r.FormValue("rename") != "" && r.FormValue("filename") != "":
			adm.AdminTab, err = m.rename(r.FormValue("filename"), r.FormValue("rename"))
		case r.FormValue("delete") == "true" && r.FormValue("filename") != "":
			adm.AdminTab, err = m.delete(r.FormValue("filename"))
		case r.FormValue("newpost") != "" && r.FormValue("newpost") != "null":
			adm.AdminTab, err = m.new(r.FormValue("newpost"), user)
		case r.FormValue("save") != "" && r.FormValue("filename") != "":
			// TODO: convert to return m.list() ??
			err = m.save(r.FormValue("filename"), r.FormValue("textdata"))
			if err != nil {
				log.Printf("Unable to save post %q: %v", r.FormValue("filename"), err)
				break out
			}
			log.Printf("Saved post %q", r.FormValue("filename"))
			// TODO: idx update single article
			idx.indexArticles()
			adm.AdminTab, err = m.list()

		default:
			adm.AdminTab, err = m.list()
		}
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
	file = path.Base(unescapeOrEmpty(file))
	if !strings.HasSuffix(file, ".md") {
		file = file + ".md"
	}
	_, err := os.Stat(path.Join(*rootDir, *postsDir, file))
	if err == nil {
		return "", fmt.Errorf("new post file %q already exists", file)
	}
	err = m.save(file,
		"[//]: # (not-published="+time.Now().Format(timeFormat)+")\n[//]: # (author="+user+")\n\n# New Post!\n\nHello world!\n\n")
	if err != nil {
		log.Printf("Unable to save post %q: %v", file, err)
		return "", err
	}
	log.Printf("Created new post %q", file)
	// TODO: idx update single article
	idx.indexArticles()
	return m.edit(file)
}

func (m post) delete(file string) (string, error) {
	file = path.Base(unescapeOrEmpty(file))
	err := os.Remove(path.Join(*rootDir, *postsDir, file))
	if err != nil {
		log.Printf("Unable to delete post %q: %v", file, err)
		return "", err
	}
	idx.deletePost(file)
	log.Printf("Deleted post %q", file)
	return m.list()
}

func (m post) rename(old, new string) (string, error) {
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
	idx.renamePost(old, new)
	log.Printf("Renamed post %v to %v", old, new)
	return m.list()
}

// perhaps we should have update in place, save and reopen
func (m post) edit(fn string) (string, error) {
	data, err := m.load(fn)
	if err != nil {
		return "", errors.New("Unable to open " + fn)
	}
	buf := strings.Builder{}
	buf.WriteString("<H1>Editing - " + html.EscapeString(fn) +
		"</H1>\n" +
		"<TEXTAREA NAME=\"textdata\" SPELLCHECK=\"true\" COLS=\"80\" ROWS=\"24\" WRAP=\"soft\" STYLE=\"width: 99%; height: 99%;\">\n" +
		data + "</TEXTAREA><P>\n" +
		"<INPUT TYPE=\"SUBMIT\" NAME=\"save\" VALUE=\"Save\"> <INPUT TYPE=\"SUBMIT\" NAME=\"cancel\" VALUE=\"Cancel\"><P>\n" +
		"<INPUT TYPE=\"HIDDEN\" NAME=\"filename\" VALUE=\"" + html.EscapeString(fn) + "\">\n",
	)
	return buf.String(), nil
}

// TODO: I think that edit should be default action on a post and view could be in a secondary column in the table?
// or better no view rather preview from inside the post
func (post) list() (string, error) {
	buf := strings.Builder{}
	buf.WriteString(`<H1>Posts</H1>
		<INPUT TYPE="HIDDEN" NAME="tab" VALUE="posts">
		<INPUT TYPE="SUBMIT" NAME="newpost" VALUE="New" ONCLICK="this.value=prompt('Name the new post:', 'new-post.md');">
		<INPUT TYPE="SUBMIT" NAME="edit" VALUE="Edit">
		<INPUT TYPE="SUBMIT" NAME="rename" VALUE="Rename" ONCLICK="this.value=prompt('Enter new name:', '');">
		<INPUT TYPE="SUBMIT" NAME="delete" VALUE="Delete" ONCLICK="this.value=confirm('Are you sure you want to delete this post?');"><P>
		<TABLE WIDTH="100%" BGCOLOR="#FFFFFF" CELLPADDING="10" CELLSPACING="0" BORDER="0">
		<TR ALIGN="LEFT"><TH>&nbsp;&nbsp;Article</TH><TH>Author</TH><TH>&darr;Published</TH><TH>Modified</TH></TR>
	`)

	idx.RLock()
	defer idx.RUnlock()
	srt := []string{}
	for a := range idx.metaData {
		srt = append(srt, a)
	}
	// TODO: also sort unpublished by mod time
	sort.SliceStable(srt, func(i, j int) bool {
		if idx.metaData[srt[i]].published.Equal(time.Unix(0, 0)) {
			return true
		} else if idx.metaData[srt[j]].published.Equal(time.Unix(0, 0)) {
			return false
		}
		return idx.metaData[srt[j]].published.Before(idx.metaData[srt[i]].published)
	})

	i := 0
	for _, a := range srt {
		p := idx.metaData[a].published.Format(timeFormat)
		if idx.metaData[a].published.Equal(time.Unix(0, 0)) {
			p = "draft"
		}
		buf.WriteString("<TR BGCOLOR=\"" + bgf[i%2 == 0] + "\">" +
			"<TD><INPUT TYPE=\"radio\" NAME=\"filename\" VALUE=\"" + a + "\">&nbsp;" +
			"<A HREF=\"/" + url.QueryEscape(strings.TrimSuffix(a, ".md")) + "\" TARGET=\"_blank\">" + html.EscapeString(a) + "</A></TD>" +
			"<TD>" + idx.metaData[a].author + "</TD>" +
			"<TD>" + p + "</TD>" +
			"<TD>" + idx.metaData[a].modified.Format(timeFormat) + "</TD></TR>\n")
		i++
	}

	buf.WriteString(`</TABLE>`)
	return buf.String(), nil
}

func (m post) save(fileName, postText string) error {
	if fileName == "" {
		return nil
	}
	fullFilename := path.Join(*rootDir, *postsDir, path.Base(unescapeOrEmpty(fileName)))
	log.Printf("Saving %q", fullFilename)
	err := os.WriteFile(fullFilename+".tmp", []byte(postText), 0644)
	if err != nil {
		return errors.New("unable to write temp file for %q: " + err.Error())
	}
	st, err := os.Stat(fullFilename + ".tmp")
	if err != nil {
		return errors.New("unable to stat temp file for %q: " + err.Error())
	}
	if st.Size() != int64(len(postText)) {
		return errors.New("temp file size != input size")
	}
	err = os.Rename(fullFilename+".tmp", fullFilename)
	if err != nil {
		return errors.New("unable to rename temp file to the target file: " + err.Error())
	}
	return nil
}

func (post) load(fileName string) (string, error) {
	f, err := os.ReadFile(path.Join(*rootDir, *postsDir, path.Base(unescapeOrEmpty(fileName))))
	if err != nil {
		return "", errors.New("unable to read " + fileName + " : " + err.Error())
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
	for x, i := range m {
		if i.IsDir() || strings.HasPrefix(i.Name(), ".") ||
			!(strings.HasSuffix(i.Name(), ".jpg") ||
				strings.HasSuffix(i.Name(), ".png") ||
				strings.HasSuffix(i.Name(), ".gif")) {
			continue
		}
		if x%8 == 0 {
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
	jpwd, err := secretsStore.Get(nil, "user:"+user)
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
		return errors.New("unable to open user db")
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
	return secretsStore.Put(nil, "user:"+user, spwd)
}

func (creds) del(user string) error {
	if *secrets == "" || secretsStore == nil {
		return errors.New("unable to open user db")
	}
	return secretsStore.Delete(nil, "user:"+user)
}

func (c creds) manager() {
	switch flag.Arg(1) {
	case "passwd":
		if flag.Arg(2) == "" {
			log.Fatal("usage: bloki user passwd <username>")
		}
		var pwd string
		fmt.Print("New Password (WILL ECHO): ")
		fmt.Scanln(&pwd)
		err := c.set(flag.Arg(2), pwd)
		if err != nil {
			log.Fatal(err)
		}
	case "delete":
		if flag.Arg(2) == "" {
			log.Fatal("usage: bloki user delete <username>")
		}
		err := c.del(flag.Arg(2))
		if err != nil {
			log.Fatal(err)
		}
	case "list":
		usr, err := secretsStore.Keys()
		if err != nil {
			log.Fatal(err)
		}
		for _, u := range usr {
			if !strings.HasPrefix(u, "user:") {
				continue
			}
			fmt.Println(u)
		}
	default:
		fmt.Println("usage: bloki user <passwd|delete|list> [username]")
	}
}

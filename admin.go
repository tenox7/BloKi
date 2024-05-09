// Admin TODO
// media list sort & search
// sort list of articles by user input
// also sort unpublished
// search posts
// post list pagination
// user management
// stats

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

func serveAdmin(w http.ResponseWriter, r *http.Request) {
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

	switch r.FormValue("tab") {
	case "media":
		m := media{}
		adm.AdminTab, err = m.mediaAdmin(r)
		adm.ActiveTab = "media"
	default:
		m := post{}
		adm.AdminTab, err = m.serve(r, user)
		adm.ActiveTab = "posts"
	}
	if err != nil {
		log.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates["admin"].Execute(w, adm)
}

func (m post) serve(r *http.Request, user string) (string, error) {
	switch {
	case r.FormValue("edit") != "" && r.FormValue("filename") != "":
		return m.edit(r.FormValue("filename"))

	case r.FormValue("save") != "" && r.FormValue("filename") != "":
		err := m.save(r.FormValue("filename"), r.FormValue("textdata"))
		if err != nil {
			log.Printf("Unable to save post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		log.Printf("Saved post %q", r.FormValue("filename"))
		// TODO: idx update single article
		idx.indexArticles()

	case r.FormValue("rename") != "" && r.FormValue("filename") != "":
		oldname := path.Base(unescapeOrEmpty(r.FormValue("filename")))
		newname := r.FormValue("rename")
		if !strings.HasSuffix(newname, ".md") {
			newname = newname + ".md"
		}
		newname = path.Base(unescapeOrEmpty(newname))
		err := os.Rename(
			path.Join(*rootDir, *postsDir, oldname),
			path.Join(*rootDir, *postsDir, newname),
		)
		if err != nil {
			log.Printf("Unable to rename post from %q to %q: %v", r.FormValue("filename"), newname, err)
			return "", err
		}
		idx.renamePost(oldname, newname)
		log.Printf("Renamed post %v to %v", r.FormValue("filename"), newname)

	case r.FormValue("delete") == "true" && r.FormValue("filename") != "":
		filename := path.Base(unescapeOrEmpty(r.FormValue("filename")))
		err := os.Remove(path.Join(*rootDir, *postsDir, filename))
		if err != nil {
			log.Printf("Unable to delete post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		idx.deletePost(r.FormValue("filename"))
		log.Printf("Deleted post %q", filename)

	case r.FormValue("newpost") != "" && r.FormValue("newpost") != "null":
		filename := r.FormValue("newpost")
		if !strings.HasSuffix(filename, ".md") {
			filename = filename + ".md"
		}
		_, err := os.Stat(path.Join(*rootDir, *postsDir, path.Base(unescapeOrEmpty(filename))))
		if err == nil {
			return "", fmt.Errorf("new post file %q already exists", filename)
		}
		err = m.save(filename,
			"[//]: # (not-published="+time.Now().Format(timeFormat)+")\n[//]: # (author="+user+")\n\n# New Post!\n\nHello world!\n\n")
		if err != nil {
			log.Printf("Unable to save post %q: %v", filename, err)
			return "", err
		}
		log.Printf("Created new post %q", filename)
		// TODO: idx update single article
		idx.indexArticles()
		return m.edit(filename)
	}
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

func (post) save(fileName, postText string) error {
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

func (m media) mediaAdmin(r *http.Request) (string, error) {
	switch {
	case r.FormValue("rename") != "" && r.FormValue("filename") != "":
		err := os.Rename(
			path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(r.FormValue("filename")))),
			path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(r.FormValue("rename")))),
		)
		if err != nil {
			log.Printf("Unable to rename media from %q to %q: %v", r.FormValue("filename"), r.FormValue("rename"), err)
			return "", err
		}
		log.Printf("Renamed media %v to %v", r.FormValue("filename"), r.FormValue("rename"))

	case r.FormValue("delete") == "true" && r.FormValue("filename") != "":
		err := os.Remove(path.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(r.FormValue("filename")))))
		if err != nil {
			log.Printf("Unable to delete media %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		log.Printf("Deleted media %q", r.FormValue("filename"))

	case r.FormValue("upload") != "":
		i, h, err := r.FormFile("fileup")
		if err != nil {
			log.Printf("Unable to upload file %v", err)
			return "", err
		}
		if h.Filename == "" {
			return m.mediaList()
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
		return m.mediaList()
	}
	return m.mediaList()
}

func (media) mediaList() (string, error) {
	buf := strings.Builder{}
	buf.WriteString(`<H1>Media</H1>
	<INPUT TYPE="HIDDEN" NAME="tab" VALUE="media">
	<INPUT TYPE="SUBMIT" NAME="rename" VALUE="Rename" ONCLICK="this.value=prompt('Enter new name:', '');">
	<INPUT TYPE="SUBMIT" NAME="delete" VALUE="Delete" ONCLICK="this.value=confirm('Are you sure you want to delete this image?');">
	<INPUT TYPE="SUBMIT" NAME="upload" VALUE="Upload">
	<INPUT TYPE="FILE" NAME="fileup">

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
	if ok {
		if c.check(u, p) {
			return u, true
		}
	}
	log.Printf("Unauthorized %q from %q", u, r.RemoteAddr)
	w.Header().Set("WWW-Authenticate", "Basic realm=\"BloKi "+*siteName+"\"")
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return "", false
}

func (creds) check(user, pass string) bool {
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

// Admin TODO
// sort list of articles
// search posts
// post list pagination pagination
// media management
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

func serveAdmin(w http.ResponseWriter, r *http.Request) {
	var err error
	r.ParseMultipartForm(10 << 20)
	user, ok := auth(w, r)
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
		adm.AdminTab, err = mediaAdmin(r)
		adm.ActiveTab = "media"
	default:
		adm.AdminTab, err = postAdmin(r, user)
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

func postAdmin(r *http.Request, user string) (string, error) {
	switch {
	case r.FormValue("edit") != "" && r.FormValue("filename") != "":
		return postEdit(r.FormValue("filename"))

	case r.FormValue("save") != "" && r.FormValue("filename") != "":
		err := postSave(r.FormValue("filename"), r.FormValue("textdata"))
		if err != nil {
			log.Printf("Unable to save post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		idx.indexArticles()

	case r.FormValue("rename") != "" && r.FormValue("filename") != "":
		os.Rename(path.Join(*rootDir, *postsDir, r.FormValue("filename")), path.Join(*rootDir, *postsDir, r.FormValue("rename")))
		idx.renamePost(r.FormValue("filename"), r.FormValue("rename"))
		log.Printf("Renamed %v to %v", r.FormValue("filename"), r.FormValue("rename"))
		r.Form.Set("filename", r.FormValue("rename"))
		idx.indexArticles()

	case r.FormValue("delete") == "true" && r.FormValue("filename") != "":
		os.Remove(path.Join(*rootDir, *postsDir, r.FormValue("filename")))
		idx.deletePost(r.FormValue("filename"))
		log.Printf("Deleted post %q", r.FormValue("filename"))
		r.Form.Del("filename")
		idx.indexArticles()

	case r.FormValue("newpost") != "":
		r.Form.Set("filename", r.FormValue("newpost"))
		// os stat to see if file exists and refuse to overwrite it
		_, err := os.Stat(path.Join(*rootDir, *postsDir, r.FormValue("filename")))
		if err == nil {
			return "", fmt.Errorf("file %q already exists", r.FormValue("filename"))
		}
		err = postSave(r.FormValue("filename"),
			"[//]: # (not-published="+time.Now().Format(timeFormat)+")\n[//]: # (author="+user+")\n\n# New Post!\n\nHello world!\n\n")
		if err != nil {
			log.Printf("Unable to save post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		log.Printf("Created new post %q", r.FormValue("filename"))
		return postEdit(r.FormValue("filename"))
	}
	return postList()
}

// perhaps we should have update in place, save and reopen
func postEdit(fn string) (string, error) {
	data, err := postLoad(fn)
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
func postList() (string, error) {
	buf := strings.Builder{}
	buf.WriteString(`<H1>Posts</H1>
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
		fn := idx.metaData[a].filename
		buf.WriteString("<TR BGCOLOR=\"" + bgf[i%2 == 0] + "\">" +
			"<TD><INPUT TYPE=\"radio\" NAME=\"filename\" VALUE=\"" + a + "\">&nbsp;" +
			"<A HREF=\"/" + url.PathEscape(strings.TrimSuffix(fn, ".md")) + "\" TARGET=\"_blank\">" + html.EscapeString(fn) + "</A></TD>" +
			"<TD>" + idx.metaData[a].author + "</TD>" +
			"<TD>" + p + "</TD>" +
			"<TD>" + idx.metaData[a].modified.Format(timeFormat) + "</TD></TR>\n")
		i++
	}

	buf.WriteString(`</TABLE>`)

	return buf.String(), nil
}

func postSave(fileName, postText string) error {
	if fileName == "" {
		return nil
	}
	fullFilename := path.Join(*rootDir, *postsDir, fileName)
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

func postLoad(fileName string) (string, error) {
	f, err := os.ReadFile(path.Join(*rootDir, *postsDir, fileName))
	if err != nil {
		return "", errors.New("unable to read " + fileName + " : " + err.Error())
	}
	return html.EscapeString(string(f)), nil
}

func mediaAdmin(r *http.Request) (string, error) {
	switch {
	}
	return mediaList()
}

func mediaList() (string, error) {
	buf := strings.Builder{}
	buf.WriteString("<H1>Media</H1>\n<TABLE BORDER=\"0\" CELLSPACING=\"10\"><TR>\n")
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
		buf.WriteString("<TD BGCOLOR=\"#D0D0D0\" ALIGN=\"center\" VALIGN=\"bottom\"><IMG SRC=\"/media/" + url.PathEscape(i.Name()) + "\" BORDER=\"0\" TITLE=\"" + nm + "\" ALT=\"" + nm + "\" WIDTH=\"150\"><BR>" + nm + "</TD>\n")
	}
	buf.WriteString("</TR></TABLE>\n")
	return buf.String(), nil
}

func auth(w http.ResponseWriter, r *http.Request) (string, bool) {
	if *secrets == "" || secretsStore == nil {
		http.Error(w, "unable to get user db", http.StatusUnauthorized)
		return "", false
	}
	u, p, ok := r.BasicAuth()
	if ok {
		if userCheck(u, p) {
			return u, true
		}
	}
	log.Printf("Unauthorized %q from %q", u, r.RemoteAddr)
	w.Header().Set("WWW-Authenticate", "Basic realm=\"BloKi "+*siteName+"\"")
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return "", false
}

func userCheck(user, pass string) bool {
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

func userSet(user, pass string) error {
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

func userDel(user string) error {
	if *secrets == "" || secretsStore == nil {
		return errors.New("unable to open user db")
	}
	return secretsStore.Delete(nil, "user:"+user)
}

func manageUsers() {
	switch flag.Arg(1) {
	case "passwd":
		if flag.Arg(2) == "" {
			log.Fatal("usage: bloki user passwd <username>")
		}
		var pwd string
		fmt.Print("New Password (WILL ECHO): ")
		fmt.Scanln(&pwd)
		err := userSet(flag.Arg(2), pwd)
		if err != nil {
			log.Fatal(err)
		}
	case "delete":
		if flag.Arg(2) == "" {
			log.Fatal("usage: bloki user delete <username>")
		}
		err := userDel(flag.Arg(2))
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

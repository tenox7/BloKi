// Admin TODO
// search posts
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
	"os"
	"path"
	"strings"
	"time"
)

type AdminTemplate struct {
	SiteName string
	AdminTab string
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
	}

	adm.AdminTab, err = postAdmin(r, user)
	if err != nil {
		log.Print(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates["admin"].Execute(w, adm)
}

func postAdmin(r *http.Request, user string) (string, error) {
	var err error
	var textArea string
	switch {
	case r.FormValue("save") != "" && r.FormValue("filename") != "":
		err := savePost(r.FormValue("filename"), r.FormValue("textdata"))
		if err != nil {
			log.Printf("Unable to save post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		textArea = r.FormValue("textdata")
		idx.indexArticles()
	case r.FormValue("load") != "" && r.FormValue("filename") != "":
		textArea, err = loadPost(r.FormValue("filename"))
		if err != nil {
			log.Printf("Unable to load post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
	case r.FormValue("rename") != "" && r.FormValue("filename") != "":
		os.Rename(path.Join(*rootDir, *postsDir, r.FormValue("filename")), path.Join(*rootDir, *postsDir, r.FormValue("rename")))
		idx.renamePost(r.FormValue("filename"), r.FormValue("rename"))
		log.Printf("Renamed %v to %v", r.FormValue("filename"), r.FormValue("rename"))
		r.Form.Set("filename", r.FormValue("rename"))
	case r.FormValue("delete") == "true" && r.FormValue("filename") != "":
		os.Remove(path.Join(*rootDir, *postsDir, r.FormValue("filename")))
		idx.deletePost(r.FormValue("filename"))
		log.Printf("Deleted post %q", r.FormValue("filename"))
		r.Form.Del("filename")
	case r.FormValue("newpost") != "":
		r.Form.Set("filename", r.FormValue("newpost"))
		textArea = "[//]: # (not-published=" + time.Now().Format(timeFormat) + ")\n[//]: # (author=" + user + ")\n\n# New Post!\n\nHello world!\n\n"
		err := savePost(r.FormValue("filename"), textArea)
		if err != nil {
			log.Printf("Unable to save post %q: %v", r.FormValue("filename"), err)
			return "", err
		}
		log.Printf("Saved post %q", r.FormValue("filename"))
	}

	buf := strings.Builder{}
	buf.WriteString(`
		<TABLE WIDTH="100%" HEIGHT="80%" BGCOLOR="#FFFFFF" CELLPADDING="0" CELLSPACING="0" BORDER="0">
		<TR>
			<TD NOWRAP WIDTH="100%" VALIGN="MIDDLE" ALIGN="LEFT" COLSPAN="2">
				<INPUT TYPE="SUBMIT" NAME="newpost" VALUE="New Post" ONCLICK="this.value=prompt('Name the new post:', 'new-post.md');">
				<INPUT TYPE="SUBMIT" NAME="load" VALUE="Load">
				<INPUT TYPE="SUBMIT" NAME="save" VALUE="Save">
				<INPUT TYPE="SUBMIT" NAME="view" VALUE="View">
				<INPUT TYPE="SUBMIT" NAME="rename" VALUE="Rename" ONCLICK="this.value=prompt('Enter new name:', '');">
				<INPUT TYPE="SUBMIT" NAME="delete" VALUE="Delete" ONCLICK="this.value=confirm('Are you sure you want to delete this post?');">
			</TD>
		</TR>	
		<TR>
		<TD NOWRAP WIDTH="10%" BGCOLOR="#F0F0F0" VALIGN="top">
			<SELECT NAME="filename" SIZE="20" STYLE="width: 100%; height: 100%">
	`)

	d, err := os.ReadDir(path.Join(*rootDir, *postsDir))
	if err != nil {
		return "", err
	}
	for _, f := range d {
		if f.IsDir() || f.Name()[0:1] == "." || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		buf.WriteString("<OPTION " + selected(f.Name(), r.FormValue("filename")) + ">" + html.EscapeString(f.Name()) + "</OPTION>\n")
	}

	buf.WriteString(`
			</SELECT>
		</TD>
		<TD NOWRAP WIDTH="80%" VALIGN="top">
			<TEXTAREA NAME="textdata" SPELLCHECK="true" COLS="80" ROWS="24" WRAP="soft" STYLE="width: 98%; height: 98%">` + textArea + `</TEXTAREA>
		</TD>
	</TR>
	</TABLE>
	`)

	return buf.String(), nil
}

func savePost(fileName, postText string) error {
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

func loadPost(fileName string) (string, error) {
	f, err := os.ReadFile(path.Join(*rootDir, *postsDir, fileName))
	if err != nil {
		return "", errors.New("unable to read " + fileName + " : " + err.Error())
	}
	return html.EscapeString(string(f)), nil
}

func selected(a, b string) string {
	if a == b {
		return "SELECTED"
	}
	return ""
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
	}
}

// BloKi - Blog & Wiki Engine
package main

import (
	"crypto/tls"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/tenox7/tkvs"
	"golang.org/x/crypto/acme/autocert"
)

var (
	siteName = flag.String("site_name", "My Blog", "Name of your blog")
	subTitle = flag.String("subtitle", "Blog about awesome things!", "Subtitle")
	artPerPg = flag.Int("articles_per_page", 5, "number of articles per page")
	ltsPosts = flag.Int("latest_posts", 15, "number of latests posts on the side")
	adminUri = flag.String("admin_uri", "/bk-admin/", "address of the admin interface")
	rootDir  = flag.String("root_dir", "site/", "directory where site data is stored")
	postsDir = flag.String("posts_subdir", "posts/", "directory holding user posts, relative to root dir")
	mediaDir = flag.String("media_subdir", "media/", "directory holding user media, relative to root dir")
	htmplDir = flag.String("template_subdir", "templates/", "directory holding html templates, relative to root dir")
	chroot   = flag.Bool("chroot", false, "chroot to root dir, requires root")
	secrets  = flag.String("secrets", "", "location of secrets file, outside of chroot/site dir")
	suidUser = flag.String("setuid", "", "Username or uid:gid pair, to setuid to if started as root")
	bindAddr = flag.String("addr", ":8080", "listener address, eg. :8080 or :443")
	fastCgi  = flag.Bool("fastcgi", false, "enable FastCGI mode")
	useGit   = flag.Bool("use_git", true, "use git repo, enabled by default")
	acmBind  = flag.String("acm_addr", "", "autocert manager listen address, eg: :80")
	acmWhLst multiString
)

var (
	charset = map[bool]string{
		true:  "UTF-8",
		false: "ISO-8859-1",
	}

	//go:embed favicon.ico
	favIcon []byte

	//go:embed templates/admin.html templates/modern.html templates/legacy.html templates/vintage.html
	templateFS embed.FS

	templates    map[string]*template.Template
	idx          postIndex
	txt          textSearch
	secretsStore *tkvs.TKVS
)

func handleMedia(w http.ResponseWriter, r *http.Request) {
	f, err := os.ReadFile(filepath.Join(*rootDir, *mediaDir, path.Base(unescapeOrEmpty(r.URL.Path))))
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "error")
		log.Print(err.Error())
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(f))
	w.Write(f)
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Write(favIcon)
}

func handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "User-agent: *\nAllow: /\n")
}

func vintage(ua string) string {
	switch {
	case strings.HasPrefix(ua, "Mozilla/5"):
		return "modern"
	case strings.HasPrefix(ua, "Mozilla/4"):
		return "legacy"
	default:
		return "vintage"
	}
}

func atoiOrZero(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

func atoiOrFatal(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		log.Fatal(err)
	}
	return i
}

func unescapeOrEmpty(s string) string {
	u, err := url.QueryUnescape(s)
	if err != nil {
		log.Printf("unescape: %q err=%v", s, err)
		return ""
	}
	return u
}

type multiString []string

func (z *multiString) String() string {
	return "something"
}

func (z *multiString) Set(v string) error {
	*z = append(*z, v)
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Print("Starting up...")
	acm := autocert.Manager{Prompt: autocert.AcceptTOS}
	templates = make(map[string]*template.Template)
	var err error
	flag.Var(&acmWhLst, "acm_host", "autocert manager allowed hostname (multi)")
	flag.Parse()

	// http handlers
	http.HandleFunc("/", handlePosts)
	http.HandleFunc("/media/", handleMedia)
	http.HandleFunc(*adminUri, handleAdmin)
	http.HandleFunc("/robots.txt", handleRobots)
	http.HandleFunc("/favicon.ico", handleFavicon)

	// open secrets before chroot
	if *secrets != "" {
		secretsStore = tkvs.New(*secrets, autocert.ErrCacheMiss)
		if secretsStore == nil {
			log.Fatal("Unable to open secrets file")
		}
		log.Printf("Opened secrets store with %v keys", len(secretsStore.Keys()))
	}

	// manage users
	if flag.Arg(0) == "user" {
		cliUserManager()
		return
	}

	// find uid/gid for setuid before chroot
	suid, sgid := getSuidSgid()

	// chroot before setuid
	chRoot()

	// listen/bind to port before setuid
	l, err := net.Listen("tcp", *bindAddr)
	if err != nil {
		log.Fatalf("unable to listen on %v: %v", *bindAddr, err)
	}
	log.Printf("Listening on %q", *bindAddr)

	// auto cert startup
	if *acmBind != "" && len(acmWhLst) > 0 && secretsStore != nil {
		acm.Cache = secretsStore
		acm.HostPolicy = autocert.HostWhitelist(acmWhLst...)
		al, err := net.Listen("tcp", *acmBind)
		if err != nil {
			log.Fatalf("unable to listen on %v: %v", *acmBind, err)
		}
		log.Printf("Starting ACME HTTP server on %v", *acmBind)
		go func() {
			err = http.Serve(al, acm.HTTPHandler(http.DefaultServeMux))
			if err != nil {
				log.Fatalf("unable to start acme http: %v", err)
			}
		}()
	}

	// setuid now
	setUidGid(suid, sgid)

	// check site, articles & media
	st, err := os.Stat(*rootDir)
	if os.IsNotExist(err) {
		err = os.Mkdir(*rootDir, 0755)
		if err != nil {
			log.Fatalf("Unable to create site directory: %v", err)
		}
	} else if !st.IsDir() {
		log.Fatalf("%v is a file", *rootDir)
	}
	st, err = os.Stat(path.Join(*rootDir, *postsDir))
	if os.IsNotExist(err) {
		log.Print("Posts directory does not not exist, creating")
		err = os.Mkdir(path.Join(*rootDir, *postsDir), 0755)
		if err != nil {
			log.Fatalf("Unable to create articles directory: %v", err)
		}
		err = gitInit()
		if err != nil {
			log.Printf("Unable to init git repo: %v", err)
		}
		idx.rescan()
		txt.rescan()
		po := post{user: "bloki"}
		_, err = po.save("my-first-post.md",
			"<!--published=\""+time.Now().Format(timeFormat)+"\"-->\n\n"+
				"# My first blog post!\n\nHello World!\n\n")
		if err != nil {
			log.Fatalf("Unable to create first post: %v", err)
		}
	} else if !st.IsDir() {
		log.Fatalf("%v is a file", path.Join(*rootDir, *postsDir))
	}
	st, err = os.Stat(path.Join(*rootDir, *mediaDir))
	if os.IsNotExist(err) {
		err = os.Mkdir(path.Join(*rootDir, *mediaDir), 0755)
		if err != nil {
			log.Fatalf("Unable to create media directory: %v", err)
		}
	} else if !st.IsDir() {
		log.Fatalf("%v is a file", path.Join(*rootDir, *postsDir))
	}

	// load templates
	for _, t := range []string{"vintage", "legacy", "modern", "admin"} {
		tpl, err := template.ParseFiles(path.Join(*rootDir, *htmplDir, t+".html"))
		switch err {
		case nil:
			templates[t] = tpl
			log.Printf("Loaded local template %q from disk", t)
		default:
			templates[t], err = template.ParseFS(templateFS, *htmplDir+t+".html")
			if err != nil {
				log.Fatalf("error parsing embedded template %q: %v", t, err)
			}
			log.Printf("Loaded embedded template %q", t)
		}
	}

	// index articles
	idx.rescan()

	// start text search
	txt.rescan()

	// favicon
	fst, err := os.Stat(path.Join(*rootDir, "favicon.ico"))
	if err == nil && !fst.IsDir() {
		f, err := os.ReadFile(path.Join(*rootDir, "favicon.ico"))
		if err == nil || len(f) > 0 {
			favIcon = f
			log.Print("Loaded local favicon.ico")
		}
	}

	// http(s) bind stuff
	switch {
	case *acmBind != "" && *secrets != "" && len(acmWhLst) > 0:
		https := &http.Server{
			Addr:      *bindAddr,
			Handler:   http.DefaultServeMux,
			TLSConfig: &tls.Config{GetCertificate: acm.GetCertificate},
		}
		log.Print("Starting HTTPS TLS Server with ACM on ", *bindAddr)
		err = https.ServeTLS(l, "", "")
	case *fastCgi:
		log.Print("Starting FastCGI Server")
		fcgi.Serve(l, http.DefaultServeMux)
	default:
		log.Print("Starting plain HTTP Server")
		err = http.Serve(l, http.DefaultServeMux)
	}
	if err != nil {
		log.Fatal(err)
	}
}

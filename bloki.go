// BloKi - Blog & Wiki Engine
package main

// TODO:
// index update for a single file + re-run sequence
// make admin save/rename/delete use that
// reindex on signal
// 2fa for admin login, probably
// https://www.twilio.com/docs/verify/quickstarts/totp
// user manager
// render node hook for /media/
// wiki style links to post/media/etc
// continue reading block element inside a post like in WP
// user comments
// s3 support
// render to static site
// throttle qps
// fastcgi
// wiki mode
// startup wizard
// statistics module, page views, latencies, etc
// service files, etc
// git integration
// make robots.txt configurable
// docker container
// cloud run, fargate
// lambda, cloud functions
// better error handling, use string builder

// MAYBE:
// constant make width - 1000px in WP?
// reindex on inotify (incl rename/move)

import (
	"crypto/tls"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/tenox7/tkvs"
	"golang.org/x/crypto/acme/autocert"
)

var (
	siteName = flag.String("site_name", "My Blog", "Name of your blog")
	subTitle = flag.String("subtitle", "&nbsp;", "Subtitle")
	artPerPg = flag.Int("articles_per_page", 3, "number of articles per page")
	adminUri = flag.String("admin_uri", "/bk-admin/", "address of the admin interface")
	rootDir  = flag.String("root_dir", "site/", "directory where site data is stored")
	postsDir = flag.String("posts_subdir", "posts/", "directory holding user posts, relative to root dir")
	mediaDir = flag.String("media_subdir", "media/", "directory holding user media, relative to root dir")
	htmplDir = flag.String("template_subdir", "templates/", "directory holding html templates, relative to root dir")
	chroot   = flag.Bool("chroot", false, "chroot to root dir, requires root")
	secrets  = flag.String("secrets", "", "location of secrets file, outside of chroot/site dir")
	suidUser = flag.String("setuid", "", "Username to setuid to if started as root")
	bindAddr = flag.String("addr", ":8080", "listener address, eg. :8080 or :443")
	acmBind  = flag.String("acm_addr", "", "autocert manager listen address, eg: :80")
	acmWhLst multiString
)

var (
	timeFormat  = "2006-01-02 15:04"
	publishedRe = regexp.MustCompile(`\[//\]: # \(published=(.+)\)`)
	authorRe    = regexp.MustCompile(`\[//\]: # \(author=(.+)\)`)
	charset     = map[bool]string{
		true:  "UTF-8",
		false: "ISO-8859-1",
	}

	//go:embed favicon.ico
	favIcon []byte

	//go:embed templates/*.html
	templateFS embed.FS

	templates    map[string]*template.Template
	idx          postIndex
	secretsStore *tkvs.KVS
)

type postIndex struct {
	pubSorted []string
	metaData  map[string]postMetadata
	pageLast  int

	sync.RWMutex
}

type postMetadata struct {
	author    string
	published time.Time
	modified  time.Time
	tile      string
	filename  string
}

type TemplateData struct {
	SiteName  string
	SubTitle  string
	Articles  string
	CharSet   string
	Paginator string
	Page      int
	PgNewer   int
	PgOlder   int
	PgOldest  int
}

func renderMd(md []byte, name, published string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.Autolink)
	d := p.Parse(md)
	r := html.NewRenderer(html.RendererOptions{
		RenderNodeHook: func() html.RenderNodeFunc {
			return func(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
				if h, ok := node.(*ast.Heading); !ok || h.Level != 1 {
					return ast.GoToNext, false
				}
				if !entering {
					io.WriteString(w, "</a></h1>\n\n<i>"+published+"</i>\n\n")
					return ast.GoToNext, true
				}
				io.WriteString(w, "<h1><a href=\""+name+"\">")
				return ast.GoToNext, true
			}
		}(),
	})
	return string(markdown.Render(d, r))
}

func renderError(name, errStr string) string {
	return string("Article " + name + " " + errStr + "<p>\n\n")
}

func (t *TemplateData) renderArticle(name string) {
	idx.RLock()
	m := idx.metaData[name]
	idx.RUnlock()
	if m.published.Equal(time.Unix(0, 0)) {
		t.Articles = renderError(name, "not found") // TODO: better error handling
		return
	}
	article, err := os.ReadFile(path.Join(*rootDir, *postsDir, name))
	if err != nil {
		t.Articles = renderError(name, "not found") // TODO: better error handling
		return
	}
	p := "By " + m.author + ", First published: " + m.published.Format(timeFormat) + ", Last updated: " + m.modified.Format(timeFormat)
	t.Articles += renderMd(article, strings.TrimSuffix(name, ".md"), p)
}

func (t *TemplateData) paginatePosts(pg int) {
	idx.RLock()
	seq := idx.pubSorted
	pgl := idx.pageLast
	idx.RUnlock()
	t.Page = pg
	t.PgOlder = pg + 1
	t.PgNewer = pg - 1
	t.PgOldest = pgl
	for i := t.Page * (*artPerPg); i < (t.Page+1)*(*artPerPg) && i < len(seq); i++ {
		t.renderArticle(seq[i])
	}
}

func servePosts(w http.ResponseWriter, r *http.Request) {
	log.Printf("view from=%q uri=%q url=%q, ua=%q", r.RemoteAddr, r.RequestURI, r.URL.Path, r.UserAgent())
	fi := path.Base(r.URL.Path)

	td := TemplateData{
		SiteName: *siteName,
		SubTitle: *subTitle,
		CharSet:  charset[strings.HasPrefix(r.UserAgent(), "Mozilla/5")],
	}

	switch {
	case len(fi) > 1:
		td.renderArticle(fi + ".md")
	default:
		r.ParseForm()
		td.paginatePosts(atoiOrZero(r.FormValue("pg")))
	}

	w.Header().Set("Content-Type", "text/html")
	err := templates[vintage(r.UserAgent())].Execute(w, td)
	if err != nil {
		log.Print(err.Error())
		io.WriteString(w, err.Error())
	}
}

func serveMedia(w http.ResponseWriter, r *http.Request) {
	f, err := os.ReadFile(filepath.Join(*rootDir, *mediaDir, path.Base(r.URL.Path)))
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "error")
		log.Print(err.Error())
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(f))
	w.Write(f)
}

func serveFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Write(favIcon)
}

func serveRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "User-agent: *\nAllow: /\n")
}

func (i *postIndex) indexArticles() {
	start := time.Now()
	d, err := os.ReadDir(path.Join(*rootDir, *postsDir))
	if err != nil {
		log.Fatal(err)
	}
	meta := make(map[string]postMetadata)
	seq := []string{}
	for _, f := range d {
		// move this whole thing in to another function that can be called externally
		// for example by admin save/rename/delete
		if f.IsDir() || f.Name()[0:1] == "." || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		fi, err := f.Info()
		if err != nil {
			continue
		}
		a, err := os.ReadFile(path.Join(*rootDir, *postsDir, f.Name()))
		if err != nil {
			log.Printf("error reading %v: %v", f.Name(), err)
			continue
		}
		author := authorRe.FindSubmatch(a)
		if len(author) < 2 {
			author = [][]byte{[]byte(""), []byte("unknown")}
		}
		m := publishedRe.FindSubmatch(a)
		if len(m) < 1 {
			m = [][]byte{[]byte(""), []byte("")}
		}
		t, err := time.Parse(timeFormat, string(m[1]))
		if err != nil {
			t = time.Unix(0, 0)
		}
		// TODO: add title from regexp
		meta[f.Name()] = postMetadata{
			filename:  f.Name(),
			modified:  fi.ModTime(),
			published: t,
			author:    string(author[1]),
		}
		if t.Equal(time.Unix(0, 0)) {
			continue
		}
		seq = append(seq, f.Name())
	}
	sort.Slice(seq, func(i, j int) bool {
		return meta[seq[j]].published.Before(meta[seq[i]].published)
	})
	pgMax := int(math.Ceil(float64(len(seq))/float64(*artPerPg)) - 1)
	log.Printf("Indexed %v articles, sequenced: %+v, last page is %v, duration %v", len(seq), seq, pgMax, time.Since(start))
	i.Lock()
	defer i.Unlock()
	idx.metaData = meta
	i.pubSorted = seq
	i.pageLast = pgMax
}

func (i *postIndex) renamePost(old, new string) {
	i.Lock()
	defer i.Unlock()
	for n, p := range i.pubSorted {
		if p != old {
			continue
		}
		i.pubSorted[n] = new
	}
	log.Printf("rename %q to %q, new index: %+v", old, new, i.pubSorted)
}

func (i *postIndex) deletePost(name string) {
	i.Lock()
	defer i.Unlock()
	seq := []string{}
	for _, s := range i.pubSorted {
		if s == name {
			continue
		}
		seq = append(seq, s)
	}
	i.pubSorted = seq
	log.Printf("deleted post %v, new index: %+v", name, i.pubSorted)
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

func userId(usr string) (int, int, error) {
	u, err := user.Lookup(usr)
	if err != nil {
		return 0, 0, err
	}
	ui, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gi, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return ui, gi, nil
}

func setUid(ui, gi int) error {
	if ui == 0 || gi == 0 {
		return nil
	}
	err := syscall.Setgid(gi)
	if err != nil {
		return err
	}
	err = syscall.Setuid(ui)
	if err != nil {
		return err
	}
	return nil
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
	http.HandleFunc("/", servePosts)
	http.HandleFunc("/media/", serveMedia)
	http.HandleFunc(*adminUri, serveAdmin)
	http.HandleFunc("/robots.txt", serveRobots)
	http.HandleFunc("/favicon.ico", serveFavicon)

	// open secrets before chroot
	if *secrets != "" {
		secretsStore = tkvs.New(*secrets, autocert.ErrCacheMiss)
		if secretsStore == nil {
			log.Fatal("Unable to open secrets file")
		}
		log.Print("Opened secrets store")
	}

	// manage users
	if flag.Arg(0) == "user" {
		manageUsers()
		return
	}

	// find uid/gid for setuid before chroot
	var suid, sgid int
	if *suidUser != "" {
		suid, sgid, err = userId(*suidUser)
		if err != nil {
			log.Fatal("unable to find setuid user", err)
		}
	}

	// chroot
	if *chroot {
		err := syscall.Chroot(*rootDir)
		if err != nil {
			log.Fatal("chroot: ", err)
		}
		log.Print("Chroot to: ", *rootDir)
		*rootDir = "/"
	}

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
	if *suidUser != "" {
		err = setUid(suid, sgid)
		if err != nil {
			log.Fatalf("unable to suid for %v: %v", *suidUser, err)
		}
		log.Printf("Setuid UID=%d GID=%d", os.Geteuid(), os.Getgid())
	}

	// check articles & media
	st, err := os.Stat(path.Join(*rootDir, *postsDir))
	if os.IsNotExist(err) {
		log.Print("articles did not exist, creating")
		err = os.Mkdir(path.Join(*rootDir, *postsDir), 0755)
		if err != nil {
			log.Fatalf("Unable to create articles directory: %v", err)
		}
		err = os.WriteFile(
			path.Join(*rootDir, *postsDir, "my-first-post.md"),
			[]byte("[//]: # (published="+time.Now().Format(timeFormat)+")\n\n# My first blog post!\n\nHello World!\n\n"),
			0644,
		)
		if err != nil {
			log.Fatalf("Unable to create first post: %v", err)
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
			log.Printf("Loaded template %q from disk", t)
		default:
			templates[t], err = template.ParseFS(templateFS, *htmplDir+t+".html")
			if err != nil {
				log.Fatalf("error parsing embedded template %q: %v", t, err)
			}
			log.Printf("Loaded template %q from embed.FS", t)
		}
	}

	// index articles
	idx.indexArticles()

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
	if *acmBind != "" && *secrets != "" && len(acmWhLst) > 0 {
		https := &http.Server{
			Addr:      *bindAddr,
			Handler:   http.DefaultServeMux,
			TLSConfig: &tls.Config{GetCertificate: acm.GetCertificate},
		}
		log.Print("Starting HTTPS TLS Server with ACM on ", *bindAddr)
		err = https.ServeTLS(l, "", "")
	} else {
		log.Print("Starting plain HTTP Server")
		err = http.Serve(l, http.DefaultServeMux)
	}
	if err != nil {
		log.Fatal(err)
	}
}

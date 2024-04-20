// BloKi - Blog & Wiki Engine
package main

// TODO:
// - robots
// - favicon
// - user manager
// - admin interface
// - modern template
// - s3 support
// - render to
// - throttle
// - fastcgi
// - wiki mode
// - startup wizard
// - statistics module, page views, latencies, etc
// - article footer/header should be customizable, a template?
// - articles could be array / range
// - service files, etc
// - git integration

import (
	"crypto/tls"
	"flag"
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

	"gopkg.in/ini.v1"
)

var (
	timeFormat      = "2006-01-02 15:04"
	statusPublished = []byte("[//]: # (published=")
	publishedRe     = regexp.MustCompile(`\[//\]: # \(published=(.+)\)`)
	authorRe        = regexp.MustCompile(`\[//\]: # \(author=(.+)\)`)
	charset         = map[bool]string{
		true:  "UTF-8",
		false: "ISO-8859-1",
	}
	favIcon []byte
)

var (
	rootDir  = flag.String("root_dir", "site/", "directory where site data is stored")
	chroot   = flag.Bool("chroot", false, "chroot to root dir, requires root")
	secrets  = flag.String("secrets", "", "location of secrets file, outside of chroot/site dir")
	suidUser = flag.String("setuid", "", "Username to setuid to if started as root")
	bindAddr = flag.String("addr", ":8080", "listener address, eg. :8080 or :443")
	acmBind  = flag.String("acm_addr", "", "autocert manager listen address, eg: :80")
	acmWhLst multiString
)

type SiteHandler struct {
	ConfigIni struct {
		SiteName        string
		SiteType        string
		ArticlesPerPage int
	}
	Templates map[string]*template.Template
	Index     []string
	Articles  string
	PageLast  int

	sync.Mutex
}

type TemplateData struct {
	SiteName  string
	Template  *template.Template
	Articles  string
	CharSet   string
	Paginator string
	Page      int
	PagePrev  int
	PageNext  int
	PageLast  int
}

func renderMd(md []byte, name string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.Autolink)
	d := p.Parse(md)
	r := html.NewRenderer(html.RendererOptions{
		Flags: html.CommonFlags,
		RenderNodeHook: func() html.RenderNodeFunc {
			return func(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
				if h, ok := node.(*ast.Heading); !ok || h.Level != 1 {
					return ast.GoToNext, false
				}
				if !entering {
					io.WriteString(w, "</a></h1>\n\n")
					return ast.GoToNext, true
				}
				io.WriteString(w, "<h1><a href=\""+name+"\">")
				// TODO: this may be a good place to put author and date of creation
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
	fp := path.Join(*rootDir, "articles", name)
	article, err := os.ReadFile(fp)
	if err != nil {
		t.Articles = renderError(name, "not found") // TODO: better error handling
		return
	}
	log.Printf("read file %q", name)
	fi, err := os.Stat(fp)
	if err != nil {
		t.Articles = renderError(name, "unable to stat")
		return
	}
	updated := fi.ModTime()

	m := publishedRe.FindSubmatch(article)
	if len(m) < 1 {
		m = [][]byte{[]byte(""), []byte("")}
	}
	published, err := time.Parse(timeFormat, string(m[1]))
	if err != nil {
		log.Printf("Unable to parse publication date in %q: %v", name, err)
		published = time.Unix(0, 0)
	}

	author := authorRe.FindSubmatch(article)
	if len(author) < 2 {
		author = [][]byte{[]byte(""), []byte("n/a")}
	}

	// TODO: this should be either a special tag in md or the template
	article = append(article, []byte(
		"\n\nFirst published: "+published.Format(timeFormat)+
			" | Last updated: "+updated.Format(timeFormat)+
			" | Author: "+string(author[1])+
			"\n\n---\n\n")...)

	t.Articles += renderMd(article, strings.TrimSuffix(name, ".md"))
}

func (t *TemplateData) paginateArticles(pg, pl, app int, idx *[]string) {
	t.Page = pg
	t.PageNext = pg + 1
	t.PagePrev = pg - 1
	t.PageLast = pl
	index := *idx
	for i := t.Page * app; i < (t.Page+1)*app && i < len(index); i++ {
		t.renderArticle(index[i])
	}
}

func (h *SiteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	log.Printf("req from=%q uri=%q url=%q, ua=%q", r.RemoteAddr, r.RequestURI, r.URL.Path, r.UserAgent())
	fi := path.Base(r.URL.Path)

	if strings.HasSuffix(fi, ".png") || strings.HasSuffix(fi, ".jpg") {
		serveMedia(w, fi)
		return
	}

	td := TemplateData{
		SiteName: h.ConfigIni.SiteName,
		CharSet:  charset[strings.HasPrefix(r.UserAgent(), "Mozilla/5")],
		Template: h.Templates[vintage(r.UserAgent())],
	}

	switch {
	case len(fi) > 1:
		td.renderArticle(fi + ".md")
	default:
		r.ParseForm()
		td.paginateArticles(atoiOrZero(r.FormValue("pg")), h.PageLast, h.ConfigIni.ArticlesPerPage, &h.Index)
	}

	w.Header().Set("Content-Type", "text/html")
	err := td.Template.Execute(w, td)
	if err != nil {
		log.Print(err.Error())
		io.WriteString(w, err.Error())
	}
}

func serveMedia(w http.ResponseWriter, fName string) {
	f, err := os.ReadFile(filepath.Join(*rootDir, "media", fName))
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

func (hdl *SiteHandler) indexArticles() {
	d, err := os.ReadDir(path.Join(*rootDir, "articles"))
	if err != nil {
		log.Fatal(err)
	}
	birth := make(map[string]time.Time)
	seq := []string{}
	log.Printf("Indexing %v articles...", len(d))
	for _, f := range d {
		if f.IsDir() || f.Name()[0:1] == "." || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}

		a, err := os.ReadFile(path.Join(*rootDir, "articles", f.Name()))
		if err != nil {
			log.Printf("error reading %v: %v", f.Name(), err)
			continue
		}
		log.Printf("indexing file %q", f.Name())

		m := publishedRe.FindSubmatch(a)
		if len(m) < 1 {
			continue
		}
		t, err := time.Parse(timeFormat, string(m[1]))
		if err != nil {
			log.Printf("Unable to parse publication date in %q: %v", f.Name(), err)
			continue
		}
		birth[f.Name()] = t
		seq = append(seq, f.Name())
	}
	sort.Slice(seq, func(i, j int) bool {
		return birth[seq[j]].Before(birth[seq[i]])
	})
	pgMax := int(math.Ceil(float64(len(seq))/float64(hdl.ConfigIni.ArticlesPerPage)) - 1)
	log.Printf("parsed %v articles sequenced: %+v, last page is %v", len(seq), seq, pgMax)
	hdl.Lock()
	defer hdl.Unlock()
	hdl.Index = seq
	hdl.PageLast = pgMax
}

// TODO: implement vintage, curl/lynx
func vintage(ua string) string {
	if strings.HasPrefix(ua, "Mozilla/5") {
		return "modern"
	}
	return "legacy"
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
	var err error
	acm := autocert.Manager{Prompt: autocert.AcceptTOS}
	flag.Var(&acmWhLst, "acm_host", "autocert manager allowed hostname (multi)")
	flag.Parse()

	// open secrets before chroot
	if *secrets != "" {
		acm.Cache = tkvs.NewJsonCache(*secrets, autocert.ErrCacheMiss)
		acm.HostPolicy = autocert.HostWhitelist(acmWhLst...)
		go http.ListenAndServe(*acmBind, acm.HTTPHandler(nil))
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

	// setuid now
	if *suidUser != "" {
		err = setUid(suid, sgid)
		if err != nil {
			log.Fatalf("unable to suid for %v: %v", *suidUser, err)
		}
		log.Printf("Setuid UID=%d GID=%d", os.Geteuid(), os.Getgid())
	}

	// load config.ini
	i, err := ini.Load(path.Join(*rootDir, "config.ini"))
	if err != nil {
		log.Fatal("unable to process config.ini: ", err)
	}

	hdl := &SiteHandler{}
	err = i.MapTo(&hdl.ConfigIni)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Config.ini: %+v", hdl.ConfigIni)

	// load templates
	hdl.Templates = make(map[string]*template.Template)
	for _, t := range []string{"vintage", "legacy", "modern"} {
		tpl, err := template.ParseFiles(path.Join(*rootDir, "templates", t+".html"))
		if err != nil {
			log.Fatal(err)
		}
		hdl.Templates[t] = tpl
	}

	// favicon.ico
	// TODO: make this configurable
	favIcon, _ = os.ReadFile(path.Join(*rootDir, "favicon.ico"))

	// index articles
	hdl.indexArticles()

	// http(s) bind stuff
	http.Handle("/", hdl)
	http.HandleFunc("/favicon.ico", serveFavicon)

	if *acmBind != "" && *secrets != "" && len(acmWhLst) > 0 {
		https := &http.Server{
			Addr:      *bindAddr,
			Handler:   http.DefaultServeMux,
			TLSConfig: &tls.Config{GetCertificate: acm.GetCertificate},
		}
		log.Print("Starting HTTPS TLS Server with ACM")
		err = https.ServeTLS(l, "", "")
	} else {
		log.Print("Starting plain HTTP Server")
		err = http.Serve(l, http.DefaultServeMux)
	}
	if err != nil {
		log.Fatal(err)
	}
}

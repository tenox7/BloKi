package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type TemplateData struct {
	SiteName    string
	SubTitle    string
	Articles    string
	CharSet     string
	Paginator   string
	Page        int
	PgNewer     int
	PgOlder     int
	PgOldest    int
	LatestPosts string
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
	m := idx.metaData[path.Base(unescapeOrEmpty(name))]
	idx.RUnlock()
	if m.published.IsZero() {
		t.Articles = renderError(name, "is not published") // TODO: better error handling
		return
	}
	article, err := os.ReadFile(path.Join(*rootDir, *postsDir, path.Base(unescapeOrEmpty(name))))
	if err != nil {
		t.Articles = renderError(name, "not found") // TODO: better error handling
		return
	}
	article = append(article, []byte("\n\n---\n\n")...)
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

func handlePosts(w http.ResponseWriter, r *http.Request) {
	log.Printf("view from=%q uri=%q url=%q, ua=%q", r.RemoteAddr, r.RequestURI, r.URL.Path, r.UserAgent())
	fi := path.Base(r.URL.Path)

	td := TemplateData{
		SiteName:    *siteName,
		SubTitle:    *subTitle,
		CharSet:     charset[strings.HasPrefix(r.UserAgent(), "Mozilla/5")],
		LatestPosts: func() string { idx.RLock(); defer idx.RUnlock(); return idx.latestPosts }(),
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

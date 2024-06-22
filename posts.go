package main

import (
	"bytes"
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

var moreTag []byte = []byte("<!--more-->")

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
	AdminUrl    string
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
					// TODO: we could potentially use https://github.com/abhinav/goldmark-anchor instead
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

func (t *TemplateData) renderArticle(file string, maxLen int) {
	file = path.Base(unescapeOrEmpty(file))
	idx.RLock()
	m := idx.metaData[file]
	idx.RUnlock()
	if m.published.IsZero() {
		// TODO: searchPosts() may find unpublished articles, they would display an error
		// we also don't want to leak data on a random hit, so say nothing
		//t.Articles = renderError(name, "is not published") // TODO: better error handling
		return
	}
	postMd, err := os.ReadFile(path.Join(*rootDir, *postsDir, file))
	if err != nil {
		log.Printf("unable to read post %q: %v", file, err)
		t.Articles = renderError(file, "not found") // TODO: better error handling
		return
	}
	// TODO: refactor as a custom ast node and render hook instead
	if maxLen > 0 {
		postMd = postMd[:maxLen]
		postMd = append(postMd, []byte("<BR>[Continue Reading...]("+m.url+")")...)
	} else if maxLen != -1 {
		ix := bytes.Index(postMd, moreTag)
		if ix != -1 {
			postMd = postMd[:ix]
			postMd = append(postMd, []byte("<BR>[Continue Reading...]("+m.url+")")...)
		}
	}
	postMd = append(postMd, []byte("\n\n---\n\n")...)
	p := "By " + m.author + ", First published: " + m.published.Format(timeFormat) + ", Last updated: " + m.modified.Format(timeFormat)
	t.Articles += renderMd(postMd, strings.TrimSuffix(file, ".md"), p)
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
		t.renderArticle(seq[i], 0)
	}
}

func (t *TemplateData) searchPosts(query string) {
	res := txt.search(query)
	if len(res) == 0 {
		t.Articles = "<H1>Nothing found</H1>No posts matched the search criteria."
		return
	}
	for i := range res {
		t.renderArticle(res[i], 200)
	}
}

func handlePosts(w http.ResponseWriter, r *http.Request) {
	log.Printf("view from=%q uri=%q url=%q, ua=%q", r.RemoteAddr, r.RequestURI, r.URL.Path, r.UserAgent())
	r.ParseForm()
	post := path.Base(r.URL.Path)
	query := unescapeOrEmpty(r.FormValue("query"))

	td := TemplateData{
		SiteName:    *siteName,
		SubTitle:    *subTitle,
		CharSet:     charset[strings.HasPrefix(r.UserAgent(), "Mozilla/5")],
		LatestPosts: func() string { idx.RLock(); defer idx.RUnlock(); return idx.latestPosts }(),
		AdminUrl:    *adminUri,
	}

	switch {
	case len(post) > 1:
		td.renderArticle(post+".md", -1)
	case query != "":
		td.searchPosts(query)
	default:
		td.paginatePosts(atoiOrZero(r.FormValue("pg")))
	}

	w.Header().Set("Content-Type", "text/html")
	err := templates[vintage(r.UserAgent())].Execute(w, td)
	if err != nil {
		log.Print(err.Error())
		io.WriteString(w, err.Error())
	}
}

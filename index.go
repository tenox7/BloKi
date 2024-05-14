// index handling routines
// index is used to keep in memory copy of post metadata
// mainly used for sorting (sequencing) so that blog posts are displayed in order
package main

import (
	"fmt"
	"html"
	"log"
	"math"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	timeFormat  = "2006-01-02 15:04"
	publishedRe = regexp.MustCompile(`\[//\]: # \(published=(.+)\)`)
	authorRe    = regexp.MustCompile(`\[//\]: # \(author=(.+)\)`)
	titleRe     = regexp.MustCompile(`(?m)^#\s+(.+)`)
)

type postIndex struct {
	pubSorted   []string
	metaData    map[string]postMetadata
	pageLast    int
	latestPosts string

	sync.RWMutex
}

type postMetadata struct {
	author    string
	published time.Time
	modified  time.Time
	title     string
	url       string
}

func (idx *postIndex) rescan() {
	start := time.Now()
	d, err := os.ReadDir(path.Join(*rootDir, *postsDir))
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range d {
		idx.add(f.Name())
	}
	idx.sequence()
	idx.RLock()
	defer idx.RUnlock()
	log.Printf("idx: indexed %v articles, sequenced: %+v, last page is %v, duration %v", len(idx.pubSorted), idx.pubSorted, idx.pageLast, time.Since(start))
}

func (idx *postIndex) sequence() {
	seq := []string{}
	idx.Lock()
	defer idx.Unlock()
	for n := range idx.metaData {
		seq = append(seq, n)
	}
	sort.Slice(seq, func(i, j int) bool {
		return idx.metaData[seq[j]].published.Before(idx.metaData[seq[i]].published)
	})
	idx.pubSorted = seq
	idx.pageLast = int(math.Ceil(float64(len(seq))/float64(*artPerPg)) - 1)
	idx.latestPosts = ""
	for i, s := range seq {
		if i >= *ltsPosts {
			break
		}
		if idx.metaData[s].published.IsZero() {
			continue
		}
		idx.latestPosts += fmt.Sprintf("<li><a href=\"/%v\">%v</a><br>\n", url.QueryEscape(idx.metaData[s].url), html.EscapeString(idx.metaData[s].title))
	}
}

func (idx *postIndex) add(name string) bool {
	if name[0:1] == "." || !strings.HasSuffix(name, ".md") {
		return false
	}
	fullName := path.Join(*rootDir, *postsDir, name)
	fi, err := os.Stat(fullName)
	if err != nil {
		log.Printf("unable to stat %q: %v", fullName, err)
		return false
	}
	if fi.IsDir() {
		return false
	}
	a, err := os.ReadFile(fullName)
	if err != nil {
		log.Printf("error reading %v: %v", name, err)
		return false
	}
	author := authorRe.FindSubmatch(a)
	if len(author) < 2 {
		author = [][]byte{[]byte(""), []byte("unknown")}
	}
	m := publishedRe.FindSubmatch(a)
	if len(m) < 1 {
		m = [][]byte{[]byte(""), []byte("")}
	}
	title := titleRe.FindSubmatch(a)
	if len(title) < 2 {
		title = [][]byte{[]byte(""), []byte(strings.TrimSuffix(name, ".md"))}
	}
	t, err := time.Parse(timeFormat, string(m[1]))
	if err != nil {
		t = time.Time{}
	}
	idx.Lock()
	defer idx.Unlock()
	// TODO: add title from regexp
	idx.metaData[name] = postMetadata{
		modified:  fi.ModTime(),
		published: t,
		author:    string(author[1]),
		title:     strings.TrimSuffix(string(title[1]), "\r"),
		url:       strings.TrimSuffix(name, ".md"),
	}
	log.Printf("idx: added %q (%v)", name, idx.metaData[name].title)
	// addPost() requires sequencing by calling pi.sequence, rename and delete do not
	return true
}

func (idx *postIndex) update(name string) {
	idx.delete(name)
	idx.add(name)
	idx.sequence()
}

func (idx *postIndex) rename(old, new string) {
	idx.Lock()
	defer idx.Unlock()
	for n, p := range idx.pubSorted {
		if p != old {
			continue
		}
		idx.pubSorted[n] = new
	}
	idx.metaData[new] = idx.metaData[old]
	delete(idx.metaData, old)
	log.Printf("idx: rename %q to %q, new index: %+v", old, new, idx.pubSorted)
}

func (pi *postIndex) delete(name string) {
	pi.Lock()
	defer pi.Unlock()
	seq := []string{}
	for _, s := range pi.pubSorted {
		if s == name {
			continue
		}
		seq = append(seq, s)
	}
	pi.pubSorted = seq
	delete(pi.metaData, name)
	log.Printf("idx: deleted post %v, new index: %+v", name, pi.pubSorted)
}

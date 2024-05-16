package main

import (
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
)

type textSearch struct {
	index bleve.Index

	sync.Mutex
}

func (t *textSearch) rescan() {
	var err error
	start := time.Now()
	t.Lock()
	t.index, err = bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		log.Fatal(err)
	}
	t.Unlock()
	dir, err := os.ReadDir(path.Join(*rootDir, *postsDir))
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range dir {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		t.add(f.Name())
	}
	log.Printf("txt: scan done in %v", time.Since(start))
}

func (t *textSearch) add(file string) {
	file = path.Base(unescapeOrEmpty(file))
	if file == "" {
		return
	}
	b, err := os.ReadFile(path.Join(*rootDir, *postsDir, file))
	if err != nil {
		return
	}
	t.Lock()
	defer t.Unlock()
	t.index.Index(file, string(b))
	log.Printf("txt: indexed %q", file)
}

func (t *textSearch) delete(file string) {
	t.Lock()
	defer t.Unlock()
	t.index.Delete(path.Base(unescapeOrEmpty(file)))
}

func (t *textSearch) rename(old, new string) {
	t.delete(old)
	t.add(new)
}

func (t *textSearch) search(query string) []string {
	if query == "" {
		return nil
	}
	t.Lock()
	defer t.Unlock()
	res, err := t.index.Search(bleve.NewSearchRequest(bleve.NewMatchQuery(query)))
	if err != nil {
		return nil
	}
	names := []string{}
	for _, hit := range res.Hits {
		names = append(names, hit.ID)
	}
	return names
}

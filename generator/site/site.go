package site

import (
	"cmp"
	"fmt"
	"html/template"
	"slices"
	"time"

	"flo.znkr.io/generator/directives"
)

// Site is an in-memory representation of the to be generated site.
type Site struct {
	templates *template.Template
	docs      map[string]Doc
}

// Doc is a single document of the site, that is anything that can be served as a static file.
type Doc struct {
	path            string
	dir             string // directory of the file, used as a relative directory to load related files
	mime            string
	meta            *Metadata
	directives      []directives.Directive
	data            []byte
	contentRenderer renderer
	pageRenderer    renderer
}

type Metadata struct {
	Title     string
	Published time.Time
	Updated   time.Time
	Abstract  string
	GoImport  string
	Redirect  string
	Template  string
	Article   bool
}

// Doc returns the document for the given path, or nil if the document cannot be found.
func (s *Site) Doc(path string) *Doc {
	d, ok := s.docs[path]
	if !ok {
		return nil
	}
	return &d
}

func (s *Site) Articles() []*Doc {
	var ret []*Doc
	for _, d := range s.docs {
		if d.meta == nil || !d.meta.Article {
			continue
		}
		ret = append(ret, &d)
	}
	slices.SortFunc(ret, func(a, b *Doc) int {
		return b.meta.Published.Compare(a.meta.Published)
	})
	return ret
}

func (s *Site) AllDocs() []*Doc {
	var ret []*Doc
	for _, d := range s.docs {
		ret = append(ret, &d)
	}
	slices.SortFunc(ret, func(a, b *Doc) int {
		return cmp.Compare(a.Path(), b.Path())
	})
	return ret
}

func (s *Site) RenderContent(d *Doc) ([]byte, error) {
	b, err := d.contentRenderer.render(s, d, d.data)
	if err != nil {
		return nil, fmt.Errorf("rendering content of %s: %v", d.path, err)
	}
	return b, nil
}

// RenderPage renders doc as a page.
func (s *Site) RenderPage(d *Doc) ([]byte, error) {
	b, err := d.pageRenderer.render(s, d, d.data)
	if err != nil {
		return nil, fmt.Errorf("rendering page for %s: %v", d.path, err)
	}
	return b, nil
}

func (d *Doc) MimeType() string { return d.mime }
func (d *Doc) Meta() *Metadata  { return d.meta }
func (d *Doc) Path() string     { return d.path }
func (d *Doc) Dir() string      { return d.dir }

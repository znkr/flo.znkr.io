package site

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

// Site is an in-memory representation of the to be generated site.
type Site struct {
	docs map[string]Doc
}

// Doc is a single document of the site, that is anything that can be served as a static file.
type Doc struct {
	Path     string
	Source   string
	MimeType string
	Meta     *Metadata
	Data     []byte
	Renderer Renderer
}

type Renderer interface {
	RenderContent(s *Site, doc *Doc, data []byte) ([]byte, error)
	RenderPage(s *Site, doc *Doc, data []byte) ([]byte, error)
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

// New creates a new site from the provided docs.
//
// If there are multiple docs for the same path, New returns an error.
func New(docs []Doc) (*Site, error) {
	s := &Site{
		docs: make(map[string]Doc),
	}
	for _, d := range docs {
		if _, exists := s.docs[d.Path]; exists {
			return nil, fmt.Errorf("duplicate doc for path %q", d.Path)
		}
		s.docs[d.Path] = d
	}
	return s, nil
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
		if d.Meta == nil || !d.Meta.Article {
			continue
		}
		ret = append(ret, &d)
	}
	slices.SortFunc(ret, func(a, b *Doc) int {
		return b.Meta.Published.Compare(a.Meta.Published)
	})
	return ret
}

func (s *Site) AllDocs() []*Doc {
	var ret []*Doc
	for _, d := range s.docs {
		ret = append(ret, &d)
	}
	slices.SortFunc(ret, func(a, b *Doc) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return ret
}

func (s *Site) RenderContent(d *Doc) ([]byte, error) {
	b, err := d.Renderer.RenderContent(s, d, d.Data)
	if err != nil {
		return nil, fmt.Errorf("rendering content of %s: %v", d.Path, err)
	}
	return b, nil
}

// RenderPage renders doc as a page.
func (s *Site) RenderPage(d *Doc) ([]byte, error) {
	b, err := d.Renderer.RenderPage(s, d, d.Data)
	if err != nil {
		return nil, fmt.Errorf("rendering page for %s: %v", d.Path, err)
	}
	return b, nil
}

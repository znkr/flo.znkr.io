package site

import (
	"cmp"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not found")

type Site struct {
	docs map[string]Doc
}

type Doc struct {
	path            string
	mime            string
	meta            *Metadata
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

var passthrough = &passthroughRenderer{}

type renderer interface {
	render(meta *Metadata, data []byte) ([]byte, error)
}

func Load(dir string) (*Site, error) {
	s := &Site{
		docs: make(map[string]Doc),
	}

	s.docs["/feed.atom"] = Doc{
		path: "/feed.atom",
		mime: "application/atom+xml;charset=utf-8",
		meta: &Metadata{
			Title: "flo.znkr.io",
		},
		contentRenderer: passthrough,
		pageRenderer: &feedRenderer{
			site: s,
		},
	}

	templates, err := loadTemplates(dir)
	if err != nil {
		return nil, fmt.Errorf("loading templates: %v", err)
	}

	sitedir := filepath.Join(dir, "site")
	err = filepath.WalkDir(sitedir, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		doc := Doc{
			contentRenderer: passthrough,
			pageRenderer:    passthrough,
		}

		meta, data, err := readFile(fpath)
		if err != nil {
			return fmt.Errorf("reading file: %v", err)
		}
		doc.meta = meta
		doc.data = data

		path := strings.TrimPrefix(fpath, sitedir)
		dir, base := filepath.Split(path)
		ext := filepath.Ext(base)

		switch ext {
		case ".html", ".md":
			if p := strings.TrimSuffix(base, ext); p == "index" {
				if dir == "/" {
					path = dir
				} else {
					path = dir[:len(dir)-1]
				}
			} else {
				path = dir + p
			}

			if doc.meta != nil {
				tname := cmp.Or(doc.meta.Template, "article")
				if tname != "" {
					t := templates.Lookup(tname)
					if t == nil {
						return fmt.Errorf("template not found %s", tname)
					}
					doc.pageRenderer = &templateRenderer{
						site:     s,
						template: t,
					}
				}
			}
		}

		switch ext {
		case ".md":
			doc.mime = "text/html;charset=UTF-8"
			doc.contentRenderer = &markdownRenderer{}
		default:
			doc.mime = mime.TypeByExtension(filepath.Ext(fpath))
		}

		doc.path = path
		s.docs[path] = doc
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading site: %v", err)
	}

	return s, nil
}

func loadTemplates(dir string) (*template.Template, error) {
	templateDir := filepath.Join(dir, "templates")
	root := template.New("")
	err := filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || !strings.HasSuffix(path, ".html") || err != nil {
			return err
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		t := root.New(path[len(templateDir)+1 : len(path)-len(".html")])
		if _, err = t.Parse(string(b)); err != nil {
			return err
		}

		return nil
	})
	return root, err
}

func readFile(file string) (*Metadata, []byte, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, fmt.Errorf("reading file: %v", err)
	}

	var meta *Metadata
	meta, data, err = parseMetadata(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing metadata: %v", err)
	}

	return meta, data, err
}

func (s *Site) Get(path string) (Doc, error) {
	c, ok := s.docs[path]
	if !ok {
		return Doc{}, ErrNotFound
	}
	return c, nil
}

func (s *Site) Articles() []Doc {
	var ret []Doc
	for _, d := range s.docs {
		if d.meta == nil || !d.meta.Article {
			continue
		}
		ret = append(ret, d)
	}
	slices.SortFunc(ret, func(a, b Doc) int {
		return b.meta.Published.Compare(a.meta.Published)
	})
	return ret
}

func (s *Site) Docs() []Doc {
	var ret []Doc
	for _, d := range s.docs {
		ret = append(ret, d)
	}
	slices.SortFunc(ret, func(a, b Doc) int {
		return cmp.Compare(a.Path(), b.Path())
	})
	return ret
}

func (d *Doc) MimeType() string { return d.mime }
func (d *Doc) Meta() *Metadata  { return d.meta }
func (d *Doc) Path() string     { return d.path }

func (d *Doc) RenderContent() ([]byte, error) {
	b, err := d.contentRenderer.render(d.meta, d.data)
	if err != nil {
		return nil, fmt.Errorf("rendering content of %s: %v", d.path, err)
	}
	return b, nil
}

func (d *Doc) RenderPage() ([]byte, error) {
	data, err := d.RenderContent()
	if err != nil {
		return nil, err
	}
	b, err := d.pageRenderer.render(d.meta, data)
	if err != nil {
		return nil, fmt.Errorf("rendering page for %s: %v", d.path, err)
	}
	return b, nil
}

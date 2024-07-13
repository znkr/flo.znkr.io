package site

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"flo.znkr.io/generator/directives"
)

// ErrNotFound is returned when documents are requested that don't exist.
var ErrNotFound = errors.New("not found")

// Site is an in-memory representation of the to be generated site.
type Site struct {
	docs map[string]Doc
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

var passthrough = &passthroughRenderer{}

type renderer interface {
	render(meta *Metadata, data []byte) ([]byte, error)
}

// Load loads a site from the directory dir.
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
			dir:             filepath.Dir(fpath),
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

	if strings.HasSuffix(file, ".md") {
		var meta *Metadata
		meta, data, err = parseMetadata(data)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing metadata: %v", err)
		}

		return meta, data, err
	}
	return nil, data, err
}

// Get returns the document for the given path, or ErrNotFound if the document cannot be found.
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

	dirs, err := directives.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directives for %s: %v", d.path, err)
	}

	for _, dir := range dirs {
		var err error
		b, err = d.applyDirective(b, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to apply directives for %s: %v", d.path, err)
		}
	}

	return b, nil
}

// RenderPage renders doc as a page.
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

func (d *Doc) applyDirective(in []byte, dir directives.Directive) ([]byte, error) {
	switch dir.Name {
	case "meta":
		return in, nil
	case "include-snippet":
		file := dir.Attrs["file"]
		if file == "" {
			return nil, fmt.Errorf("inline-snipped: missing or empty file attribute")
		}
		b, err := os.ReadFile(filepath.Join(d.dir, file))
		if err != nil {
			return nil, fmt.Errorf("inline-snipped: %v", err)
		}
		var buf bytes.Buffer
		buf.Write(in[:dir.Pos])
		buf.WriteString("<pre><code>")
		buf.WriteString(html.EscapeString(string(b)))
		buf.WriteString("</code></pre>")
		buf.Write(in[dir.End:])
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unknown directive: %s", dir.Name)
	}
}

func parseMetadata(in []byte) (*Metadata, []byte, error) {
	meta := Metadata{}

	// Take title from first header. This assumes that every document starts with the header
	// and doesn't have anything before it.
	if len(in) > 2 && in[0] == '#' && in[1] == ' ' {
		eol := slices.Index(in, '\n')
		if eol < 0 {
			return nil, in, nil
		}
		meta.Title = strings.TrimSpace(string(in[1:eol]))
		in = in[eol:]
	}

	metadir, err := directives.ParseFirst(in, "meta")
	switch {
	case err == nil:
		// nothing to do
	case errors.Is(err, directives.ErrNotFound):
		return &meta, in, nil
	default:
		return nil, nil, err
	}

	parseTime := func(key string) (time.Time, error) {
		v, ok := metadir.Attrs[key]
		if !ok {
			return time.Time{}, nil
		}
		t, err := time.ParseInLocation("2006-01-02", v, tz)
		if err != nil {
			return time.Time{}, fmt.Errorf("parsing %s: %v", key, err)
		}
		return t, nil
	}
	published, err := parseTime("published")
	if err != nil {
		return nil, nil, err
	}
	updated, err := parseTime("updated")
	if err != nil {
		return nil, nil, err
	}
	if updated.IsZero() {
		updated = published
	}

	meta.Published = published
	meta.Updated = updated
	meta.Abstract = metadir.Attrs["summary"]
	meta.GoImport = metadir.Attrs["go-import"]
	meta.Redirect = metadir.Attrs["redirect"]
	meta.Template = metadir.Attrs["template"]
	meta.Article = metadir.Attrs["article"] != "false"
	return &meta, in, nil
}

var tz *time.Location

func init() {
	var err error
	tz, err = time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
}

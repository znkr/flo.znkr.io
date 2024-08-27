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

	"flo.znkr.io/generator/directives"
)

// Load loads a site from the directory dir.
func Load(dir string) (*Site, error) {
	templates, err := loadTemplates(filepath.Join(dir, "templates"))
	if err != nil {
		return nil, fmt.Errorf("loading templates: %v", err)
	}

	docs, err := loadDocs(filepath.Join(dir, "site"), templates)
	if err != nil {
		return nil, err
	}

	docs["/feed.atom"] = Doc{
		path: "/feed.atom",
		mime: "application/atom+xml;charset=utf-8",
		meta: &Metadata{
			Title: "flo.znkr.io",
		},
		contentRenderer: &passthroughRenderer{},
		pageRenderer:    &feedRenderer{},
	}

	return &Site{
		docs:      docs,
		templates: templates,
	}, nil
}

func loadTemplates(dir string) (*template.Template, error) {
	root := template.New("")
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || !strings.HasSuffix(path, ".html") || err != nil {
			return err
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		t := root.New(path[len(dir)+1 : len(path)-len(".html")])
		if _, err = t.Parse(string(b)); err != nil {
			return err
		}

		return nil
	})
	return root, err
}

func loadDocs(dir string, templates *template.Template) (map[string]Doc, error) {
	docs := make(map[string]Doc)
	err := filepath.WalkDir(dir, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		doc := Doc{
			dir:             filepath.Dir(fpath),
			contentRenderer: &passthroughRenderer{},
			pageRenderer:    &passthroughRenderer{},
		}

		meta, data, err := readFile(fpath)
		if err != nil {
			return fmt.Errorf("reading file: %v", err)
		}
		doc.meta = meta
		doc.data = data

		path := strings.TrimPrefix(fpath, dir)
		dir, base := filepath.Split(path)
		ext := filepath.Ext(base)

		switch ext {
		case ".md":
			if p := strings.TrimSuffix(base, ext); p == "index" {
				if dir == "/" {
					path = dir
				} else {
					path = dir[:len(dir)-1]
				}
			} else {
				path = dir + p
			}
			doc.mime = "text/html;charset=UTF-8"
			doc.contentRenderer = chain(&markdownRenderer{}, &directivesRenderer{})

			tname := "article"
			if doc.meta != nil {
				tname = cmp.Or(doc.meta.Template, tname)
			}
			t := templates.Lookup(tname)
			if t == nil {
				return fmt.Errorf("template not found %s", tname)
			}
			doc.pageRenderer = chain(doc.contentRenderer, &templateRenderer{t})
		default:
			doc.mime = mime.TypeByExtension(filepath.Ext(fpath))
		}

		doc.path = path
		docs[path] = doc
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading docs: %v", err)
	}
	return docs, nil
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

package main

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

	"flo.znkr.io/generator/atom"
	"flo.znkr.io/generator/directives"
	"flo.znkr.io/generator/goldmark"
	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/site/renderers"
)

// load loads a site from the directory dir.
func load(dir string) (*site.Site, error) {
	templates, err := loadTemplates(filepath.Join(dir, "templates"))
	if err != nil {
		return nil, fmt.Errorf("loading templates: %v", err)
	}

	docs, err := loadDocs(filepath.Join(dir, "site"), templates)
	if err != nil {
		return nil, err
	}

	docs = append(docs, site.Doc{
		Path:     "/feed.atom",
		MimeType: "application/atom+xml;charset=utf-8",
		Meta: &site.Metadata{
			Title: "flo.znkr.io",
		},
		ContentRenderer: nil,
		PageRenderer:    atom.Renderer,
	})

	return site.New(docs)
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

func loadDocs(dir string, templates *template.Template) ([]site.Doc, error) {
	directivesRenderer := directives.NewRenderer(templates)

	templateRenderers := make(map[string]*renderers.TemplateRenderer)
	templateRenderer := func(name string) *renderers.TemplateRenderer {
		if r, ok := templateRenderers[name]; ok {
			return r
		}

		t := templates.Lookup(name)
		if t == nil {
			return nil
		}

		r := renderers.NewTemplateRenderer(t)
		templateRenderers[name] = r
		return r
	}

	var docs []site.Doc
	err := filepath.WalkDir(dir, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		doc := site.Doc{
			Source:          fpath,
			ContentRenderer: renderers.Passthrough,
			PageRenderer:    renderers.Passthrough,
		}

		meta, data, err := readFile(fpath)
		if err != nil {
			return fmt.Errorf("reading file: %v", err)
		}
		doc.Meta = meta
		doc.Data = data

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
			doc.MimeType = "text/html;charset=UTF-8"
			doc.ContentRenderer = renderers.Chain(goldmark.Renderer, directivesRenderer)

			tname := "article"
			if doc.Meta != nil {
				tname = cmp.Or(doc.Meta.Template, tname)
			}
			r := templateRenderer(tname)
			if r == nil {
				return fmt.Errorf("template not found %s", tname)
			}
			doc.PageRenderer = renderers.Chain(doc.ContentRenderer, r)
		default:
			doc.MimeType = mime.TypeByExtension(filepath.Ext(fpath))
		}

		doc.Path = path
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading docs: %v", err)
	}
	return docs, nil
}

func readFile(file string) (*site.Metadata, []byte, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, fmt.Errorf("reading file: %v", err)
	}

	if strings.HasSuffix(file, ".md") {
		var meta *site.Metadata
		meta, data, err = parseMetadata(data)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing metadata: %v", err)
		}

		return meta, data, err
	}
	return nil, data, err
}

func parseMetadata(in []byte) (*site.Metadata, []byte, error) {
	meta := site.Metadata{}

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

package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"flo.znkr.io/generator/metadata"
	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
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

	docs = append(docs,
		site.Doc{
			Path:     "/",
			MimeType: "text/html;charset=utf-8",
			Meta: &site.Metadata{
				Title:    "Florian Zenker's website",
				GoImport: "flo.znkr.io git https://github.com/znkr/flo.znkr.io",
			},
			Renderer: mustNewIndexRenderer(templates),
		},
		site.Doc{
			Path:     "/feed.atom",
			MimeType: "application/atom+xml;charset=utf-8",
			Meta: &site.Metadata{
				Title: "Florian Zenker's website",
			},
			Renderer: renderers.Atom,
		},
	)

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
	markdownRenderers := make(map[string]*renderers.MarkdownRenderer)
	for _, typ := range []string{"article", "page"} {
		r, err := renderers.NewMarkdownRenderer(templates, renderers.MarkdownRendererOptions{
			PageTemplate: typ,
		})
		if err != nil {
			return nil, err
		}
		markdownRenderers[typ] = r
	}

	var docs []site.Doc
	err := filepath.WalkDir(dir, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if _, err := os.Stat(filepath.Join(fpath, ".ignore")); err == nil {
				return fs.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(fpath, ".") {
			return nil // skip hidden files
		}

		doc := site.Doc{
			Source:   fpath,
			Renderer: renderers.Passthrough,
		}

		data, err := os.ReadFile(fpath)
		if err != nil {
			return fmt.Errorf("reading file: %v", err)
		}
		doc.Data = data

		path := strings.TrimPrefix(fpath, dir)
		dir, base := filepath.Split(path)
		ext := filepath.Ext(base)

		switch ext {
		case ".md":
			doc.Meta, doc.Data, err = metadata.Parse(data)
			if err != nil {
				return fmt.Errorf("parsing metadata: %v", err)
			}

			if p := strings.TrimSuffix(base, ext); p == "index" {
				if dir == "/" {
					path = dir
				} else {
					path = dir[:len(dir)-1]
				}
			} else {
				path = dir + p
			}
			doc.MimeType = "text/html;charset=utf-8"
			doc.Renderer = markdownRenderers[doc.Meta.Type]
			if doc.Renderer == nil {
				return fmt.Errorf("unknown doc type: %s", doc.Meta.Type)
			}
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

func mustNewIndexRenderer(templates *template.Template) *renderers.MarkdownRenderer {
	r, err := renderers.NewMarkdownRenderer(templates, renderers.MarkdownRendererOptions{
		PageTemplate: "index",
	})
	if err != nil {
		log.Fatalf("creating index renderer: %v", err)
	}
	return r
}

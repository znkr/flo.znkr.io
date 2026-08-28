package main

import (
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/html"
)

// load loads a site from the directory dir.
func load(dir string) (*site.Site, error) {
	templates, err := loadTemplates(filepath.Join(dir, "templates"))
	if err != nil {
		return nil, fmt.Errorf("loading templates: %v", err)
	}

	libs, err := loadLibraries(dir)
	if err != nil {
		return nil, err
	}

	docs, err := loadDocs(dir, templates, libs)
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

// loadLibraries compiles every .mst file in root/lib, whose bindings every
// markst document on the site can then use without importing anything. They
// are compiled once and shared: the functions they define run against
// whichever document is being compiled, so there is nothing per-document to
// redo.
//
// Files are compiled in name order, and each one can use what the files before
// it defined. Where two define the same name the later file wins, and a
// document's own binding wins over both.
func loadLibraries(root string) ([]*markst.Library, error) {
	dir := filepath.Join(root, "lib")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading library directory: %v", err)
	}

	var libs []*markst.Library
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".mst" {
			continue
		}
		// The path is relative to the working directory, so diagnostics about
		// a library — which surface while some *document* is being compiled —
		// can be opened straight from an editor.
		path := filepath.Join("lib", e.Name())
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, fmt.Errorf("reading library: %v", err)
		}
		lib, err := markst.LoadLibrary(path, data, libs)
		if err != nil {
			return nil, err
		}
		libs = append(libs, lib)
	}
	return libs, nil
}

// loadDocs loads all documents below root/site. Errors are reported with a
// path relative to root, which is the working directory, so that they can be
// opened directly in an editor.
func loadDocs(root string, templates *template.Template, libs []*markst.Library) ([]site.Doc, error) {
	markstRenderers := make(map[string]*html.Renderer)
	for _, typ := range []string{"article", "page"} {
		wr, err := html.NewRenderer(templates, html.Options{
			PageTemplate: typ,
		})
		if err != nil {
			return nil, err
		}
		markstRenderers[typ] = wr
	}

	dir := filepath.Join(root, "site")

	ignores := newIgnoreRules(dir)

	var docs []site.Doc
	err := filepath.WalkDir(dir, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if fpath != dir && ignores.match(fpath) {
				return fs.SkipDir
			}
			return ignores.load(fpath)
		}

		if strings.HasPrefix(d.Name(), ".") {
			return nil // skip hidden files
		}

		if ignores.match(fpath) {
			return nil
		}

		doc := site.Doc{
			Source:   fpath,
			Renderer: renderers.Passthrough,
		}

		// Path relative to the working directory, used for error messages.
		rpath, rerr := filepath.Rel(root, fpath)
		if rerr != nil {
			rpath = fpath
		}

		data, err := os.ReadFile(fpath)
		if err != nil {
			return fmt.Errorf("reading file: %v", err)
		}
		doc.Data = data

		path := strings.TrimPrefix(fpath, dir)
		dir, base := filepath.Split(path)
		ext := filepath.Ext(base)
		if ext == ".mst" {
			if p := strings.TrimSuffix(base, ext); p == "index" {
				if dir == "/" {
					path = dir
				} else {
					path = dir[:len(dir)-1]
				}
			} else {
				path = dir + p
			}
		}

		switch ext {
		case ".mst":
			meta, rd, err := markst.Load(rpath, filepath.Dir(fpath), path, data, libs)
			if err != nil {
				// The error already names the file and the position within it.
				return err
			}
			doc.Meta = meta
			doc.RenderData = rd
			doc.MimeType = "text/html;charset=utf-8"
			doc.Renderer = markstRenderers[doc.Meta.Type]
			if doc.Renderer == nil {
				return fmt.Errorf("%s: unknown doc type: %s", rpath, doc.Meta.Type)
			}
		default:
			doc.MimeType = mime.TypeByExtension(filepath.Ext(fpath))
		}

		doc.Path = path
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func mustNewIndexRenderer(templates *template.Template) site.Renderer {
	r, err := renderers.NewIndexRenderer(templates)
	if err != nil {
		log.Fatalf("creating index renderer: %v", err)
	}
	return r
}

// ignoreRules holds the glob patterns from the .ignore files seen so far,
// keyed by the directory each file lives in.
//
// A pattern is matched against the path relative to that directory, so an
// .ignore may exclude entries in its subdirectories as well. Ignoring a
// directory ignores everything below it.
type ignoreRules struct {
	root     string
	patterns map[string][]string
}

func newIgnoreRules(root string) *ignoreRules {
	return &ignoreRules{root: root, patterns: make(map[string][]string)}
}

// load reads the .ignore file in dir, if there is one. Blank lines and lines
// starting with # are ignored.
func (ir *ignoreRules) load(dir string) error {
	name := filepath.Join(dir, ".ignore")
	b, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("reading ignore file: %v", err)
	}

	var patterns []string
	for line := range strings.Lines(string(b)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := path.Match(line, ""); err != nil {
			return fmt.Errorf("%s: invalid pattern %q: %v", name, line, err)
		}
		patterns = append(patterns, line)
	}
	ir.patterns[dir] = patterns
	return nil
}

// match reports whether fpath is excluded by an .ignore file in its own
// directory or in any parent directory up to the site root.
func (ir *ignoreRules) match(fpath string) bool {
	for dir := filepath.Dir(fpath); ; dir = filepath.Dir(dir) {
		if len(ir.patterns[dir]) > 0 {
			rel, err := filepath.Rel(dir, fpath)
			if err != nil {
				return false
			}
			for _, pat := range ir.patterns[dir] {
				if ok, _ := path.Match(pat, filepath.ToSlash(rel)); ok {
					return true
				}
			}
		}
		if dir == ir.root || dir == filepath.Dir(dir) {
			return false
		}
	}
}

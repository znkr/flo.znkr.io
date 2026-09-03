package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"mime"
	"path"
	"strings"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/markst/html"
	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/source"
)

// cacheSize is how many computed values are held.
//
// It counts items, which is a proxy for the two costs that matter: what a value
// takes to hold and what it takes to compute again. One build of this site
// leaves 151 values behind, about 700KB of them bytes, and the spread between
// the largest and the average is around twentyfold. So this is a dozen or so
// builds of history, which is as far back as an edit is worth reusing.
const cacheSize = 2048

const (
	siteTitle    = "Florian Zenker's website"
	siteGoImport = "flo.znkr.io git https://github.com/znkr/flo.znkr.io"
)

// plan declares the build of the site tree and returns it. It computes nothing:
// a document is compiled, highlighted and rendered when the artifact holding it
// is first asked for.
func plan(tree *source.Tree) (*site.Site, build.Artifact[[]markst.Diagnostic], error) {
	templates := build.Derive[*template.Template]("templates", parseTemplates, tree.Sub("templates"))

	var libs []build.Artifact[*markst.Lib]
	for _, f := range tree.Files("lib") {
		if path.Ext(f.Path) != ".mst" {
			continue
		}
		// The path is relative to the working directory, so diagnostics about a
		// library -- which surface while some *document* is being compiled --
		// can be opened straight from an editor.
		libs = append(libs, build.Derive[*markst.Lib]("lib", compileLib, f.Path, tree.Data(f)))
	}

	var docs []site.Doc
	// entries and contents run parallel over the compiled documents: the index
	// reads the first, the feed both.
	var entries []build.Artifact[renderers.Entry]
	var contents []build.Artifact[[]byte]
	var compiled []build.Artifact[*markst.Doc]

	for _, f := range tree.Files("") {
		if !f.Doc {
			continue
		}

		p := strings.TrimPrefix(f.Path, "site")
		dir, base := path.Split(p)
		ext := path.Ext(base)

		if ext != ".mst" {
			// The path decides both where the document is served and, through
			// its extension, as what. The bytes are the whole of the document.
			data := tree.Data(f)
			docs = append(docs, site.Doc{
				Path:     p,
				Source:   f.Path,
				MimeType: mimeType(ext),
				Meta:     noMeta(),
				Page:     data,
				Content:  data,
			})
			continue
		}

		// The path decides where the document is served. This is based on the
		// file name:
		// - index.mst is served at the directory's path, so that a directory can
		//   hold a document that is the default for it.
		// - other files are served at the files path without the extension.
		if name := strings.TrimSuffix(base, ext); name == "index" {
			if dir == "/" {
				p = dir
			} else {
				p = strings.TrimSuffix(dir, "/")
			}
		} else {
			p = dir + name
		}

		// docFS is the file system the document can include from. It is the
		// directory a document is in. A document at the site root has no
		// directory of its own, so it can include nothing.
		docFS := tree.Empty()
		docRoot := ""
		if dir != "/" {
			docRoot = dir
			docFS = tree.Sub(path.Join("site", dir))
		}

		mdoc := build.Derive[*markst.Doc]("doc", compileDoc, f.Path, tree.Data(f), docFS, libs)

		// The summaryFrags is not reached by a walk of the body, so rendering it
		// waits on its own fragments (usually none) and not on the article's
		// (often many).
		summaryFrags := deriveFragments("summary fragments", summaryFragments, mdoc)
		summary := build.Derive[string]("summary", html.RenderSummary, mdoc, summaryFrags)
		meta := build.Derive[site.Metadata]("meta", withSummary, mdoc, summary)

		bodyFrags := deriveFragments("body fragments", bodyFragments, mdoc)
		content := build.Derive[[]byte]("content", html.RenderContent, templates, mdoc, bodyFrags, p, docRoot)
		page := build.Derive[[]byte]("page", html.RenderPage, templates, mdoc, meta, bodyFrags, p, docRoot)

		docs = append(docs, site.Doc{
			Path:     p,
			Source:   f.Path,
			MimeType: "text/html;charset=utf-8",
			Meta:     meta,
			Page:     page,
			Content:  content,
		})
		entries = append(entries, build.Derive[renderers.Entry]("entry", entryOf, p, meta))
		contents = append(contents, content)
		compiled = append(compiled, mdoc)
	}

	index := build.Derive[[]byte]("index", renderers.RenderIndex, templates, siteTitle, siteGoImport, entries)
	feed := build.Derive[[]byte]("feed", renderers.RenderAtom, siteTitle, entries, contents)

	docs = append(docs,
		site.Doc{
			Path:     "/",
			MimeType: "text/html;charset=utf-8",
			Meta:     constMeta(siteTitle, siteGoImport),
			Page:     index,
			Content:  index,
		},
		site.Doc{
			Path:     "/feed.atom",
			MimeType: "application/atom+xml;charset=utf-8",
			Meta:     constMeta(siteTitle, ""),
			Page:     feed,
			Content:  feed,
		},
	)

	// Every warning the site produces, in one artifact, so that they can be
	// reported together however few of them a build had to recompute.
	diags := build.Derive[[]markst.Diagnostic]("diags", collectDiags, libs, compiled)

	s, err := site.New(docs)
	return s, diags, err
}

// collectDiags gathers the warnings compiling the site produced, libraries
// first.
func collectDiags(libs []*markst.Lib, docs []*markst.Doc) ([]markst.Diagnostic, error) {
	var ret []markst.Diagnostic
	for _, l := range libs {
		ret = append(ret, l.Diags...)
	}
	for _, d := range docs {
		ret = append(ret, d.Diags...)
	}
	return ret, nil
}

// loadDir scans dir, plans the site it holds, and returns it with a cache to
// compute it against, reporting whatever compiling it warns about.
func loadDir(ctx context.Context, dir string) (*build.Cache, *site.Site, error) {
	tree, err := source.Scan(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("scanning %s: %v", dir, err)
	}
	s, diags, err := plan(tree)
	if err != nil {
		return nil, nil, err
	}
	c := build.NewCache(cacheSize)
	ds, err := diags.Get(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	markst.Report(ds)
	return c, s, nil
}

// deriveFragments declares the highlighting a document needs, one artifact per
// fragment, so that an edit costs only the deriveFragments it changed.
func deriveFragments(kind string, walk any, doc build.Artifact[*markst.Doc]) build.Artifact[map[build.Key]builtins.Fragment] {
	as := build.Derive[[]build.Artifact[builtins.Fragment]](kind, walk, doc)
	return build.Collect[builtins.Fragment]("fragments", as)
}

func summaryFragments(d *markst.Doc) ([]build.Artifact[builtins.Fragment], error) {
	return builtins.Artifacts(d.Summary)
}

func bodyFragments(d *markst.Doc) ([]build.Artifact[builtins.Fragment], error) {
	return builtins.Artifacts(d.Doc)
}

// withSummary returns the document's metadata with its summary rendered in.
func withSummary(d *markst.Doc, summary string) (site.Metadata, error) {
	m := d.Meta
	m.Summary = summary
	return m, nil
}

func entryOf(p string, m site.Metadata) (renderers.Entry, error) {
	return renderers.Entry{Path: p, Meta: m}, nil
}

// noMeta returns the metadata of a document that says nothing about itself.
func noMeta() build.Artifact[site.Metadata] {
	return build.Const("meta.none", site.Metadata{})
}

// constMeta returns the metadata of a document the generator writes rather than
// reads. The index and the feed say what they say here rather than in a
// document, so these two fields are the whole of what tells one from the other.
func constMeta(title, goImport string) build.Artifact[site.Metadata] {
	return build.Const("meta.const", site.Metadata{Title: title, GoImport: goImport})
}

// mimeType reports how a file with this extension is served.
func mimeType(ext string) string {
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	// What mime.TypeByExtension knows depends on the MIME database of the
	// machine the site is built on, and that database has nothing to say about
	// the sources articles link to (.go, .diff, .mod, ...). Serving those as
	// plain text is both what they are and the same everywhere.
	return "text/plain;charset=utf-8"
}

// The two compile rules report nothing: a warning is part of what compiling
// produced, and is carried in the value so that it can be reported again for a
// document that did not have to be compiled again. See collectDiags.
//
// The context is the one the caller that asked for the document passed to
// [build.Artifact.Get]. It bounds the compile without being part of the key.
func compileDoc(ctx context.Context, p string, data []byte, docFS fs.FS, libs []*markst.Lib) (*markst.Doc, error) {
	// A compile error already names the file and the position within it.
	return markst.Load(ctx, p, data, docFS, libs)
}

// compileLib compiles one library on its own: a library may use nothing
// another library defines, so that each is an artifact of its own file and an
// edit to one recompiles only that one.
func compileLib(ctx context.Context, p string, data []byte) (*markst.Lib, error) {
	return markst.LoadLibrary(ctx, p, data, nil)
}

// parseTemplates parses every .html file in fsys as a template named by its
// path within it, without the extension.
func parseTemplates(fsys fs.FS) (*template.Template, error) {
	root := template.New("")
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		_, err = root.New(strings.TrimSuffix(p, ".html")).Parse(string(b))
		return err
	})
	if err != nil {
		return nil, err
	}
	return root, nil
}

package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"mime"
	"path"
	"slices"
	"strings"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/card"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/builtins"
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
	siteSummary  = "Thoughts and notes about things that interest me, mostly programming."
	siteGoImport = "flo.znkr.io git https://github.com/znkr/flo.znkr.io"
	// siteOrigin is the site's origin, without a trailing slash: every URL of
	// the site is it and a path.
	siteOrigin = "https://flo.znkr.io"
)

// robotsTxt is what a crawler is told: everything may be crawled, and the
// sitemap is here. A draft is kept out of search by the noindex in its own
// head, which a crawler has to fetch the document to see.
const robotsTxt = "User-agent: *\n" +
	"Allow: /\n" +
	"\n" +
	"Sitemap: " + siteOrigin + "/sitemap.xml\n"

// plan declares the build of the site tree and returns it. It computes nothing:
// a document is compiled, highlighted and rendered when the artifact holding it
// is first asked for.
func plan(tree *source.Tree) (*site.Site, build.Artifact[[]markst.Diagnostic], error) {
	templates := build.Derive[*template.Template]("templates", parseTemplates, tree.Sub("templates"))

	// A page loads a file in _assets by a URL that carries the key of the
	// file's artifact, which is a hash of its content.
	var assets renderers.Assets
	for _, f := range tree.Files("site/_assets") {
		p := strings.TrimPrefix(f.Path, "site")
		assets = append(assets, renderers.Asset{Path: p, URL: p + "?v=" + tree.Data(f).Key().String()[:8]})
	}

	var libs []build.Artifact[*markst.Lib]
	for _, f := range tree.Files("lib") {
		if path.Ext(f.Path) != ".mst" {
			continue
		}
		// The path is relative to the working directory, so diagnostics about a
		// library -- which surface while some *document* is being compiled --
		// can be opened straight from an editor.
		libs = append(libs, build.Derive[*markst.Lib]("lib", markst.LoadLibrary, f.Path, tree.Data(f)))
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
				Path:        p,
				Source:      f.Path,
				MimeType:    mimeType(ext),
				Meta:        noMeta(),
				Page:        data,
				FeedContent: data,
			})
			continue
		}

		// The path decides where the document is served. This is based on the
		// file name:
		// - index.mst is served at the directory's path, so that a directory can
		//   hold a document that is the default for it.
		// - other files are served at the files path without the extension.
		// Either path ends in a slash, because the page is packed as the
		// index.html of a directory and GitHub Pages redirects the path without
		// the slash to the one with it.
		if name := strings.TrimSuffix(base, ext); name == "index" {
			p = dir
		} else {
			p = dir + name + "/"
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

		mdoc := build.Derive[*markst.Doc]("doc", markst.Load, f.Path, tree.Data(f), docFS, libs)

		// The summaryFrags is not reached by a walk of the body, so rendering it
		// waits on its own fragments (usually none) and not on the article's
		// (often many).
		summaryFrags := deriveFragments("summary fragments", (*markst.Doc).SummaryFragments, mdoc)
		summary := build.Derive[string]("summary", markst.RenderSummary, mdoc, summaryFrags)
		meta := build.Derive[site.Metadata]("meta", siteMetadata, f.Path, p, mdoc, summary)

		bodyFrags := deriveFragments("body fragments", (*markst.Doc).Fragments, mdoc)
		// The feed embeds content, so its footnotes are drawn without the
		// marks the page's stylesheet and script make work.
		content := build.Derive[[]byte]("content", renderers.RenderBody, templates, mdoc, bodyFrags, p, docRoot, markst.Endnotes)
		body := build.Derive[[]byte]("body", renderers.RenderBody, templates, mdoc, bodyFrags, p, docRoot, markst.Popovers)
		toc := build.Derive[[]byte]("toc", markst.RenderTOC, mdoc, bodyFrags, p)
		page := build.Derive[[]byte]("page", renderers.RenderPage, templates, meta, assets, body, toc)
		cardImg := build.Derive[[]byte]("card", card.Render, meta)

		docs = append(docs, site.Doc{
			Path:        p,
			Source:      f.Path,
			MimeType:    "text/html;charset=utf-8",
			Meta:        meta,
			Page:        page,
			FeedContent: content,
		})
		docs = append(docs, cardDoc(p, cardImg))
		entries = append(entries, build.Derive[renderers.Entry]("entry", entryOf, p, meta))
		contents = append(contents, content)
		compiled = append(compiled, mdoc)
	}

	indexMeta := constMeta(site.Metadata{
		Title:        siteTitle,
		Summary:      siteSummary,
		GoImport:     siteGoImport,
		CanonicalURL: canonicalURL("/"),
		Image:        cardURL("/"),
		ImageWidth:   card.Width,
		ImageHeight:  card.Height,
	})
	index := build.Derive[[]byte]("index", renderers.RenderIndex, templates, indexMeta, assets, entries)
	feed := build.Derive[[]byte]("feed", renderers.RenderAtom, siteTitle, entries, contents)
	// entries holds the compiled documents, which the index is not one of, and
	// the feed reads it against contents. The sitemap lists the index too.
	indexEntry := build.Derive[renderers.Entry]("entry", entryOf, "/", indexMeta)
	sitemap := build.Derive[[]byte]("sitemap", renderers.RenderSitemap, append(slices.Clone(entries), indexEntry))
	robots := build.Const("robots", []byte(robotsTxt))

	docs = append(docs,
		site.Doc{
			Path:        "/",
			MimeType:    "text/html;charset=utf-8",
			Meta:        indexMeta,
			Page:        index,
			FeedContent: index,
		},
		site.Doc{
			Path:        "/feed.atom",
			MimeType:    "application/atom+xml;charset=utf-8",
			Meta:        constMeta(site.Metadata{Title: siteTitle}),
			Page:        feed,
			FeedContent: feed,
		},
		site.Doc{
			Path:        "/sitemap.xml",
			MimeType:    "application/xml;charset=utf-8",
			Meta:        noMeta(),
			Page:        sitemap,
			FeedContent: sitemap,
		},
		site.Doc{
			Path:        "/robots.txt",
			MimeType:    "text/plain;charset=utf-8",
			Meta:        noMeta(),
			Page:        robots,
			FeedContent: robots,
		},
		cardDoc("/", build.Derive[[]byte]("card", card.Render, indexMeta)),
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

// deriveFragments declares the highlighting walk finds in a document, one
// artifact per fragment, so that an edit costs only the fragments it changed.
func deriveFragments(kind string, walk func(*markst.Doc) ([]build.Artifact[builtins.Fragment], error), doc build.Artifact[*markst.Doc]) build.Artifact[map[build.Key]builtins.Fragment] {
	as := build.Derive[[]build.Artifact[builtins.Fragment]](kind, walk, doc)
	return build.Collect[builtins.Fragment]("fragments", as)
}

// siteMetadata returns what the site knows about the document at source: what
// it says about itself, held to the types there is a page template for, with
// its summary rendered in and its canonical URL.
func siteMetadata(source, path string, d *markst.Doc, summary string) (site.Metadata, error) {
	if !slices.Contains(site.DocTypes, d.Meta.Type) {
		return site.Metadata{}, fmt.Errorf("%s: unknown doc type: %q", source, d.Meta.Type)
	}
	m := site.Metadata{
		Title:     d.Meta.Title,
		Source:    source,
		Type:      d.Meta.Type,
		Published: d.Meta.Published,
		Updated:   d.Meta.Updated,
		// The summary is rendered markup, so it carries the whitespace around
		// the block it is written in. That whitespace is significant in the
		// attribute of a meta tag and in the feed.
		Summary: strings.TrimSpace(summary),
	}

	m.CanonicalURL = canonicalURL(path)
	m.Image = cardURL(path)
	m.ImageWidth, m.ImageHeight = card.Width, card.Height

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
// document.
func constMeta(m site.Metadata) build.Artifact[site.Metadata] {
	return build.Const("meta.const", m)
}

// cardDoc returns the document holding the card of the page at p.
func cardDoc(p string, img build.Artifact[[]byte]) site.Doc {
	return site.Doc{
		Path:        cardPath(p),
		MimeType:    "image/png",
		Meta:        noMeta(),
		Page:        img,
		FeedContent: img,
	}
}

// cardPath returns the path the card of the page at p is served at.
func cardPath(p string) string { return path.Join(p, "card.png") }

// canonicalURL returns the URL the page at p is served from, which is the one
// URL of it a search engine is to keep.
func canonicalURL(p string) string { return siteOrigin + p }

// cardURL returns the URL a link preview loads the card of the page at p from.
func cardURL(p string) string { return siteOrigin + cardPath(p) }

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

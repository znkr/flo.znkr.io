// Package plan declares the site a source tree holds as a build of artifacts.
// Planning computes nothing; a value is computed when its artifact is first
// asked for.
package plan

import (
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

// Options selects what a plan of the site includes. The zero Options plans the
// site as it is published.
type Options struct {
	// Drafts reports whether the index and the feed list the drafts.
	Drafts bool
}

// Plan declares the build of the site tree with the zero [Options] and returns
// it.
func Plan(tree *source.Tree) (*site.Site, build.Artifact[[]markst.Diagnostic], error) {
	return Options{}.Plan(tree)
}

// Plan declares the build of the site tree and returns it. It computes nothing:
// a document is compiled, highlighted and rendered when the artifact holding it
// is first asked for.
func (o Options) Plan(tree *source.Tree) (*site.Site, build.Artifact[[]markst.Diagnostic], error) {
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
	// entries and contents run parallel over the compiled documents: the
	// articles are selected from the first, and the feed reads both.
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
				Errors:      noErrors(),
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

		mdoc := build.Derive[*markst.Doc]("doc", markst.Load, f.Path, tree.Data(f), docFS, libs, site.DocTypes)

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
			Errors:      build.Derive[string]("errors", docErrors, mdoc),
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
	listed := build.Derive[[]renderers.Entry]("articles", articles, entries, o)
	index := build.Derive[[]byte]("index", renderers.RenderIndex, templates, indexMeta, assets, listed)
	feed := build.Derive[[]byte]("feed", renderers.RenderAtom, siteTitle, listed, entries, contents)
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
			Errors:      noErrors(),
			Page:        index,
			FeedContent: index,
		},
		site.Doc{
			Path:        "/feed.atom",
			MimeType:    "application/atom+xml;charset=utf-8",
			Meta:        constMeta(site.Metadata{Title: siteTitle}),
			Errors:      noErrors(),
			Page:        feed,
			FeedContent: feed,
		},
		site.Doc{
			Path:        "/sitemap.xml",
			MimeType:    "application/xml;charset=utf-8",
			Meta:        noMeta(),
			Errors:      noErrors(),
			Page:        sitemap,
			FeedContent: sitemap,
		},
		site.Doc{
			Path:        "/robots.txt",
			MimeType:    "text/plain;charset=utf-8",
			Meta:        noMeta(),
			Errors:      noErrors(),
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

// collectDiags gathers the warnings and errors compiling the site produced, libraries
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

// deriveFragments declares the highlighting walk finds in a document, one
// artifact per fragment, so that an edit costs only the fragments it changed.
func deriveFragments(kind string, walk func(*markst.Doc) ([]build.Artifact[builtins.Fragment], error), doc build.Artifact[*markst.Doc]) build.Artifact[map[build.Key]builtins.Fragment] {
	as := build.Derive[[]build.Artifact[builtins.Fragment]](kind, walk, doc)
	return build.Collect[builtins.Fragment]("fragments", as)
}

// siteMetadata returns what the site knows about the document at source: what
// it says about itself, with its summary rendered in and its canonical URL. A
// document with errors has the type [site.ErrorType] and nothing else.
func siteMetadata(source, path string, d *markst.Doc, summary string) (site.Metadata, error) {
	if d.Err() != nil {
		// A document with errors has no URL, so nothing lists it.
		return site.Metadata{Title: "Error", Source: source, Type: site.ErrorType}, nil
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

// articles returns the articles among entries that o selects: the drafts, if o
// includes them, then the published articles, newest first. Two articles
// published on the same day keep the order they came in, as do two drafts.
func articles(entries []renderers.Entry, o Options) ([]renderers.Entry, error) {
	var ret []renderers.Entry
	for _, e := range entries {
		if e.Meta.Type != "article" || (e.Meta.Draft() && !o.Drafts) {
			continue
		}
		ret = append(ret, e)
	}
	slices.SortStableFunc(ret, func(a, b renderers.Entry) int {
		// A draft has no publication date and sorts ahead of every date.
		switch ad, bd := a.Meta.Draft(), b.Meta.Draft(); {
		case ad && !bd:
			return -1
		case bd && !ad:
			return 1
		}
		return b.Meta.Published.Compare(a.Meta.Published)
	})
	return ret, nil
}

func entryOf(p string, m site.Metadata) (renderers.Entry, error) {
	return renderers.Entry{Path: p, Meta: m}, nil
}

// noMeta returns the metadata of a document that says nothing about itself.
func noMeta() build.Artifact[site.Metadata] {
	return build.Const("meta.none", site.Metadata{})
}

// noErrors returns the errors of a document that cannot have any.
func noErrors() build.Artifact[string] {
	return build.Const("errors.none", "")
}

// docErrors returns the errors of d, one per line, or "" if there are none.
func docErrors(d *markst.Doc) (string, error) {
	if err := d.Err(); err != nil {
		return err.Error(), nil
	}
	return "", nil
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
		Errors:      noErrors(),
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

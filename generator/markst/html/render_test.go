package html_test

import (
	"html/template"
	"os"
	"strings"
	"testing"

	"flo.znkr.io/generator/build"
	gmarkst "flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/markst/builtins"
	"flo.znkr.io/generator/markst/html"
	"flo.znkr.io/generator/site"
	"znkr.io/markst"
	"znkr.io/markst/value"
)

// render compiles src against the site's own markst library and renders it the
// way a page body is rendered. The real lib/lib.mst is used rather than a stub:
// what is being checked is that the admonitions the site actually ships produce
// the markup its stylesheet is written against.
func render(t *testing.T, src string) string {
	t.Helper()

	templates := template.New("")
	for _, name := range []string{"article", "fragments/include_snippet", "fragments/include_diff"} {
		template.Must(templates.New(name).Parse(""))
	}

	var libs []*gmarkst.Lib
	for _, name := range []string{"lib", "admonition", "math"} {
		file := name + ".mst"
		path := "../../../lib/" + file
		libSrc, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		lib, err := gmarkst.LoadLibrary(t.Context(), file, libSrc, nil)
		if err != nil {
			t.Fatalf("compiling %s: %v", file, err)
		}
		if len(lib.Diags) > 0 {
			t.Errorf("%s has diagnostics:\n%s", file, formatDiags(lib.Diags))
		}
		libs = append(libs, lib)
	}

	src = "#article(title: \"T\")\n\n" + src
	doc, err := gmarkst.Load(t.Context(), "test.mst", []byte(src), nil, libs)
	if err != nil {
		t.Fatalf("compiling document: %v", err)
	}
	if len(doc.Diags) > 0 {
		t.Errorf("document has diagnostics:\n%s", formatDiags(doc.Diags))
	}

	out, err := html.RenderContent(templates, doc, resolveAll(t, doc.Doc), "/test", "")
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	return string(out)
}

// resolveAll does the highlighting the build would have done before rendering.
func resolveAll(t *testing.T, c value.Content) map[build.Key]builtins.Fragment {
	t.Helper()
	arts, err := builtins.Artifacts(c)
	if err != nil {
		t.Fatalf("Artifacts() = %v", err)
	}
	cache := build.NewCache(0)
	frags := make(map[build.Key]builtins.Fragment, len(arts))
	for _, a := range arts {
		f, err := a.Get(t.Context(), cache)
		if err != nil {
			t.Fatalf("Get() = %v", err)
		}
		frags[a.Key()] = f
	}
	return frags
}

func formatDiags(diags []markst.Diagnostic) string {
	var sb strings.Builder
	markst.FormatDiagnostics(&sb, diags)
	return sb.String()
}

func TestAdmonitions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{{
		name: "note_default_title",
		in:   "#note[Body.]",
		want: `<div class="admonition note"><p class="admonition-title">Note</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		name: "tip_default_title",
		in:   "#tip[Body.]",
		want: `<div class="admonition tip"><p class="admonition-title">Tip</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		name: "note_with_title",
		in:   `#note(title: "Beware")[Body.]`,
		want: `<div class="admonition note"><p class="admonition-title">Beware</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		// The title is content, so it is escaped like any other text.
		name: "title_is_escaped",
		in:   `#note(title: "a < b & c")[Body.]`,
		want: `<div class="admonition note"><p class="admonition-title">a &lt; b &amp; c</p>` + "\n" +
			`<p>Body.</p>` + "\n</div>\n",
	}, {
		// The body is realized content: markup inside it still works, and it
		// is not double-wrapped in a paragraph.
		name: "body_keeps_its_markup",
		in:   `#note[Body with *strong* and a #link("https://example.com")[link].]`,
		want: `<div class="admonition note"><p class="admonition-title">Note</p>` + "\n" +
			`<p>Body with <strong>strong</strong> and a <a href="https://example.com">link</a>.</p>` + "\n</div>\n",
	}, {
		// A flow body groups into paragraphs, so a blank line makes two — the
		// thing a leaf element could never do.
		name: "multiple_paragraphs",
		in:   "#note[First.\n\n  Second.]",
		want: `<div class="admonition note"><p class="admonition-title">Note</p>` + "\n" +
			`<p>First.</p>` + "\n<p>Second.</p>" + "\n</div>\n",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(t, tt.in); got != tt.want {
				t.Errorf("render(%q):\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// The rendering itself is znkr.io/markst/html's, and tested there. What is
// left here is the part this site adds to it: the anchor link on a heading, the
// highlighting raw text gets, and the table of contents.

func TestHeadingAnchorLink(t *testing.T) {
	// <h1> is the article title, so the document's own headings start at <h2>,
	// and each carries a link to itself for the stylesheet to reveal on hover.
	got := render(t, "= Intro <intro>\n\n== Deeper\n")
	want := "<h2 id=\"intro\">Intro<a href=\"#intro\" class=\"anchor-link\"></a></h2>\n" +
		"<h3 id=\"deeper\">Deeper<a href=\"#deeper\" class=\"anchor-link\"></a></h3>\n"
	if got != want {
		t.Errorf("renderBody():\n got: %q\nwant: %q", got, want)
	}
}

func TestRawIsHighlighted(t *testing.T) {
	// A language chroma knows is marked up with this site's own classes; one it
	// does not know falls back to unstyled text.
	got := render(t, "```go\nfunc f() {}\n```")
	if !strings.Contains(got, "<pre><code>") {
		t.Errorf("render() = %q, want a highlighted code block", got)
	}
	if !strings.Contains(got, "hl-") {
		t.Errorf("render() = %q, want this site's highlight classes", got)
	}
	if plain := render(t, "```\nfunc f() {}\n```"); plain != "<pre><code>func f() {}</code></pre>\n" {
		t.Errorf("render() without a language = %q, want it unstyled", plain)
	}
}

func TestReferenceToAHeading(t *testing.T) {
	got := render(t, "= Intro <intro>\n\nSee @intro.")
	want := "<h2 id=\"intro\">Intro<a href=\"#intro\" class=\"anchor-link\"></a></h2>\n" +
		`<p>See <a href="#intro">intro</a>.</p>` + "\n"
	if got != want {
		t.Errorf("renderBody():\n got: %q\nwant: %q", got, want)
	}
}

func TestImagePathIsSiteRelative(t *testing.T) {
	// A path in a document resolves against the page the document is served
	// at, not against the site root.
	got := render(t, `#image("cat.png")`)
	if want := `<p><img src="/test/cat.png" alt=""></p>` + "\n"; got != want {
		t.Errorf("renderBody():\n got: %q\nwant: %q", got, want)
	}
}

// TestNoteCarriesNoIDs checks that a label inside a footnote is written once.
// The note a mark opens is a second rendering of the body the endnote list
// already holds, and two elements with one id is markup no browser can resolve.
func TestNoteCarriesNoIDs(t *testing.T) {
	templates := template.New("")
	template.Must(templates.New("article").Parse("{{.Content}}"))
	for _, name := range []string{"fragments/include_snippet", "fragments/include_diff"} {
		template.Must(templates.New(name).Parse(""))
	}

	src := "#metadata((title: \"T\", type: \"article\")) <doc-meta>\n\n" +
		"Text.#footnote[A note. <in-note>]<note> and again@note\n"
	doc, err := gmarkst.Load(t.Context(), "test.mst", []byte(src), nil, nil)
	if err != nil {
		t.Fatalf("compiling document: %v", err)
	}

	out, err := html.RenderPage(templates, doc, site.Metadata{Type: "article"}, resolveAll(t, doc.Doc), "/test", "")
	if err != nil {
		t.Fatalf("RenderPage() = %v", err)
	}
	if n := strings.Count(string(out), `id="in-note"`); n != 1 {
		t.Errorf("RenderPage() wrote id=\"in-note\" %d times, want 1:\n%s", n, out)
	}
	if n := strings.Count(string(out), `class="footnote-body"`); n != 2 {
		t.Errorf("RenderPage() wrote %d notes, want 2:\n%s", n, out)
	}
}

// TestRenderPageRejectsANonPageType checks the guard on metadata that does not
// name a page template. Lookup alone does not catch it: the empty name finds
// the root template, and a fragment name finds a template that is no page.
func TestRenderPageRejectsANonPageType(t *testing.T) {
	templates := template.New("")
	for _, name := range []string{"article", "fragments/include_snippet", "fragments/include_diff"} {
		template.Must(templates.New(name).Parse(""))
	}

	src := "#metadata((title: \"T\", type: \"article\")) <doc-meta>\n\nHello.\n"
	doc, err := gmarkst.Load(t.Context(), "test.mst", []byte(src), nil, nil)
	if err != nil {
		t.Fatalf("compiling document: %v", err)
	}

	for _, typ := range []string{"", "fragments/include_snippet"} {
		_, err := html.RenderPage(templates, doc, site.Metadata{Type: typ}, nil, "/test", "")
		if err == nil || !strings.Contains(err.Error(), "unknown doc type") {
			t.Errorf("RenderPage() with type %q = %v, want an unknown doc type error", typ, err)
		}
	}
}

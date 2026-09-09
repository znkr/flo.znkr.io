// In package html, not html_test, because renderTOC is unexported.
package html

import (
	"testing"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst/builtins"
	"znkr.io/markst"
	"znkr.io/markst/value"
)

// compileTOC compiles src the way the loader does, with an index, which is
// what the outline is read from.
func compileTOC(t *testing.T, src string) *renderer {
	t.Helper()
	var index value.Index
	doc, _, err := markst.Compile(t.Context(), []byte(src), markst.WithIndex(&index))
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	arts, err := builtins.Artifacts(doc)
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
	return &renderer{path: "/test", doc: doc, index: &index, frags: frags, notes: indexNotes(doc)}
}

func TestRenderTOC(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{{
		name: "flat",
		in:   "= One\n\n= Two",
		want: `<ul><li><a href="#one">One</a></li>` + "\n" +
			`<li><a href="#two">Two</a></li>` + "\n</ul>",
	}, {
		name: "nested",
		in:   "= One\n\n== A\n\n= Two",
		want: `<ul><li><a href="#one">One</a>` +
			`<ul><li><a href="#a">A</a></li>` + "\n</ul>" + "</li>\n" +
			`<li><a href="#two">Two</a></li>` + "\n</ul>",
	}, {
		// A skipped level nests under the nearest shallower heading rather
		// than inventing an empty one to sit in.
		name: "skipped_level",
		in:   "= One\n\n=== Deep",
		want: `<ul><li><a href="#one">One</a>` +
			`<ul><li><a href="#deep">Deep</a></li>` + "\n</ul>" + "</li>\n</ul>",
	}, {
		name: "markup_in_a_heading",
		in:   "= A *bold* title",
		want: `<ul><li><a href="#a-bold-title">A <strong>bold</strong> title</a></li>` + "\n</ul>",
	}, {
		name: "no_headings",
		in:   "Just a paragraph.",
		want: "",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := compileTOC(t, tt.in)
			got, err := r.renderTOC()
			if err != nil {
				t.Fatalf("renderTOC: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("renderTOC(%q):\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRenderTOCDropsFootnoteMarks checks that a footnote cited from a heading
// leaves no mark in the table of contents, and no endnote list either.
func TestRenderTOCDropsFootnoteMarks(t *testing.T) {
	src := "Body.#footnote[First]#footnote[Second]<second>\n\n= Head#footnote[Third] and@second\n"
	r := compileTOC(t, src)
	got, err := r.renderTOC()
	if err != nil {
		t.Fatalf("renderTOC: %v", err)
	}
	want := `<ul><li><a href="#head-and">Head and</a></li>` + "\n</ul>"
	if string(got) != want {
		t.Errorf("renderTOC():\n got: %q\nwant: %q", got, want)
	}
}

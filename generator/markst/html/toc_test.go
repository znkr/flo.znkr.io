// In package html, not html_test, because renderTOC is unexported.
package html

import (
	"testing"

	"znkr.io/markst"
)

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
			doc, _, err := markst.Compile([]byte(tt.in))
			if err != nil {
				t.Fatalf("compiling: %v", err)
			}
			got, err := (&renderer{path: "/test"}).renderTOC(doc)
			if err != nil {
				t.Fatalf("renderTOC: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("renderTOC(%q):\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRenderTOCFootnoteNumbering checks that a footnote cited from a heading
// keeps the number it has in the body: the table of contents is rendered as a
// fragment of the document, not as one of its own.
func TestRenderTOCFootnoteNumbering(t *testing.T) {
	doc, _, err := markst.Compile([]byte("Body.#footnote[First]\n\n= Head#footnote[Second]\n"))
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	got, err := (&renderer{path: "/test"}).renderTOC(doc)
	if err != nil {
		t.Fatalf("renderTOC: %v", err)
	}
	want := `<ul><li><a href="#head">Head<sup id="fnref:2"><a href="#fn:2">2</a></sup></a></li>` + "\n</ul>"
	if string(got) != want {
		t.Errorf("renderTOC():\n got: %q\nwant: %q", got, want)
	}
}

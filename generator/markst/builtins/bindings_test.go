package builtins

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/highlight"
	"znkr.io/diff"
	"znkr.io/markst"
	"znkr.io/markst/value"
)

const helloGo = `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

const exampleDiff = ` package main
 
-func main() {}
+func main() {
+	println("hi")
+}
`

func docRoot(t *testing.T) fs.FS {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q) = %v", dir, err)
	}
	for name, data := range map[string]string{
		"hello.go":     helloGo,
		"example.diff": exampleDiff,
		"before.go":    "package main\n\nfunc main() {}\n",
		"after.go":     "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n",
	} {
		if err := root.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root.FS()
}

// text strips the highlighter's markup back off and drops the trailing
// newline, so that a test can assert on the source line without depending on
// how chroma tokenized it.
func text(html string) string {
	var sb strings.Builder
	depth := 0
	for _, r := range html {
		switch {
		case r == '<':
			depth++
		case r == '>':
			depth--
		case depth == 0:
			sb.WriteRune(r)
		}
	}
	unescaped := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&#34;", `"`).Replace(sb.String())
	return strings.TrimSuffix(unescaped, "\n")
}

// compileDoc compiles a markst document against the include bindings for a
// document living in dir at /doc, and returns the payloads of every Custom
// element it produced, in document order.
func compileDoc(t *testing.T, fs fs.FS, src string) []any {
	t.Helper()
	doc, warns, err := markst.Compile(t.Context(), []byte(src), markst.WithName("index.mst"), markst.WithBindings(Bindings(fs)))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("Compile() warnings = %v, want none", warns)
	}
	var out []any
	for c := range value.Preorder(doc, value.SetOf(value.KindCustom)) {
		if custom, ok := c.Node().(*value.Custom); ok {
			out = append(out, custom.Value)
		}
	}
	return out
}

// include compiles a document consisting of the single call src and returns the
// one include it produced, together with the fragment its request builds.
//
// The two go together: the include says what to draw around a fragment and the
// fragment is what there is to draw, and neither is much of a test without the
// other.
func include(t *testing.T, fsys fs.FS, src string) (*Include, Fragment) {
	t.Helper()
	doc, warns, err := markst.Compile(t.Context(), []byte(src+"\n"),
		markst.WithName("index.mst"), markst.WithBindings(Bindings(fsys)))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("Compile() warnings = %v, want none", warns)
	}

	var payloads []any
	for c := range value.Preorder(doc, value.SetOf(value.KindCustom)) {
		payloads = append(payloads, c.Node().(*value.Custom).Value)
	}
	if len(payloads) != 1 {
		t.Fatalf("%s produced %d custom elements, want 1", src, len(payloads))
	}
	data, ok := payloads[0].(*Include)
	if !ok {
		t.Fatalf("%s produced a %T, want *Include", src, payloads[0])
	}

	arts, err := Artifacts(doc)
	if err != nil {
		t.Fatalf("Artifacts() = %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("%s produced %d fragments, want 1", src, len(arts))
	}
	frag, err := arts[0].Get(t.Context(), build.NewCache(0))
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	return data, frag
}

// compileErr compiles a document expected to fail and returns its diagnostics.
func compileErr(t *testing.T, fs fs.FS, src string) markst.DiagnosticList {
	t.Helper()
	_, _, err := markst.Compile(t.Context(), []byte(src), markst.WithName("index.mst"), markst.WithBindings(Bindings(fs)))
	var diags markst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("Compile() error is %T (%v), want markst.DiagnosticList", err, err)
	}
	return diags
}

func TestSnippet(t *testing.T) {
	fs := docRoot(t)

	tests := []struct {
		name                string
		src                 string
		wantFile, wantPath  string
		wantFirst, wantLast string
		wantLineNos         []int
	}{
		{
			name:     "whole_file",
			src:      `#include-snippet("hello.go")`,
			wantFile: "hello.go", wantPath: "hello.go",
			wantFirst: "package main", wantLast: "}",
			wantLineNos: []int{1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:     "line_range_is_1_based_inclusive",
			src:      `#include-snippet("hello.go", lines: "3..5")`,
			wantFile: "hello.go", wantPath: "hello.go",
			wantFirst: `import "fmt"`, wantLast: "func main() {",
			wantLineNos: []int{3, 4, 5},
		},
		{
			name:     "open_start",
			src:      `#include-snippet("hello.go", lines: "..2")`,
			wantFile: "hello.go", wantPath: "hello.go",
			wantFirst: "package main", wantLast: "",
			wantLineNos: []int{1, 2},
		},
		{
			name:     "open_end_is_clamped",
			src:      `#include-snippet("hello.go", lines: "6..999")`,
			wantFile: "hello.go", wantPath: "hello.go",
			wantFirst: "\tfmt.Println(\"hello\")", wantLast: "}",
			wantLineNos: []int{6, 7},
		},
		{
			name:     "display_overrides_the_caption",
			src:      `#include-snippet("hello.go", display: "the greeter")`,
			wantFile: "the greeter", wantPath: "hello.go",
			wantFirst: "package main", wantLast: "}",
			wantLineNos: []int{1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:     "lang_overrides_the_lexer",
			src:      `#include-snippet("hello.go", lang: "text")`,
			wantFile: "hello.go", wantPath: "hello.go",
			wantFirst: "package main", wantLast: "}",
			wantLineNos: []int{1, 2, 3, 4, 5, 6, 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, frag := include(t, fs, tt.src)
			if got.File != tt.wantFile {
				t.Errorf("File = %q, want %q", got.File, tt.wantFile)
			}
			if got.FilePath != tt.wantPath {
				t.Errorf("FilePath = %q, want %q", got.FilePath, tt.wantPath)
			}
			lines, err := got.SelectLines(lines(t, frag))
			if err != nil {
				t.Fatalf("SelectLines() = %v", err)
			}
			var nos []int
			for _, l := range lines {
				nos = append(nos, l.LineNo)
			}
			if len(nos) != len(tt.wantLineNos) {
				t.Fatalf("line numbers = %v, want %v", nos, tt.wantLineNos)
			}
			for i := range nos {
				if nos[i] != tt.wantLineNos[i] {
					t.Fatalf("line numbers = %v, want %v", nos, tt.wantLineNos)
				}
			}
			if got := text(string(lines[0].Content)); got != tt.wantFirst {
				t.Errorf("first line = %q, want %q", got, tt.wantFirst)
			}
			if got := text(string(lines[len(lines)-1].Content)); got != tt.wantLast {
				t.Errorf("last line = %q, want %q", got, tt.wantLast)
			}
		})
	}
}

// lines and edits unwrap a fragment, failing the test if it is the other kind.
func lines(t *testing.T, f Fragment) []highlight.Line {
	t.Helper()
	l, ok := f.(Lines)
	if !ok {
		t.Fatalf("fragment is a %T, want Lines", f)
	}
	return l
}

func edits(t *testing.T, f Fragment) []highlight.Edit {
	t.Helper()
	e, ok := f.(Edits)
	if !ok {
		t.Fatalf("fragment is a %T, want Edits", f)
	}
	return e
}

// wantEdits is the diff both forms of include-diff produce for the fixtures:
// the same three matches, one deletion and three insertions.
var wantEdits = []struct {
	op   diff.Op
	x, y int
	line string
}{
	{diff.Match, 1, 1, "package main"},
	{diff.Match, 2, 2, ""},
	{diff.Delete, 3, -1, "func main() {}"},
	{diff.Insert, -1, 3, "func main() {"},
	{diff.Insert, -1, 4, "\tprintln(\"hi\")"},
	{diff.Insert, -1, 5, "}"},
}

func checkEdits(t *testing.T, got []highlight.Edit) {
	t.Helper()
	if len(got) != len(wantEdits) {
		t.Fatalf("got %d edits, want %d: %v", len(got), len(wantEdits), got)
	}
	for i, w := range wantEdits {
		if got[i].Op != w.op || got[i].LineNoX != w.x || got[i].LineNoY != w.y {
			t.Errorf("edit %d = {%v %d %d}, want {%v %d %d}",
				i, got[i].Op, got[i].LineNoX, got[i].LineNoY, w.op, w.x, w.y)
		}
		if line := text(string(got[i].Content)); line != w.line {
			t.Errorf("edit %d line = %q, want %q", i, line, w.line)
		}
	}
}

func TestDiff(t *testing.T) {
	fs := docRoot(t)

	t.Run("from_diff_file", func(t *testing.T) {
		got, frag := include(t, fs, `#include-diff("example.diff", lang: "go")`)
		if got.File != "example.diff" || got.FilePath != "example.diff" {
			t.Errorf("caption = %q -> %q, want example.diff -> example.diff", got.File, got.FilePath)
		}
		checkEdits(t, edits(t, frag))
	})

	t.Run("from_two_files", func(t *testing.T) {
		got, frag := include(t, fs, `#include-diff(a: "before.go", b: "after.go")`)
		// The caption names the after side: a diff is about what the file
		// became.
		if got.File != "after.go" || got.FilePath != "after.go" {
			t.Errorf("caption = %q -> %q, want after.go -> after.go", got.File, got.FilePath)
		}
		checkEdits(t, edits(t, frag))
	})

	t.Run("dev_null_is_an_empty_side", func(t *testing.T) {
		_, frag := include(t, fs, `#include-diff(a: "/dev/null", b: "before.go")`)
		for i, ed := range edits(t, frag) {
			if !ed.IsInsert() {
				t.Fatalf("edit %d is %v, want every line inserted when the before side is empty", i, ed.Op)
			}
		}
	})

	t.Run("display_overrides_the_caption", func(t *testing.T) {
		got, _ := include(t, fs, `#include-diff("example.diff", lang: "go", display: "the change")`)
		if got.File != "the change" || got.FilePath != "example.diff" {
			t.Errorf("caption = %q -> %q, want 'the change' -> example.diff", got.File, got.FilePath)
		}
	})
}

// TestOneFileTwoWaysAreDistinctFragments checks the case a closed set of
// requests makes easy to get wrong: the same file included as a snippet and as
// a diff carries the same bytes and the same lexer, so nothing but the type of
// the request tells the two apart.
func TestOneFileTwoWaysAreDistinctFragments(t *testing.T) {
	fsys := docRoot(t)
	// The same lexer on both, so that the two requests are identical field for
	// field and only their type separates them.
	src := "#include-snippet(\"example.diff\", lang: \"diff\")\n\n#include-diff(\"example.diff\", lang: \"diff\")\n"
	doc, _, err := markst.Compile(t.Context(), []byte(src),
		markst.WithName("index.mst"), markst.WithBindings(Bindings(fsys)))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}

	var reqs []Request
	for cur := range value.Preorder(doc, value.SetOf(value.KindCustom)) {
		r, err := request(cur.Node())
		if err != nil {
			t.Fatalf("request() = %v", err)
		}
		reqs = append(reqs, r)
	}
	if len(reqs) != 2 {
		t.Fatalf("the document holds %d requests, want 2", len(reqs))
	}
	if _, ok := reqs[0].(Highlight); !ok {
		t.Errorf("include-snippet produced a %T, want Highlight", reqs[0])
	}
	if _, ok := reqs[1].(ParseDiff); !ok {
		t.Errorf("include-diff produced a %T, want ParseDiff", reqs[1])
	}

	// The build keys a fragment on its request alone, so the two must not agree
	// on a key or one would be served the other's highlighting.
	c := build.NewCache(0)
	arts, err := Artifacts(doc)
	if err != nil {
		t.Fatalf("Artifacts() = %v", err)
	}
	a, b := arts[0], arts[1]
	if a.Key() == b.Key() {
		t.Fatal("the snippet and the diff share a key")
	}

	snippet, err := a.Get(t.Context(), c)
	if err != nil {
		t.Fatalf("resolving the snippet: %v", err)
	}
	diff, err := b.Get(t.Context(), c)
	if err != nil {
		t.Fatalf("resolving the diff: %v", err)
	}
	if lines, ok := snippet.(Lines); !ok || len(lines) == 0 {
		t.Errorf("the snippet resolved to %T, want non-empty Lines", snippet)
	}
	if edits, ok := diff.(Edits); !ok || len(edits) == 0 {
		t.Errorf("the diff resolved to %T, want non-empty Edits", diff)
	}
}

// TestIncludeIsABlock checks the property the includes depend on: an
// include stands on its own, never swallowed into the paragraph around it.
func TestIncludeIsABlock(t *testing.T) {
	fs := docRoot(t)
	doc, _, err := markst.Compile(t.Context(), []byte("Before.\n\n#include-snippet(\"hello.go\")\n\nAfter.\n"),
		markst.WithBindings(Bindings(fs)))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}

	for c := range value.Preorder(doc, value.SetOf(value.KindPar)) {
		par, ok := c.Node().(*value.Par)
		if !ok {
			continue
		}
		for sub := range value.Preorder(par.Body, value.SetOf(value.KindCustom)) {
			if _, ok := sub.Node().(*value.Custom); ok {
				t.Fatalf("include ended up inside a paragraph:\n%s", value.FormatContent(doc))
			}
		}
	}
}

// TestErrorsPointAtTheArgument is why the bindings do their work while
// the document compiles: every failure is a diagnostic in the document, at the
// argument that caused it.
func TestErrorsPointAtTheArgument(t *testing.T) {
	fs := docRoot(t)
	tests := []struct {
		name    string
		src     string
		wantMsg string
		wantCol uint32 // 1-based column the diagnostic starts at
	}{
		{
			name:    "missing_file",
			src:     `#include-snippet("nope.go")`,
			wantMsg: "no such file",
			wantCol: 18,
		},
		{
			name:    "empty_file",
			src:     `#include-snippet("")`,
			wantMsg: "empty file attribute",
			wantCol: 18,
		},
		{
			name:    "malformed_range_blames_lines",
			src:     `#include-snippet("hello.go", lines: "a..b")`,
			wantMsg: "invalid lines attribute",
			wantCol: 37,
		},
		{
			name:    "inverted_range_blames_lines",
			src:     `#include-snippet("hello.go", lines: "5..2")`,
			wantMsg: "empty lines range",
			wantCol: 37,
		},
		{
			name:    "negative_end_blames_lines",
			src:     `#include-snippet("hello.go", lines: "..-1")`,
			wantMsg: "invalid lines attribute",
			wantCol: 37,
		},
		{
			name:    "negative_start_blames_lines",
			src:     `#include-snippet("hello.go", lines: "-3..5")`,
			wantMsg: "invalid lines attribute",
			wantCol: 37,
		},
		{
			name:    "zero_start_blames_lines",
			src:     `#include-snippet("hello.go", lines: "0..5")`,
			wantMsg: "invalid lines attribute",
			wantCol: 37,
		},
		{
			name:    "start_past_the_end_blames_lines",
			src:     `#include-snippet("hello.go", lines: "99..")`,
			wantMsg: "empty lines range",
			wantCol: 37,
		},
		{
			name:    "missing_diff_blames_the_positional",
			src:     `#include-diff("nope.diff")`,
			wantMsg: "no such file",
			wantCol: 15,
		},
		{
			name:    "unreadable_a_blames_a",
			src:     `#include-diff(a: "nope.go", b: "after.go")`,
			wantMsg: "no such file",
			wantCol: 18,
		},
		{
			name:    "unreadable_b_blames_b",
			src:     `#include-diff(a: "before.go", b: "nope.go")`,
			wantMsg: "no such file",
			wantCol: 34,
		},
		{
			name:    "both_forms_blames_the_positional",
			src:     `#include-diff("example.diff", a: "before.go", b: "after.go")`,
			wantMsg: "either diff or a and b",
			wantCol: 15,
		},
		{
			// With no positional argument written, there is nothing inside
			// the call to underline, so the diagnostic falls back to the whole
			// expression — which starts at the name, just past the `#`.
			name:    "neither_form_blames_the_call",
			src:     `#include-diff(display: "x")`,
			wantMsg: "either diff or a and b",
			wantCol: 2,
		},
		{
			name:    "half_of_the_two_file_form_blames_the_call",
			src:     `#include-diff(a: "before.go")`,
			wantMsg: "either diff or a and b",
			wantCol: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := compileErr(t, fs, tt.src+"\n")
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want exactly one", diags)
			}
			d := diags[0]
			if !strings.Contains(d.Msg, tt.wantMsg) {
				t.Errorf("message = %q, want it to mention %q", d.Msg, tt.wantMsg)
			}
			if d.Origin != "index.mst" {
				t.Errorf("origin = %q, want index.mst", d.Origin)
			}
			if d.Loc.Start.Column != tt.wantCol {
				t.Errorf("column = %d, want %d (%s)", d.Loc.Start.Column, tt.wantCol, tt.src)
			}
		})
	}
}

// TestFileIsRequired checks that a snippet without a file fails: the parameter
// carries no default, so markst rejects the call before the implementation runs.
func TestFileIsRequired(t *testing.T) {
	fs := docRoot(t)
	diags := compileErr(t, fs, "#include-snippet()\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "missing argument: file") {
		t.Errorf("diagnostics = %v, want one about a missing file argument", diags)
	}
}

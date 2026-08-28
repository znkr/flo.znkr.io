package builtins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// srcDir writes the fixture files the includes read and returns the directory
// they live in.
func srcDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range map[string]string{
		"hello.go":     helloGo,
		"example.diff": exampleDiff,
		"before.go":    "package main\n\nfunc main() {}\n",
		"after.go":     "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
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
func compileDoc(t *testing.T, dir, src string) []any {
	t.Helper()
	doc, warns, err := markst.Compile([]byte(src), markst.WithName("index.mst"), markst.WithBindings(Bindings(dir, "/doc")))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("Compile() warnings = %v, want none", warns)
	}
	var out []any
	for c := range value.All(doc) {
		if custom, ok := c.(*value.Custom); ok {
			out = append(out, custom.Value)
		}
	}
	return out
}

// include compiles a document consisting of the single call src and returns the
// one payload it produced.
func include[T any](t *testing.T, dir, src string) T {
	t.Helper()
	got := compileDoc(t, dir, src+"\n")
	if len(got) != 1 {
		t.Fatalf("%s produced %d custom elements, want 1", src, len(got))
	}
	data, ok := got[0].(T)
	if !ok {
		t.Fatalf("%s produced a %T, want %T", src, got[0], data)
	}
	return data
}

// compileErr compiles a document expected to fail and returns its diagnostics.
func compileErr(t *testing.T, dir, src string) markst.DiagnosticList {
	t.Helper()
	_, _, err := markst.Compile([]byte(src), markst.WithName("index.mst"), markst.WithBindings(Bindings(dir, "/doc")))
	var diags markst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("Compile() error is %T (%v), want markst.DiagnosticList", err, err)
	}
	return diags
}

func TestSnippet(t *testing.T) {
	dir := srcDir(t)

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
			wantFile: "hello.go", wantPath: "/doc/hello.go",
			wantFirst: "package main", wantLast: "}",
			wantLineNos: []int{1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:     "line_range_is_1_based_inclusive",
			src:      `#include-snippet("hello.go", lines: "3..5")`,
			wantFile: "hello.go", wantPath: "/doc/hello.go",
			wantFirst: `import "fmt"`, wantLast: "func main() {",
			wantLineNos: []int{3, 4, 5},
		},
		{
			name:     "open_start",
			src:      `#include-snippet("hello.go", lines: "..2")`,
			wantFile: "hello.go", wantPath: "/doc/hello.go",
			wantFirst: "package main", wantLast: "",
			wantLineNos: []int{1, 2},
		},
		{
			name:     "open_end_is_clamped",
			src:      `#include-snippet("hello.go", lines: "6..999")`,
			wantFile: "hello.go", wantPath: "/doc/hello.go",
			wantFirst: "\tfmt.Println(\"hello\")", wantLast: "}",
			wantLineNos: []int{6, 7},
		},
		{
			name:     "display_overrides_the_caption",
			src:      `#include-snippet("hello.go", display: "the greeter")`,
			wantFile: "the greeter", wantPath: "/doc/hello.go",
			wantFirst: "package main", wantLast: "}",
			wantLineNos: []int{1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:     "lang_overrides_the_lexer",
			src:      `#include-snippet("hello.go", lang: "text")`,
			wantFile: "hello.go", wantPath: "/doc/hello.go",
			wantFirst: "package main", wantLast: "}",
			wantLineNos: []int{1, 2, 3, 4, 5, 6, 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := include[*SnippetData](t, dir, tt.src)
			if got.File != tt.wantFile {
				t.Errorf("File = %q, want %q", got.File, tt.wantFile)
			}
			if got.FilePath != tt.wantPath {
				t.Errorf("FilePath = %q, want %q", got.FilePath, tt.wantPath)
			}
			var nos []int
			for _, l := range got.Lines {
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
			if got := text(string(got.Lines[0].Content)); got != tt.wantFirst {
				t.Errorf("first line = %q, want %q", got, tt.wantFirst)
			}
			if got := text(string(got.Lines[len(got.Lines)-1].Content)); got != tt.wantLast {
				t.Errorf("last line = %q, want %q", got, tt.wantLast)
			}
		})
	}
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
	dir := srcDir(t)

	t.Run("from_diff_file", func(t *testing.T) {
		got := include[*DiffData](t, dir, `#include-diff("example.diff", lang: "go")`)
		if got.File != "example.diff" || got.FilePath != "/doc/example.diff" {
			t.Errorf("caption = %q -> %q, want example.diff -> /doc/example.diff", got.File, got.FilePath)
		}
		checkEdits(t, got.Diff)
	})

	t.Run("from_two_files", func(t *testing.T) {
		got := include[*DiffData](t, dir, `#include-diff(a: "before.go", b: "after.go")`)
		// The caption names the after side: a diff is about what the file
		// became.
		if got.File != "after.go" || got.FilePath != "/doc/after.go" {
			t.Errorf("caption = %q -> %q, want after.go -> /doc/after.go", got.File, got.FilePath)
		}
		checkEdits(t, got.Diff)
	})

	t.Run("dev_null_is_an_empty_side", func(t *testing.T) {
		got := include[*DiffData](t, dir, `#include-diff(a: "/dev/null", b: "before.go")`)
		for i, ed := range got.Diff {
			if !ed.IsInsert() {
				t.Fatalf("edit %d is %v, want every line inserted when the before side is empty", i, ed.Op)
			}
		}
	})

	t.Run("display_overrides_the_caption", func(t *testing.T) {
		got := include[*DiffData](t, dir, `#include-diff("example.diff", lang: "go", display: "the change")`)
		if got.File != "the change" || got.FilePath != "/doc/example.diff" {
			t.Errorf("caption = %q -> %q, want 'the change' -> /doc/example.diff", got.File, got.FilePath)
		}
	})
}

// TestIncludeIsABlock checks the property the includes depend on: an
// include stands on its own, never swallowed into the paragraph around it.
func TestIncludeIsABlock(t *testing.T) {
	dir := srcDir(t)
	doc, _, err := markst.Compile([]byte("Before.\n\n#include-snippet(\"hello.go\")\n\nAfter.\n"),
		markst.WithBindings(Bindings(dir, "/doc")))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}

	for c := range value.All(doc) {
		par, ok := c.(*value.Par)
		if !ok {
			continue
		}
		for sub := range value.All(par.Body) {
			if _, ok := sub.(*value.Custom); ok {
				t.Fatalf("include ended up inside a paragraph:\n%s", value.FormatContent(doc))
			}
		}
	}
}

// TestErrorsPointAtTheArgument is why the bindings do their work while
// the document compiles: every failure is a diagnostic in the document, at the
// argument that caused it.
func TestErrorsPointAtTheArgument(t *testing.T) {
	dir := srcDir(t)
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
			diags := compileErr(t, dir, tt.src+"\n")
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
	dir := srcDir(t)
	diags := compileErr(t, dir, "#include-snippet()\n")
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "missing argument: file") {
		t.Errorf("diagnostics = %v, want one about a missing file argument", diags)
	}
}

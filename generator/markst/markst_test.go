package markst

import (
	"fmt"
	"strings"
	"testing"
)

// types are the doc types the tests load documents with.
var types = []string{"article", "page"}

const docMeta = `#metadata((
    title: "Test",
    type: "article",
)) <doc-meta>

`

// TestLoadErrors checks that a failing document is returned with the file,
// line, and column of every error it contains.
func TestLoadErrors(t *testing.T) {
	src := docMeta + "= Heading\n\nText with #undefined_function() in it.\n\nMore #another_missing(1).\n"
	doc, err := Load(t.Context(), "site/doc/index.mst", []byte(src), nil, nil, types)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	err = doc.Err()
	if err == nil {
		t.Fatal("Err() = nil, want error")
	}
	want := []string{
		"site/doc/index.mst:8:12: unknown variable: undefined_function",
		"site/doc/index.mst:10:7: unknown variable: another_missing",
	}
	got := strings.Split(err.Error(), "\n")
	if len(got) != len(want) {
		t.Fatalf("Err() =\n%s\nwant %d lines:\n%s", err, len(want), strings.Join(want, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestLoadMetadataErrors checks that a document whose metadata cannot be read
// is returned with zero metadata and an error naming the file.
func TestLoadMetadataErrors(t *testing.T) {
	for _, tc := range []struct {
		src, want string
	}{
		{"= Heading\n\nNo doc-meta here.\n", "site/doc/index.mst: missing doc-meta"},
		{"#metadata(\"x\") <doc-meta>\n", "site/doc/index.mst: doc-meta is not a dict"},
		{"#metadata((title: 1, type: \"article\")) <doc-meta>\n", "site/doc/index.mst: value for key title has wrong type value.Int, expected value.Str"},
		{"#metadata((title: \"T\", type: \"artikel\")) <doc-meta>\n", `site/doc/index.mst: unknown doc type: "artikel"`},
	} {
		doc, err := Load(t.Context(), "site/doc/index.mst", []byte(tc.src), nil, nil, types)
		if err != nil {
			t.Fatalf("Load(%q) = %v", tc.src, err)
		}
		if doc.Meta != (Metadata{}) {
			t.Errorf("Load(%q) metadata = %+v, want zero", tc.src, doc.Meta)
		}
		if got := fmt.Sprint(doc.Err()); got != tc.want {
			t.Errorf("Load(%q).Err() = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestLoad checks the happy path: metadata comes back and nothing is reported.
func TestLoad(t *testing.T) {
	doc, err := Load(t.Context(), "site/doc/index.mst", []byte(docMeta+"= Heading\n\nSome text.\n"), nil, nil, types)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if doc.Meta.Title != "Test" || doc.Meta.Type != "article" {
		t.Errorf("Load() metadata = %+v, want title Test, type article", doc.Meta)
	}
	if doc.Doc == nil || doc.Index == nil {
		t.Error("Load() returned no document")
	}
	if len(doc.Diags) > 0 {
		t.Errorf("Load() diagnostics = %v, want none", doc.Diags)
	}
}

// TestLoadWithLibrary checks the wiring this package exists to provide: a
// document uses a binding from lib/ that it never imported, and a failure
// inside that binding is reported against the library's own file, naming the
// document line that reached it.
func TestLoadWithLibrary(t *testing.T) {
	lib, err := LoadLibrary(t.Context(), "lib/lib.mst", []byte(`#let published(v) = {
    datetime.parse_date(v)
}
`))
	if err != nil {
		t.Fatalf("LoadLibrary() = %v", err)
	}
	libs := []*Lib{lib}

	t.Run("used without import", func(t *testing.T) {
		src := `#metadata((title: "Test", type: "article", published: published("2024-07-06"))) <doc-meta>` + "\n\n= Heading\n"
		doc, err := Load(t.Context(), "site/doc/index.mst", []byte(src), nil, libs, types)
		if err != nil {
			t.Fatalf("Load() = %v", err)
		}
		if doc.Meta.Title != "Test" {
			t.Errorf("Title = %q, want %q", doc.Meta.Title, "Test")
		}
	})

	t.Run("failure names the library and the call site", func(t *testing.T) {
		src := docMeta + `#published("2024-13-01")` + "\n"
		doc, err := Load(t.Context(), "site/doc/index.mst", []byte(src), nil, libs, types)
		if err != nil {
			t.Fatalf("Load() = %v", err)
		}
		err = doc.Err()
		if err == nil {
			t.Fatal("Err() = nil, want error")
		}
		want := `lib/lib.mst:2:25: invalid date: "2024-13-01"` + "\n" +
			"  hint: dates must be written as `yyyy-mm-dd`, e.g. `2024-02-29`\n" +
			"  note: called from site/doc/index.mst:6:2 in published"
		if err.Error() != want {
			t.Errorf("Err() =\n%s\nwant:\n%s", err, want)
		}
	})
}

// TestLoadWithFailedLibrary checks that a document using a library that did
// not compile is returned with an error naming the library.
func TestLoadWithFailedLibrary(t *testing.T) {
	lib, err := LoadLibrary(t.Context(), "lib/lib.mst", []byte("#let x = undefined\n"))
	if err != nil {
		t.Fatalf("LoadLibrary() = %v", err)
	}
	if lib.Lib != nil {
		t.Fatal("LoadLibrary() returned a library, want none")
	}
	doc, err := Load(t.Context(), "site/doc/index.mst", []byte(docMeta), nil, []*Lib{lib}, types)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	want := "site/doc/index.mst: uses a library with errors: lib/lib.mst"
	if got := fmt.Sprint(doc.Err()); got != want {
		t.Errorf("Err() = %q, want %q", got, want)
	}
}

package markst

import (
	"strings"
	"testing"
)

const docMeta = `#metadata((
    title: "Test",
    type: "article",
)) <doc-meta>

`

// TestLoadErrors checks that a failing document is reported with the file,
// line, and column of every error it contains.
func TestLoadErrors(t *testing.T) {
	src := docMeta + "= Heading\n\nText with #undefined_function() in it.\n\nMore #another_missing(1).\n"
	_, _, err := Load("site/doc/index.mst", "site/doc", "/doc", []byte(src), nil)
	if err == nil {
		t.Fatal("Load() = nil, want error")
	}
	want := []string{
		"site/doc/index.mst:8:12: unknown variable: undefined_function",
		"site/doc/index.mst:10:7: unknown variable: another_missing",
	}
	got := strings.Split(err.Error(), "\n")
	if len(got) != len(want) {
		t.Fatalf("Load() =\n%s\nwant %d lines:\n%s", err, len(want), strings.Join(want, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestLoadMetadataErrorNamesFile checks that the errors that have no position
// to report still say which file they came from.
func TestLoadMetadataErrorNamesFile(t *testing.T) {
	_, _, err := Load("site/doc/index.mst", "site/doc", "/doc", []byte("= Heading\n\nNo doc-meta here.\n"), nil)
	if err == nil {
		t.Fatal("Load() = nil, want error")
	}
	if !strings.HasPrefix(err.Error(), "site/doc/index.mst: ") {
		t.Errorf("Load() = %q, want it to name the file", err)
	}
}

// TestLoad checks the happy path: metadata comes back and nothing is reported.
func TestLoad(t *testing.T) {
	meta, rd, err := Load("site/doc/index.mst", "site/doc", "/doc", []byte(docMeta+"= Heading\n\nSome text.\n"), nil)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if meta.Title != "Test" || meta.Type != "article" {
		t.Errorf("Load() metadata = %+v, want title Test, type article", meta)
	}
	if rd == nil || rd.Doc == nil {
		t.Error("Load() render data is empty")
	}
}

// TestLoadWithLibrary checks the wiring this package exists to provide: a
// document uses a binding from lib/ that it never imported, and a failure
// inside that binding is reported against the library's own file, naming the
// document line that reached it.
func TestLoadWithLibrary(t *testing.T) {
	lib, err := LoadLibrary("lib/lib.mst", []byte(`#let published(v) = {
    datetime.parse_date(v)
}
`), nil)
	if err != nil {
		t.Fatalf("LoadLibrary() = %v", err)
	}
	libs := []*Library{lib}

	t.Run("used without import", func(t *testing.T) {
		src := `#metadata((title: "Test", type: "article", published: published("2024-07-06"))) <doc-meta>` + "\n\n= Heading\n"
		meta, _, err := Load("site/doc/index.mst", "site/doc", "/doc", []byte(src), libs)
		if err != nil {
			t.Fatalf("Load() = %v", err)
		}
		if meta.Title != "Test" {
			t.Errorf("Title = %q, want %q", meta.Title, "Test")
		}
	})

	t.Run("failure names the library and the call site", func(t *testing.T) {
		src := docMeta + `#published("2024-13-01")` + "\n"
		_, _, err := Load("site/doc/index.mst", "site/doc", "/doc", []byte(src), libs)
		if err == nil {
			t.Fatal("Load() = nil, want error")
		}
		want := `lib/lib.mst:2:25: invalid date: "2024-13-01"` + "\n" +
			"  hint: dates must be written as `yyyy-mm-dd`, e.g. `2024-02-29`\n" +
			"  note: called from site/doc/index.mst:6:2 in published"
		if err.Error() != want {
			t.Errorf("Load() =\n%s\nwant:\n%s", err, want)
		}
	})
}

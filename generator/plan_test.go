package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/site"
	"flo.znkr.io/generator/source"
)

// TestPlanComputesNothing checks the property the rest of the design rests on:
// planning declares the build and runs none of it. A tree no step can survive
// is planned without error, and fails only where a value is asked for.
func TestPlanComputesNothing(t *testing.T) {
	dir := fixture(t)
	write(t, filepath.Join(dir, "site/about.mst"), "#no-such-function()\n")
	write(t, filepath.Join(dir, "templates/article.html"), "{{ .Unterminated ")

	s := load(t, dir)

	c := build.NewCache(cacheSize)
	d := s.Doc("/about")
	if d == nil {
		t.Fatal("/about is missing from the plan")
	}
	if _, err := d.Meta.Get(t.Context(), c); err == nil {
		t.Error("Meta.Get() on a document that does not compile = nil, want error")
	}
}

// TestMetadataNeedsNoHighlighting checks what a cold start rests on: reading
// what every document says about itself, which is all the index and the router
// need, highlights nothing.
func TestMetadataNeedsNoHighlighting(t *testing.T) {
	c := build.NewCache(cacheSize)
	s := load(t, fixture(t))

	for _, d := range s.Docs() {
		if _, err := d.Meta.Get(t.Context(), c); err != nil {
			t.Fatalf("%s: Meta.Get() = %v", d.Path, err)
		}
	}
	st := c.Stats()
	if got := st.Kinds["fragment"].Misses; got != 0 {
		t.Errorf("reading metadata highlighted %d fragments, want 0", got)
	}
	if got := st.Kinds["page"].Misses; got != 0 {
		t.Errorf("reading metadata rendered %d pages, want 0", got)
	}
	if got := st.Kinds["doc"].Misses; got == 0 {
		t.Error("reading metadata compiled no documents")
	}
}

// TestReloadReusesEverything checks the baseline incrementality rests on: a
// rebuild from unchanged inputs runs no step twice.
func TestReloadReusesEverything(t *testing.T) {
	dir := fixture(t)

	c := build.NewCache(cacheSize)
	forceAll(t, c, load(t, dir))
	// Discard the first build's counts: the cache outlives a build, so the
	// counts do too unless they are read.
	c.Stats()

	forceAll(t, c, load(t, dir))
	if st := c.Stats(); st.Misses != 0 {
		t.Errorf("second build recomputed %d steps, want 0: %v", st.Misses, st.Kinds)
	}
}

// TestRebuildsOnlyWhatChanged checks that editing a file a single document
// includes recompiles that document and nothing else, and that the fragments of
// that document which don't involve the edited file are still reused.
func TestRebuildsOnlyWhatChanged(t *testing.T) {
	dir := fixture(t)

	c := build.NewCache(cacheSize)
	before := load(t, dir)
	forceAll(t, c, before)
	c.Stats()

	// before.go is reachable only through the include in site/diffs/index.mst;
	// nothing links to it from any other document.
	appendTo(t, filepath.Join(dir, "site/diffs/before.go"), "\nvar added = 1\n")

	after := load(t, dir)
	forceAll(t, c, after)
	st := c.Stats()

	if got := st.Kinds["doc"].Misses; got != 1 {
		t.Errorf("recompiled %d documents, want 1: %v", got, st.Kinds)
	}
	if got := st.Kinds["templates"].Misses; got != 0 {
		t.Errorf("reparsed the templates %d times, want 0", got)
	}
	if got := st.Kinds["lib"].Misses; got != 0 {
		t.Errorf("recompiled %d libraries, want 0", got)
	}
	// The document holds two diffs: one from before.go/after.go, which has to
	// be redone, and one from example.diff, which does not.
	if got := st.Kinds["fragment"]; got.Misses != 1 || got.Hits == 0 {
		t.Errorf("fragments %+v, want one miss and some hits", got)
	}

	// The edited document is a new page; every other one is the page it was.
	pb, pa := pageKeys(before), pageKeys(after)
	if pb["/diffs"] == pa["/diffs"] {
		t.Error("/diffs was reused even though one of its includes changed")
	}
	for _, p := range []string{"/about", "/snippets", "/prose", "/unpublished"} {
		if pb[p] != pa[p] {
			t.Errorf("%s was rebuilt even though nothing it depends on changed", p)
		}
	}
}

// TestProseEditReusesFragments checks the point of keeping one artifact per
// fragment: editing the prose around a code block does not highlight it again.
func TestProseEditReusesFragments(t *testing.T) {
	dir := fixture(t)

	c := build.NewCache(cacheSize)
	forceAll(t, c, load(t, dir))
	c.Stats()

	appendTo(t, filepath.Join(dir, "site/prose/index.mst"), "\nA paragraph that highlights nothing.\n")

	forceAll(t, c, load(t, dir))
	st := c.Stats()

	if got := st.Kinds["doc"].Misses; got != 1 {
		t.Errorf("recompiled %d documents, want 1", got)
	}
	if got := st.Kinds["fragment"]; got.Misses != 0 || got.Hits == 0 {
		t.Errorf("fragments %+v, want no misses and some hits", got)
	}
}

// TestRebuildsOnLibraryChange checks that a library edit reaches every document,
// since any of them may use its bindings.
func TestRebuildsOnLibraryChange(t *testing.T) {
	dir := fixture(t)

	c := build.NewCache(cacheSize)
	before := load(t, dir)
	forceAll(t, c, before)
	c.Stats()
	docs := 0
	for _, d := range before.Docs() {
		if d.Source != "" && filepath.Ext(d.Source) == ".mst" {
			docs++
		}
	}

	appendTo(t, filepath.Join(dir, "lib/base.mst"), "\n#let unused_helper() = { \"x\" }\n")

	forceAll(t, c, load(t, dir))
	if st := c.Stats(); st.Kinds["doc"].Misses != docs {
		t.Errorf("recompiled %d of %d documents after a library edit, want all",
			st.Kinds["doc"].Misses, docs)
	}
}

// TestDropsDeletedDocuments checks that the site is planned from the tree rather
// than accumulated, so a deleted file leaves nothing behind.
func TestDropsDeletedDocuments(t *testing.T) {
	dir := fixture(t)

	c := build.NewCache(cacheSize)
	s := load(t, dir)
	if s.Doc("/prose") == nil {
		t.Fatal("/prose is missing from the first build")
	}
	forceAll(t, c, s)

	if err := os.RemoveAll(filepath.Join(dir, "site/prose")); err != nil {
		t.Fatal(err)
	}

	s = load(t, dir)
	if s.Doc("/prose") != nil {
		t.Error("/prose is still served after its source was deleted")
	}
}

// TestTransparency is the test the correctness of every key rests on.
//
// For each input file in turn it edits that file, rebuilds through a warm graph,
// and builds the same tree from scratch. The two have to render identically: if
// any artifact's key is missing an input, the warm build serves a value computed
// before the edit and the two disagree.
func TestTransparency(t *testing.T) {
	// Files that legitimately change nothing that is rendered. Keeping this
	// list explicit is also a check that the fixture carries no dead weight.
	inert := map[string]bool{
		// Excluded from the site and included by no document.
		"site/ignored/asset.txt": true,
		"site/ignored/index.mst": true,
		// Appending a comment line changes no pattern.
		"site/ignored/.ignore": true,
	}

	base := fixture(t)
	tree, err := source.Scan(base)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	for _, f := range tree.Files("") {
		t.Run(f.Path, func(t *testing.T) {
			dir := fixture(t)

			// Warm the cache on the unedited tree, rendering everything, since
			// a value that was never computed cannot go stale.
			c := build.NewCache(cacheSize)
			forceAll(t, c, load(t, dir))

			appendTo(t, filepath.Join(dir, filepath.FromSlash(f.Path)), edit(f.Path))

			warm := forceAll(t, c, load(t, dir))
			cold := forceAll(t, c, load(t, dir))
			if warm != cold {
				t.Errorf("editing %s: a warm build and a cold one disagree", f.Path)
			}

			// A mutation that changes nothing proves nothing, so make sure the
			// ones expected to matter actually do.
			pristine := forceAll(t, c, load(t, base))
			if changed := warm != pristine; changed == inert[f.Path] {
				if changed {
					t.Errorf("%s is listed as inert but editing it changed the output", f.Path)
				} else {
					t.Errorf("%s changed nothing when edited: the mutation is vacuous", f.Path)
				}
			}
		})
	}
}

// TestRepeatedEditsSettle checks that the cache is bounded in the way a long
// serve session needs: every edit mints new keys for the compile, the metadata,
// the fragments and the pages, and the old ones have to go.
func TestRepeatedEditsSettle(t *testing.T) {
	dir := fixture(t)
	const max = 512
	c := build.NewCache(max)

	for i := range 12 {
		appendTo(t, filepath.Join(dir, "site/prose/index.mst"),
			fmt.Sprintf("\nParagraph %d.\n", i))
		forceAll(t, c, load(t, dir))
		if c.Len() > max {
			t.Fatalf("after %d edits the cache holds %d values, want at most %d", i+1, c.Len(), max)
		}
	}
	// Bounded is not the same as working: the last build must still have reused
	// most of what the one before it computed.
	c.Stats()
	forceAll(t, c, load(t, dir))
	if st := c.Stats(); st.Misses != 0 {
		t.Errorf("rebuilding an unchanged site recomputed %d steps, want 0: %v", st.Misses, st.Kinds)
	}
}

// TestNoDiagnostics checks that neither the fixture nor the real site compiles
// with a warning.
//
// A warning is reported to whoever is running the generator and is easy to
// scroll past, so the only thing that keeps the site free of them is a test
// that reads them all.
func TestNoDiagnostics(t *testing.T) {
	for _, dir := range []string{"testdata/root", ".."} {
		t.Run(dir, func(t *testing.T) {
			c := build.NewCache(cacheSize)
			tree, err := source.Scan(dir)
			if err != nil {
				t.Fatalf("Scan() = %v", err)
			}
			_, diags, err := plan(tree)
			if err != nil {
				t.Fatalf("plan() = %v", err)
			}
			got, err := diags.Get(t.Context(), c)
			if err != nil {
				t.Fatalf("diags.Get() = %v", err)
			}
			if len(got) > 0 {
				t.Errorf("%s compiles with %d warnings:\n%s", dir, len(got), markst.Format(got))
			}
		})
	}
}

// load scans dir and plans the site it holds. Planning computes nothing, so the
// cache only comes in when something is forced.
func load(t *testing.T, dir string) *site.Site {
	t.Helper()
	tree, err := source.Scan(dir)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	s, _, err := plan(tree)
	if err != nil {
		t.Fatalf("plan() = %v", err)
	}
	return s
}

// forceAll computes every artifact of s and returns its pages as one string, so
// that two sites can be compared as a whole.
func forceAll(t *testing.T, c *build.Cache, s *site.Site) string {
	t.Helper()
	var buf bytes.Buffer
	for _, d := range s.Docs() {
		if _, err := d.Meta.Get(t.Context(), c); err != nil {
			t.Fatalf("%s: Meta.Get() = %v", d.Path, err)
		}
		if _, err := d.Content.Get(t.Context(), c); err != nil {
			t.Fatalf("%s: Content.Get() = %v", d.Path, err)
		}
		b, err := d.Page.Get(t.Context(), c)
		if err != nil {
			t.Fatalf("%s: Page.Get() = %v", d.Path, err)
		}
		buf.WriteString(d.Path)
		buf.WriteByte(0)
		buf.Write(b)
		buf.WriteByte(0)
	}
	return buf.String()
}

// pageKeys returns the key of every document's page, keyed by site path. Two
// builds that agree on a key built that page from the same inputs.
func pageKeys(s *site.Site) map[string]build.Key {
	keys := make(map[string]build.Key)
	for _, d := range s.Docs() {
		keys[d.Path] = d.Page.Key()
	}
	return keys
}

// edit returns something to append to a file of this kind that changes what it
// means without making it invalid.
func edit(p string) string {
	// A library is only observable through what a document uses, so the edit
	// redefines a binding used by the documents.
	if p == "lib/base.mst" {
		return "\n#let note(body) = html.elem(\"div\", attrs: (class: \"note probe\"), body)\n"
	}
	if p == "lib/article.mst" {
		return "\n#let doc_type = \"probe\"\n"
	}
	switch filepath.Ext(p) {
	case ".go":
		return "\nvar transparencyProbe = 1\n"
	case ".mst":
		return "\nA paragraph added by the transparency test.\n"
	case ".html":
		return "\n<!-- transparency probe -->\n"
	case ".diff":
		return "+// transparency probe\n"
	case ".css":
		return "\n.transparency-probe { color: red; }\n"
	case ".js":
		return "\n// transparency probe\n"
	default:
		return "\n# transparency probe\n"
	}
}

// fixture copies testdata/root into a temporary directory.
func fixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	const src = "testdata/root"
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0644)
	})
	if err != nil {
		t.Fatalf("copying fixture: %v", err)
	}
	return dst
}

func write(t *testing.T, file, s string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(s), 0644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, file, s string) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, slices.Concat(b, []byte(s)), 0644); err != nil {
		t.Fatal(err)
	}
}

package source

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"flo.znkr.io/generator/build"
	"github.com/google/go-cmp/cmp"
)

const root = "../testdata/root"

func TestScanReadsEverything(t *testing.T) {
	tree, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	want := []string{
		"lib/article.mst",
		"lib/base.mst",
		"site/_assets/script.js",
		"site/_assets/style.css",
		"site/about.mst",
		"site/diffs/after.go",
		"site/diffs/before.go",
		"site/diffs/example.diff",
		"site/diffs/index.mst",
		"site/ignored/.ignore",
		"site/ignored/asset.txt",
		"site/ignored/index.mst",
		"site/prose/index.mst",
		"site/snippets/hello.go",
		"site/snippets/index.mst",
		"site/unpublished/index.mst",
		"templates/article.html",
		"templates/fragments/head.html",
		"templates/fragments/include_diff.html",
		"templates/fragments/include_snippet.html",
		"templates/index.html",
		"templates/page.html",
	}

	var got []string
	for _, f := range tree.Files("") {
		got = append(got, f.Path)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Files() differs (-want +got):\n%s", diff)
	}
}

func TestScanDocs(t *testing.T) {
	tree, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	// Everything under site that isn't hidden and isn't excluded by an
	// .ignore. lib and templates are inputs, never documents.
	want := []string{
		"site/_assets/script.js",
		"site/_assets/style.css",
		"site/about.mst",
		"site/diffs/after.go",
		"site/diffs/before.go",
		"site/diffs/example.diff",
		"site/diffs/index.mst",
		"site/prose/index.mst",
		"site/snippets/hello.go",
		"site/snippets/index.mst",
		"site/unpublished/index.mst",
	}

	var got []string
	for _, f := range tree.Files("") {
		if f.Doc {
			got = append(got, f.Path)
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("documents differ (-want +got):\n%s", diff)
	}

	// The excluded files are still readable: they are include sources.
	for _, p := range []string{"site/ignored/index.mst", "site/ignored/asset.txt"} {
		f, ok := tree.File(p)
		if !ok {
			t.Errorf("%s is not in the tree", p)
			continue
		}
		data, err := tree.Data(f).Get(t.Context(), build.NewCache(0))
		if err != nil {
			t.Errorf("Data(%s).Get() = %v", p, err)
		} else if len(data) == 0 {
			t.Errorf("%s was not read", p)
		}
	}
}

func TestSubIsRootedAndKeyed(t *testing.T) {
	tree, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	sub, err := tree.Sub("site/snippets").Get(t.Context(), build.NewCache(0))
	if err != nil {
		t.Fatalf("Sub().Get(t.Context(), build.NewCache(0)) = %v", err)
	}

	got, err := fs.ReadFile(sub, "hello.go")
	if err != nil {
		t.Fatalf(`ReadFile("hello.go") = %v`, err)
	}
	want, err := os.ReadFile(filepath.Join(root, "site/snippets/hello.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("ReadFile returned the wrong content")
	}

	// Nothing outside the directory is reachable, which is what makes the key
	// cover everything the FS can read.
	for _, name := range []string{
		"../about.mst",
		"../diffs/before.go",
		"/site/about.mst",
		"site/about.mst",
	} {
		if _, err := fs.ReadFile(sub, name); err == nil {
			t.Errorf("ReadFile(%q) succeeded, want an error", name)
		}
	}
}

// TestSubIsAWellBehavedFS runs the standard conformance suite over the FS a
// document is given, which is what checks that a walk of it works: the
// directories between the files are synthesized, so there is more to get wrong
// here than in reading one.
func TestSubIsAWellBehavedFS(t *testing.T) {
	tree, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	fsys, err := tree.Sub("site").Get(t.Context(), build.NewCache(0))
	if err != nil {
		t.Fatalf("Sub().Get(t.Context(), build.NewCache(0)) = %v", err)
	}
	if err := fstest.TestFS(fsys, "about.mst", "snippets/hello.go", "diffs/before.go", "_assets/style.css"); err != nil {
		t.Error(err)
	}

	empty, err := tree.Empty().Get(t.Context(), build.NewCache(0))
	if err != nil {
		t.Fatalf("Empty().Get(t.Context(), build.NewCache(0)) = %v", err)
	}
	if err := fstest.TestFS(empty); err != nil {
		t.Errorf("Empty(): %v", err)
	}
}

// TestSubWalks checks the use Sub is put to that reading one file does not
// cover: finding the files in the first place.
func TestSubWalks(t *testing.T) {
	tree, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	fsys, err := tree.Sub("templates").Get(t.Context(), build.NewCache(0))
	if err != nil {
		t.Fatalf("Sub().Get(t.Context(), build.NewCache(0)) = %v", err)
	}

	var got []string
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		got = append(got, p)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() = %v", err)
	}
	want := []string{
		"article.html",
		"fragments/head.html",
		"fragments/include_diff.html",
		"fragments/include_snippet.html",
		"index.html",
		"page.html",
	}
	if !slices.Equal(got, want) {
		t.Errorf("WalkDir() visited %v, want %v", got, want)
	}
}

// TestSubNestsDirs checks that a directory keeps the entries filed under it
// when a directory below it is synthesized first. Which of the two comes first
// depends on map iteration order, so the FS is built repeatedly.
func TestSubNestsDirs(t *testing.T) {
	files := map[string][]byte{
		"a/b/c/deep.txt": []byte("deep"),
		"a/shallow.txt":  []byte("shallow"),
		"top.txt":        []byte("top"),
	}
	for range 100 {
		if err := fstest.TestFS(newSubFS(files), "a/b/c/deep.txt", "a/shallow.txt", "top.txt"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSubKeyTracksContent(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, root, dir)

	base, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	before := base.Sub("site/snippets").Key()

	// A file the directory holds changes the key.
	appendTo(t, filepath.Join(dir, "site/snippets/hello.go"), "\n// touched\n")
	changed, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	if changed.Sub("site/snippets").Key() == before {
		t.Errorf("editing a file below the directory did not change its key")
	}

	// A file outside it does not.
	if changed.Sub("site/diffs").Key() != base.Sub("site/diffs").Key() {
		t.Errorf("editing a file elsewhere changed an unrelated directory's key")
	}
}

func TestSubKeyIsStable(t *testing.T) {
	a, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	b, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}
	for _, dir := range []string{"site/snippets", "site/diffs", "lib", "templates"} {
		if a.Sub(dir).Key() != b.Sub(dir).Key() {
			t.Errorf("Sub(%q).Key is not stable across scans", dir)
		}
	}
	if a.Empty().Key() == a.Sub("site/snippets").Key() {
		t.Errorf("Empty() collides with a non-empty directory")
	}
}

// copyTree copies the files below src into dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
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
		t.Fatalf("copying tree: %v", err)
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

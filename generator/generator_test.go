package main

import (
	"archive/tar"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"flo.znkr.io/generator/pack"
	"flo.znkr.io/generator/renderers"
	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// TestGenerator runs the generator end to end: it loads the small site in
// testdata/root, packs it, and compares the resulting archive against the
// golden tree in testdata/golden. The fixture brings its own templates and
// library so that the golden files stay small and readable; the real site is
// covered by TestPackSite.
//
// Run `go test ./generator -update` to regenerate the golden tree after an
// intentional change, and read the diff before committing it.
func TestGenerator(t *testing.T) {
	const golden = "testdata/golden"

	c, s, err := loadDir(t.Context(), "testdata/root")
	if err != nil {
		t.Fatalf("loadDir() = %v", err)
	}

	tarfile := filepath.Join(t.TempDir(), "site.tar")
	if err := pack.Pack(t.Context(), tarfile, c, s); err != nil {
		t.Fatalf("Pack() = %v", err)
	}
	got := readTar(t, tarfile)

	if *update {
		if err := os.RemoveAll(golden); err != nil {
			t.Fatalf("removing golden tree: %v", err)
		}
		for name, data := range got {
			file := filepath.Join(golden, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
				t.Fatalf("creating golden directory: %v", err)
			}
			if err := os.WriteFile(file, data, 0644); err != nil {
				t.Fatalf("writing golden file: %v", err)
			}
		}
		t.Logf("wrote %d golden files to %s", len(got), golden)
		return
	}

	want := readTree(t, golden)

	if diff := cmp.Diff(names(want), names(got)); diff != "" {
		t.Errorf("packed files differ from %s (-want +got):\n%s\nrerun with -update if this is intentional", golden, diff)
	}

	for _, name := range names(want) {
		g, ok := got[name]
		if !ok {
			continue // already reported above
		}
		if diff := cmp.Diff(string(want[name]), string(g)); diff != "" {
			t.Errorf("%s differs (-want +got):\n%s", name, diff)
		}
	}
}

// TestPackSite packs the real site in this repository, the way the deploy
// workflow does. It deliberately doesn't check what the pages look like —
// that's TestGenerator's job — only that every document still loads, renders,
// and ends up in the archive, so that a broken article or template is caught
// here rather than on deploy.
func TestPackSite(t *testing.T) {
	// The test binary runs in the package directory, so the site root is one
	// level up.
	c, s, err := loadDir(t.Context(), "..")
	if err != nil {
		t.Fatalf("loadDir() = %v", err)
	}

	tarfile := filepath.Join(t.TempDir(), "znkr.tar")
	if err := pack.Pack(t.Context(), tarfile, c, s); err != nil {
		t.Fatalf("Pack() = %v", err)
	}
	got := readTar(t, tarfile)

	docs := s.Docs()
	if len(docs) == 0 {
		t.Fatal("site has no documents")
	}
	if len(got) != len(docs) {
		t.Errorf("archive has %d files, site has %d documents", len(got), len(docs))
	}

	// Everything the site needs to be usable: the index, the feed, the
	// assets referenced by every page, and a page per article.
	want := []string{"index.html", "feed.atom", "_assets/style.css", "_assets/script.js", "about/index.html"}
	var entries []renderers.Entry
	for _, d := range docs {
		meta, err := d.Meta.Get(t.Context(), c)
		if err != nil {
			t.Fatalf("%s: Meta.Get() = %v", d.Path, err)
		}
		entries = append(entries, renderers.Entry{Path: d.Path, Meta: meta})
	}
	articles := renderers.Articles(entries)
	if len(articles) == 0 {
		t.Error("site has no articles")
	}
	for _, a := range articles {
		want = append(want, path.Join(a.Path, "index.html")[1:])
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("%s is missing from the archive", name)
		}
	}

	for name, data := range got {
		if len(data) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

// readTar reads the tar archive at file and returns its regular files keyed by
// name, with the leading "./" stripped. It fails the test if the archive is
// malformed, if it holds anything but files and directories, or if a file
// shows up before the header for the directory that holds it.
func readTar(t *testing.T, file string) map[string][]byte {
	t.Helper()

	f, err := os.Open(file)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer f.Close()

	files := make(map[string][]byte)
	dirs := make(map[string]bool)
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading archive: %v", err)
		}

		name, ok := stripDot(hdr.Name)
		if !ok {
			t.Errorf("entry %q is not relative to the archive root", hdr.Name)
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			dirs[path.Clean(name)] = true
		case tar.TypeReg:
			dir := path.Dir(name)
			if !dirs[dir] {
				t.Errorf("%s appears before a header for directory %s/", name, dir)
			}
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			if int64(len(b)) != hdr.Size {
				t.Errorf("%s: read %d bytes, header says %d", name, len(b), hdr.Size)
			}
			if _, ok := files[name]; ok {
				t.Errorf("%s appears twice in the archive", name)
			}
			files[name] = b
		default:
			t.Errorf("%s has unexpected type %q", name, hdr.Typeflag)
		}
	}
	return files
}

// stripDot removes the leading "./" that pack writes on every entry, and the
// trailing "/" it writes on directories. The archive is unpacked into the
// document root, so an entry that isn't relative to it would escape it.
func stripDot(name string) (string, bool) {
	if len(name) < 2 || name[0] != '.' || name[1] != '/' {
		return "", false
	}
	name = strings.TrimSuffix(name[2:], "/")
	if name == "" {
		return ".", true
	}
	if name != path.Clean(name) {
		return "", false
	}
	return name, true
}

// readTree reads every file below dir, keyed by slash-separated path relative
// to dir, in the same shape as [readTar].
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()

	files := make(map[string][]byte)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	return files
}

func names(files map[string][]byte) []string {
	ret := make([]string, 0, len(files))
	for name := range files {
		ret = append(ret, name)
	}
	sort.Strings(ret)
	return ret
}

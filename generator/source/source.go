// Package source reads the site's input files into an immutable, hashed
// snapshot.
//
// It is the generator's only filesystem boundary: everything downstream reads
// from a [Tree], so every input a build step can reach is one this package has
// already hashed.
package source

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"flo.znkr.io/generator/build"
)

// Dirs are the subdirectories of the site root that [Scan] reads.
var Dirs = []string{"templates", "lib", "site"}

// File is one input file. It says where the file is and what it may become,
// not what is in it: the bytes are reached through [Tree.Data] or [Tree.Sub],
// so that a step that reads a file is a step that depends on it.
type File struct {
	// Path is the slash-separated path from the tree root, for example
	// "site/diff/diff.go".
	Path string
	// Doc reports whether the file may become a site document. Files in an
	// .ignore excludes are still read: they are snippet and diff sources.
	Doc bool
}

// Tree is a snapshot of the input files below a site root. It does not change
// after [Scan] returns, so it is safe for concurrent use.
type Tree struct {
	files   map[string]*File
	content map[string]build.Artifact[[]byte]
	sorted  []string
}

// Scan reads every file below root's templates, lib, and site directories.
// Missing directories are skipped; hidden directories are not read.
func Scan(root string) (*Tree, error) {
	t := &Tree{
		files:   make(map[string]*File),
		content: make(map[string]build.Artifact[[]byte]),
	}

	for _, sub := range Dirs {
		dir := filepath.Join(root, sub)
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := t.scanDir(root, dir); err != nil {
			return nil, err
		}
	}

	t.sorted = make([]string, 0, len(t.files))
	for p := range t.files {
		t.sorted = append(t.sorted, p)
	}
	slices.Sort(t.sorted)
	return t, nil
}

func (t *Tree) scanDir(root, dir string) error {
	ignores := newIgnoreRules(dir)
	// Whether a directory is excluded, keyed by its path. Excluding a
	// directory excludes everything below it, which is why the answer for a
	// parent has to be carried down rather than recomputed per file.
	skipped := make(map[string]bool)

	return filepath.WalkDir(dir, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if fpath != dir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			// Ancestors have been visited already, so their patterns are
			// loaded and this directory can be judged before its own .ignore
			// adds rules that only apply below it.
			skipped[fpath] = fpath != dir && (skipped[filepath.Dir(fpath)] || ignores.match(fpath))
			return ignores.load(fpath)
		}

		data, err := os.ReadFile(fpath)
		if err != nil {
			return fmt.Errorf("reading file: %v", err)
		}

		rel, err := filepath.Rel(root, fpath)
		if err != nil {
			return err
		}
		p := filepath.ToSlash(rel)

		t.content[p] = build.Const("source.file", data)
		t.files[p] = &File{
			Path: p,
			Doc: strings.HasPrefix(p, "site/") &&
				!strings.HasPrefix(d.Name(), ".") &&
				!skipped[filepath.Dir(fpath)] &&
				!ignores.match(fpath),
		}
		return nil
	})
}

// Data returns f's bytes as an artifact.
func (t *Tree) Data(f *File) build.Artifact[[]byte] {
	a, ok := t.content[f.Path]
	if !ok {
		panic(fmt.Sprintf("source: %s is not in this tree", f.Path))
	}
	return a
}

// File returns the file at the given slash path.
func (t *Tree) File(p string) (*File, bool) {
	f, ok := t.files[p]
	return f, ok
}

// Files returns every file below dir, ordered by path. An empty dir returns
// the whole tree.
func (t *Tree) Files(dir string) []*File {
	prefix := ""
	if dir != "" {
		prefix = strings.TrimSuffix(dir, "/") + "/"
	}
	i := sort.SearchStrings(t.sorted, prefix)
	var ret []*File
	for _, p := range t.sorted[i:] {
		if !strings.HasPrefix(p, prefix) {
			break
		}
		ret = append(ret, t.files[p])
	}
	return ret
}

// Sub returns the files below dir as an [fs.FS], together with the key that
// covers them.
//
// The FS cannot read outside dir, so a step given this dependency can only
// read files its key already hashes.
func (t *Tree) Sub(dir string) build.Artifact[fs.FS] {
	dir = strings.TrimSuffix(dir, "/")

	// The path is a parameter of the rule so that moving a file changes the
	// key, and the content is a dependency so that editing one does.
	files := t.Files(dir)
	names := make([]string, 0, len(files))
	contents := make([]build.Artifact[[]byte], 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimPrefix(f.Path, dir+"/"))
		contents = append(contents, t.content[f.Path])
	}

	return build.Derive[fs.FS]("source.sub", makeSubFS, names, contents)
}

// Empty returns a dependency on no files: an FS that cannot read anything. It
// is what a document that cannot include files gets.
func (t *Tree) Empty() build.Artifact[fs.FS] {
	return build.Derive[fs.FS]("source.sub", makeSubFS, []string(nil), []build.Artifact[[]byte](nil))
}

// makeSubFS returns an FS over the named files, whose paths are relative to its
// root. names and data run parallel.
func makeSubFS(names []string, data [][]byte) (fs.FS, error) {
	files := make(map[string][]byte, len(names))
	for i, name := range names {
		files[name] = data[i]
	}
	return newSubFS(files), nil
}

// subFS serves a fixed set of files, keyed by path relative to its root. The
// directories between them are synthesized, so that the set can be walked.
type subFS struct {
	files map[string][]byte
	dirs  map[string][]fs.DirEntry
}

// newSubFS returns an FS over files, which are keyed by path relative to its
// root.
func newSubFS(files map[string][]byte) *subFS {
	s := &subFS{files: files, dirs: map[string][]fs.DirEntry{".": nil}}

	// A file's directory, and every directory above it, has to exist before the
	// entries can be filed under it.
	seen := make(map[string]bool)
	for p := range files {
		for d := path.Dir(p); !seen[d]; d = path.Dir(d) {
			seen[d] = true
			if d == "." {
				break
			}
			if _, ok := s.dirs[d]; !ok {
				s.dirs[d] = nil
			}
			s.dirs[path.Dir(d)] = append(s.dirs[path.Dir(d)], dirEntry(path.Base(d)))
		}
	}
	for p, data := range files {
		d := path.Dir(p)
		s.dirs[d] = append(s.dirs[d], fileEntry{path.Base(p), data})
	}
	for _, entries := range s.dirs {
		slices.SortFunc(entries, func(a, b fs.DirEntry) int { return cmp.Compare(a.Name(), b.Name()) })
	}
	return s
}

func (s *subFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := s.files[name]; ok {
		return &openFile{info: fileInfo{path.Base(name), data}, r: bytes.NewReader(data)}, nil
	}
	if entries, ok := s.dirs[name]; ok {
		return &openDir{name: path.Base(name), entries: entries}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (s *subFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	data, ok := s.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return slices.Clone(data), nil
}

func (s *subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	entries, ok := s.dirs[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return slices.Clone(entries), nil
}

func (s *subFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := s.files[name]; ok {
		return fileInfo{path.Base(name), data}, nil
	}
	if _, ok := s.dirs[name]; ok {
		return dirInfo(path.Base(name)), nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

type openFile struct {
	info fileInfo
	r    *bytes.Reader
}

func (f *openFile) Stat() (fs.FileInfo, error)         { return f.info, nil }
func (f *openFile) Read(b []byte) (int, error)         { return f.r.Read(b) }
func (f *openFile) Seek(o int64, w int) (int64, error) { return f.r.Seek(o, w) }
func (f *openFile) Close() error                       { return nil }

// openDir is a directory being read, which is the entries it holds and how far
// through them the reader has got.
type openDir struct {
	name    string
	entries []fs.DirEntry
	pos     int
}

func (d *openDir) Stat() (fs.FileInfo, error) { return dirInfo(d.name), nil }
func (d *openDir) Close() error               { return nil }

func (d *openDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}

func (d *openDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.pos:]
	if n <= 0 {
		d.pos = len(d.entries)
		return slices.Clone(rest), nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	rest = rest[:min(n, len(rest))]
	d.pos += len(rest)
	return slices.Clone(rest), nil
}

// fileEntry is one file within a subFS: its name in the directory it is filed
// under, and its bytes.
type fileEntry struct {
	name string
	data []byte
}

func (e fileEntry) Name() string               { return e.name }
func (e fileEntry) IsDir() bool                { return false }
func (e fileEntry) Type() fs.FileMode          { return 0 }
func (e fileEntry) Info() (fs.FileInfo, error) { return fileInfo(e), nil }

type dirEntry string

func (e dirEntry) Name() string               { return string(e) }
func (e dirEntry) IsDir() bool                { return true }
func (e dirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (e dirEntry) Info() (fs.FileInfo, error) { return dirInfo(e), nil }

type fileInfo fileEntry

func (fi fileInfo) Name() string       { return fi.name }
func (fi fileInfo) Size() int64        { return int64(len(fi.data)) }
func (fi fileInfo) Mode() fs.FileMode  { return 0444 }
func (fi fileInfo) ModTime() time.Time { return time.Time{} }
func (fi fileInfo) IsDir() bool        { return false }
func (fi fileInfo) Sys() any           { return nil }

type dirInfo string

func (di dirInfo) Name() string       { return string(di) }
func (di dirInfo) Size() int64        { return 0 }
func (di dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0555 }
func (di dirInfo) ModTime() time.Time { return time.Time{} }
func (di dirInfo) IsDir() bool        { return true }
func (di dirInfo) Sys() any           { return nil }

var (
	_ fs.ReadFileFS  = (*subFS)(nil)
	_ fs.ReadDirFS   = (*subFS)(nil)
	_ fs.StatFS      = (*subFS)(nil)
	_ fs.ReadDirFile = (*openDir)(nil)
	_ io.Seeker      = (*openFile)(nil)
)

// ignoreRules holds the glob patterns from the .ignore files seen so far,
// keyed by the directory each file lives in.
//
// A pattern is matched against the path relative to that directory, so an
// .ignore may exclude entries in its subdirectories as well. Ignoring a
// directory ignores everything below it.
type ignoreRules struct {
	root     string
	patterns map[string][]string
}

func newIgnoreRules(root string) *ignoreRules {
	return &ignoreRules{root: root, patterns: make(map[string][]string)}
}

// load reads the .ignore file in dir, if there is one. Blank lines and lines
// starting with # are ignored.
func (ir *ignoreRules) load(dir string) error {
	name := filepath.Join(dir, ".ignore")
	b, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("reading ignore file: %v", err)
	}

	var patterns []string
	for line := range strings.Lines(string(b)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := path.Match(line, ""); err != nil {
			return fmt.Errorf("%s: invalid pattern %q: %v", name, line, err)
		}
		patterns = append(patterns, line)
	}
	ir.patterns[dir] = patterns
	return nil
}

// match reports whether fpath is excluded by an .ignore file in its own
// directory or in any parent directory up to the site root.
func (ir *ignoreRules) match(fpath string) bool {
	for dir := filepath.Dir(fpath); ; dir = filepath.Dir(dir) {
		if len(ir.patterns[dir]) > 0 {
			rel, err := filepath.Rel(dir, fpath)
			if err != nil {
				return false
			}
			for _, pat := range ir.patterns[dir] {
				if ok, _ := path.Match(pat, filepath.ToSlash(rel)); ok {
					return true
				}
			}
		}
		if dir == ir.root || dir == filepath.Dir(dir) {
			return false
		}
	}
}

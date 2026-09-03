package site

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"flo.znkr.io/generator/build"
)

// Site is an in-memory representation of the to be generated site.
type Site struct {
	docs map[string]Doc
}

// Doc is a single document of the site, that is anything that can be served as
// a static file.
//
// Where it is served and as what are known from the file it came from. What it
// says and what it looks like are artifacts, computed when they are first asked
// for, so a site can be assembled and routed without rendering anything.
type Doc struct {
	// Path is the path on the site, starting with a slash. Every document
	// has a unique path.
	Path string
	// Source is the path to the source file on disk, if any. It is empty for
	// documents that are not generated from a file.
	Source string
	// MimeType is the MIME type of the document, which is used to set the
	// Content-Type header when serving it.
	MimeType string
	// Meta is what the document says about itself. It is the zero Metadata for
	// a document that says nothing, such as an asset copied through unchanged.
	Meta build.Artifact[Metadata]
	// Page is the document as it is served.
	Page build.Artifact[[]byte]
	// Content is the document's body without the page around it, which is what
	// the feed embeds. It is the whole document for an asset.
	Content build.Artifact[[]byte]
}

// DocTypes are the types a document may declare in its metadata. Each names the
// template the document is rendered with.
var DocTypes = []string{"article", "page"}

type Metadata struct {
	Title     string
	Published time.Time
	Updated   time.Time
	Summary   string
	GoImport  string
	Redirect  string
	Type      string
}

// New creates a new site from the provided docs.
//
// If there are multiple docs for the same path, New returns an error.
func New(docs []Doc) (*Site, error) {
	s := &Site{
		docs: make(map[string]Doc),
	}
	for _, d := range docs {
		if _, exists := s.docs[d.Path]; exists {
			return nil, fmt.Errorf("duplicate doc for path %q", d.Path)
		}
		s.docs[d.Path] = d
	}
	return s, nil
}

// Doc returns the document for the given path, or nil if the document cannot be found.
func (s *Site) Doc(path string) *Doc {
	d, ok := s.docs[path]
	if !ok {
		return nil
	}
	return &d
}

// Docs returns every document, ordered by path.
func (s *Site) Docs() []*Doc {
	var ret []*Doc
	for _, d := range s.docs {
		ret = append(ret, &d)
	}
	slices.SortFunc(ret, func(a, b *Doc) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return ret
}

package renderers

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"html/template"
	"time"

	"flo.znkr.io/generator/jsonld"
	"flo.znkr.io/generator/site"
)

const (
	siteName   = "flo.znkr.io"
	authorName = "Florian Zenker"
	authorURL  = "https://flo.znkr.io/about/"
)

// Head is what the head fragment draws: the meta tags a document sets, its
// JSON-LD, and the two values written outside a meta tag.
type Head struct {
	Title        string
	Tags         []Tag
	JSONLD       template.JS
}

// Tag is one <meta> element.
type Tag struct {
	// Attr is the attribute that names the tag: name, property or http-equiv.
	//
	// It is an HTMLAttr because html/template filters a dynamic attribute name
	// to [a-z0-9] and would drop http-equiv.
	Attr template.HTMLAttr
	// Name is that attribute's value, such as "og:title".
	Name string
	// Content is what the tag sets.
	Content string
}

func name(n, content string) Tag     { return Tag{Attr: "name", Name: n, Content: content} }
func property(n, content string) Tag { return Tag{Attr: "property", Name: n, Content: content} }

// NewHead returns the head of the page m describes.
func NewHead(m site.Metadata) (Head, error) {
	h := Head{
		Title:        m.Title,
	}

	if m.GoImport != "" {
		h.Tags = append(h.Tags, name("go-import", m.GoImport))
	}
	if m.Redirect != "" {
		h.Tags = append(h.Tags, Tag{
			Attr:    "http-equiv",
			Name:    "refresh",
			Content: fmt.Sprintf("0;URL='%s'", m.Redirect),
		})
	}

	ogType := "website"
	if m.Type == "article" {
		ogType = "article"
	}
	h.Tags = append(h.Tags,
		property("og:site_name", siteName),
		property("og:title", m.Title),
		property("og:type", ogType),
	)
	if m.Image != "" {
		h.Tags = append(h.Tags,
			property("og:image", m.Image),
			name("twitter:card", "summary_large_image"),
		)
	}
	if m.CanonicalURL != "" {
		h.Tags = append(h.Tags, property("og:url", m.CanonicalURL))
		h.Tags = append(h.Tags, property("canonical", m.CanonicalURL))
	}
	if m.Type == "article" {
		h.Tags = append(h.Tags, property("article:author", authorURL))
		if !m.Published.IsZero() {
			published := m.Published.Format(time.DateOnly)
			updated := m.Updated.Format(time.DateOnly)
			h.Tags = append(h.Tags, property("article:published_time", published))
			if published != updated {
				h.Tags = append(h.Tags, property("article:modified_time", updated))
			}
		}
	}
	if m.Summary != "" {
		h.Tags = append(h.Tags,
			name("description", m.Summary),
			property("og:description", m.Summary),
		)
	}

	j, err := renderJSONLD(m)
	if err != nil {
		return Head{}, err
	}
	h.JSONLD = j

	return h, nil
}

// renderJSONLD returns the JSON-LD of the document m describes, empty for a
// document there is no schema.org type for.
func renderJSONLD(m site.Metadata) (template.JS, error) {
	var v any
	switch m.Type {
	case "article":
		a := jsonld.Article{
			Headline: m.Title,
			Author: []jsonld.Person{{
				Name: authorName,
				URL:  authorURL,
			}},
			Image: m.Image,
			URL:   m.CanonicalURL,
		}
		if !m.Published.IsZero() {
			a.DatePublished = m.Published.Format(time.RFC3339)
			a.DateModified = m.Updated.Format(time.RFC3339)
		}
		v = a
	default:
		return "", nil
	}
	// EscapeForHTML is what keeps a </script> in a title from ending the script
	// element the JSON-LD is written into.
	d, err := json.Marshal(v, jsontext.EscapeForHTML(true))
	if err != nil {
		return "", fmt.Errorf("marshaling JSON-LD: %v", err)
	}
	return template.JS(d), nil
}

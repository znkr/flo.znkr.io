package renderers

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"html/template"
	"strconv"
	"time"

	"flo.znkr.io/generator/jsonld"
	"flo.znkr.io/generator/site"
)

const (
	siteName   = "flo.znkr.io"
	siteURL    = "https://flo.znkr.io/"
	authorName = "Florian Zenker"
	authorURL  = "https://flo.znkr.io/about/"
	language   = "en"
	locale     = "en_US"
	// themeColor is the middle stop of the header gradient, which the site
	// header is drawn with.
	themeColor = "#1d8fe1"
	// fediverseCreator is the account Mastodon credits a link preview to.
	fediverseCreator = "@znkr@hachyderm.io"
)

// The ids the nodes of a page's graph name each other by. A node is a document
// of its own, so its id is a URL of the site with a fragment.
const (
	websiteID = siteURL + "#website"
	personID  = authorURL + "#me"
)

// authorProfiles are the author's accounts elsewhere. schema.org states them
// as sameAs, Mastodon checks a profile's link against a rel=me here, and the
// site footer links to each of them by its icon.
var authorProfiles = []Profile{
	{Name: "GitHub", URL: "https://github.com/znkr", Icon: "github"},
	{Name: "Mastodon", URL: "https://hachyderm.io/@znkr", Icon: "mastodon"},
	{Name: "Bluesky", URL: "https://bsky.app/profile/flo.znkr.io", Icon: "bluesky"},
}

// Profile is an account of the author's elsewhere.
type Profile struct {
	// Name is what the account is called, which is what a link to it is
	// labeled with.
	Name string
	// URL is the account's page.
	URL string
	// Icon names the stylesheet's icon for it.
	Icon string
}

// profileURLs returns the URL of every profile, which is what schema.org states
// as sameAs.
func profileURLs() []string {
	urls := make([]string, len(authorProfiles))
	for i, p := range authorProfiles {
		urls[i] = p.URL
	}
	return urls
}

// Head is what the head fragment draws: the title of the page, the assets it
// can load, the links and meta tags it sets, and what it says about itself as
// JSON-LD.
type Head struct {
	Title  string
	Assets Assets
	Links  []Link
	Tags   []Tag
	JSONLD template.JS
}

// Link is one <link> element.
type Link struct {
	Rel  string
	Href string
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

// NewHead returns the head of the page m describes, which can load assets.
func NewHead(m site.Metadata, assets Assets) (Head, error) {
	h := Head{Title: m.Title, Assets: assets}

	if m.CanonicalURL != "" {
		h.Links = append(h.Links, Link{"canonical", m.CanonicalURL})
	}
	h.Links = append(h.Links, Link{"author", authorURL})
	for _, p := range authorProfiles {
		h.Links = append(h.Links, Link{"me", p.URL})
	}

	// A draft is served like every other document, so it says for itself that
	// it is not to be indexed.
	if m.Draft() {
		h.Tags = append(h.Tags, name("robots", "noindex"))
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
	h.Tags = append(h.Tags,
		name("author", authorName),
		name("fediverse:creator", fediverseCreator),
		// The stylesheet has no dark palette, so a browser is told what it gets
		// rather than left to derive one.
		name("color-scheme", "light"),
		name("theme-color", themeColor),
	)
	if m.Summary != "" {
		h.Tags = append(h.Tags, name("description", m.Summary))
	}

	ogType := "website"
	if m.Type == "article" {
		ogType = "article"
	}
	h.Tags = append(h.Tags,
		property("og:site_name", siteName),
		property("og:title", m.Title),
		property("og:type", ogType),
		property("og:locale", locale),
	)
	if m.CanonicalURL != "" {
		h.Tags = append(h.Tags, property("og:url", m.CanonicalURL))
	}
	if m.Summary != "" {
		h.Tags = append(h.Tags, property("og:description", m.Summary))
	}
	if m.Image != "" {
		h.Tags = append(h.Tags, property("og:image", m.Image))
		if m.ImageWidth > 0 && m.ImageHeight > 0 {
			h.Tags = append(h.Tags,
				property("og:image:width", strconv.Itoa(m.ImageWidth)),
				property("og:image:height", strconv.Itoa(m.ImageHeight)),
			)
		}
		h.Tags = append(h.Tags, property("og:image:alt", imageAlt(m)))
	}
	if m.Type == "article" {
		h.Tags = append(h.Tags, property("article:author", authorURL))
		if !m.Published.IsZero() {
			published := m.Published.Format(time.RFC3339)
			updated := m.Updated.Format(time.RFC3339)
			h.Tags = append(h.Tags, property("article:published_time", published))
			if published != updated {
				h.Tags = append(h.Tags, property("article:modified_time", updated))
			}
		}
	}
	if m.Image != "" {
		h.Tags = append(h.Tags, name("twitter:card", "summary_large_image"))
	}

	j, err := renderJSONLD(graph(m))
	if err != nil {
		return Head{}, err
	}
	h.JSONLD = j

	return h, nil
}

// imageAlt describes the card of the page m describes, which draws the title
// and the site name.
func imageAlt(m site.Metadata) string {
	return m.Title + " - " + siteName
}

// graph returns what the page m describes says about itself: the site and its
// author, which every page states, the page, and the article it holds. A page
// without a URL states nothing: a node is named by a URL of the site.
func graph(m site.Metadata) jsonld.Graph {
	if m.CanonicalURL == "" {
		return jsonld.Graph{}
	}

	var g jsonld.Graph
	g.Nodes = append(g.Nodes,
		jsonld.WebSite{
			ID:         websiteID,
			URL:        siteURL,
			Name:       siteName,
			Publisher:  jsonld.Ref{ID: personID},
			InLanguage: language,
		},
		jsonld.Person{
			ID:     personID,
			Name:   authorName,
			URL:    authorURL,
			SameAs: profileURLs(),
		},
	)

	page := jsonld.WebPage{
		ID:          m.CanonicalURL + "#webpage",
		URL:         m.CanonicalURL,
		Name:        m.Title,
		Description: m.Summary,
		IsPartOf:    jsonld.Ref{ID: websiteID},
		InLanguage:  language,
	}
	// The site root is the first step of every breadcrumb, so a breadcrumb of
	// its own would hold one step and say nothing.
	var crumbs jsonld.BreadcrumbList
	if m.CanonicalURL != siteURL {
		crumbs = jsonld.BreadcrumbList{
			ID: m.CanonicalURL + "#breadcrumb",
			Items: []jsonld.ListItem{
				{Position: 1, Name: "Home", Item: siteURL},
				{Position: 2, Name: m.Title},
			},
		}
		page.Breadcrumb = jsonld.Ref{ID: crumbs.ID}
	}
	g.Nodes = append(g.Nodes, page)
	if crumbs.ID != "" {
		g.Nodes = append(g.Nodes, crumbs)
	}

	if m.Type == "article" {
		a := jsonld.Article{
			ID:               m.CanonicalURL + "#article",
			URL:              m.CanonicalURL,
			Headline:         m.Title,
			Description:      m.Summary,
			Author:           jsonld.Ref{ID: personID},
			Publisher:        jsonld.Ref{ID: personID},
			IsPartOf:         jsonld.Ref{ID: websiteID},
			MainEntityOfPage: jsonld.Ref{ID: page.ID},
			InLanguage:       language,
		}
		if m.Image != "" {
			a.Image = jsonld.ImageObject{
				URL:     m.Image,
				Width:   m.ImageWidth,
				Height:  m.ImageHeight,
				Caption: imageAlt(m),
			}
		}
		if !m.Published.IsZero() {
			a.DatePublished = m.Published.Format(time.RFC3339)
			a.DateModified = m.Updated.Format(time.RFC3339)
		}
		g.Nodes = append(g.Nodes, a)
	}

	return g
}

// renderJSONLD returns g as the JSON-LD a page carries, empty for a graph
// without nodes.
func renderJSONLD(g jsonld.Graph) (template.JS, error) {
	if len(g.Nodes) == 0 {
		return "", nil
	}
	// EscapeForHTML is what keeps a </script> in a title from ending the script
	// element the JSON-LD is written into.
	d, err := json.Marshal(g, jsontext.EscapeForHTML(true))
	if err != nil {
		return "", fmt.Errorf("marshaling JSON-LD: %v", err)
	}
	return template.JS(d), nil
}

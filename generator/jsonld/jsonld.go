// Package jsonld writes the schema.org description of a page as JSON-LD.
//
// A page describes itself as a [Graph]: a set of nodes that name each other by
// id, so the author of every article is one node in one place and not a copy in
// every page.
package jsonld

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

const context = "https://schema.org"

// Node is one node of a graph: a value this package writes a schema.org type
// for.
type Node interface {
	MarshalJSONTo(enc *jsontext.Encoder) error
}

// Graph is what a page says about itself.
type Graph struct {
	Nodes []Node
}

// Ref names a node by its id.
type Ref struct {
	ID string `json:"@id"`
}

// WebSite is the site a page is part of.
type WebSite struct {
	ID         string `json:"@id"`
	URL        string `json:"url"`
	Name       string `json:"name"`
	Publisher  Ref    `json:"publisher"`
	InLanguage string `json:"inLanguage"`
}

// WebPage is a page of the site.
type WebPage struct {
	ID          string `json:"@id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPartOf    Ref    `json:"isPartOf"`
	Breadcrumb  Ref    `json:"breadcrumb,omitzero"`
	InLanguage  string `json:"inLanguage"`
}

// Article is the article a page holds.
type Article struct {
	ID               string      `json:"@id"`
	URL              string      `json:"url"`
	Headline         string      `json:"headline"`
	Description      string      `json:"description,omitempty"`
	Image            ImageObject `json:"image,omitzero"`
	DatePublished    string      `json:"datePublished,omitempty"`
	DateModified     string      `json:"dateModified,omitempty"`
	Author           Ref         `json:"author"`
	Publisher        Ref         `json:"publisher"`
	IsPartOf         Ref         `json:"isPartOf"`
	MainEntityOfPage Ref         `json:"mainEntityOfPage"`
	InLanguage       string      `json:"inLanguage"`
}

// Person is who wrote the site.
type Person struct {
	ID     string   `json:"@id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	SameAs []string `json:"sameAs,omitempty"`
}

// ImageObject is an image, with the size it is drawn at.
type ImageObject struct {
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Caption string `json:"caption,omitempty"`
}

// BreadcrumbList is the path from the site root to a page.
type BreadcrumbList struct {
	ID    string     `json:"@id"`
	Items []ListItem `json:"itemListElement"`
}

// ListItem is one step of a [BreadcrumbList]. Item is the URL the step leads
// to, empty for the page the breadcrumb is on.
type ListItem struct {
	Position int    `json:"position"`
	Name     string `json:"name"`
	Item     string `json:"item,omitempty"`
}

// typed adds the schema.org type of T around it. T must not have a marshal
// method of its own, or marshaling recurses.
type typed[T any] struct {
	Type  string `json:"@type"`
	Inner T      `json:",embed"`
}

// The unexported types carry the fields without the marshal method.
type (
	webSite        WebSite
	webPage        WebPage
	article        Article
	person         Person
	imageObject    ImageObject
	breadcrumbList BreadcrumbList
	listItem       ListItem
)

func (g Graph) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, struct {
		Context string `json:"@context"`
		Graph   []Node `json:"@graph"`
	}{context, g.Nodes})
}

func (v WebSite) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[webSite]{"WebSite", webSite(v)})
}

func (v WebPage) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[webPage]{"WebPage", webPage(v)})
}

func (v Article) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[article]{"Article", article(v)})
}

func (v Person) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[person]{"Person", person(v)})
}

func (v ImageObject) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[imageObject]{"ImageObject", imageObject(v)})
}

func (v BreadcrumbList) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[breadcrumbList]{"BreadcrumbList", breadcrumbList(v)})
}

func (v ListItem) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typed[listItem]{"ListItem", listItem(v)})
}

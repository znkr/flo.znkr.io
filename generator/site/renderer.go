package site

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"golang.org/x/tools/blog/atom"
)

var passthrough = &passthroughRenderer{}

type renderer interface {
	render(site *Site, meta *Metadata, data []byte) ([]byte, error)
}

type passthroughRenderer struct{}

func (r *passthroughRenderer) render(site *Site, meta *Metadata, data []byte) ([]byte, error) {
	return data, nil
}

type markdownRenderer struct{}

func (r *markdownRenderer) render(site *Site, meta *Metadata, data []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Footnote),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	var buf bytes.Buffer
	if err := md.Convert(data, &buf); err != nil {
		return nil, fmt.Errorf("rendering markdown: %v", err)
	}

	return buf.Bytes(), nil
}

type templateRenderer struct {
	template *template.Template
}

func (r *templateRenderer) render(site *Site, meta *Metadata, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	err := r.template.Execute(&buf, struct {
		Meta    *Metadata
		Site    *Site
		Content template.HTML
	}{
		Meta:    meta,
		Site:    site,
		Content: template.HTML(data),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}

type feedRenderer struct{}

func (r *feedRenderer) render(site *Site, meta *Metadata, data []byte) ([]byte, error) {
	articles := site.Articles()
	updated := articles[0].meta.Updated

	feed := atom.Feed{
		Title:   meta.Title,
		ID:      "tag:znkr.io,2024:articles",
		Updated: atom.Time(updated),
		Link: []atom.Link{{
			Rel:  "self",
			Href: "https://flo.znkr.io/feed.atom",
		}},
	}

	for _, doc := range articles {
		html, err := site.RenderContent(doc)
		if err != nil {
			return nil, err
		}

		e := &atom.Entry{
			Title: doc.meta.Title,
			ID:    feed.ID + doc.path,
			Link: []atom.Link{{
				Rel:  "alternate",
				Href: "https://flo.znkr.io" + doc.path,
			}},
			Published: atom.Time(doc.meta.Published),
			Updated:   atom.Time(doc.meta.Updated),
			Summary: &atom.Text{
				Type: "html",
				Body: doc.meta.Abstract,
			},
			Content: &atom.Text{
				Type: "html",
				Body: string(html),
			},
			Author: &atom.Person{
				Name: "Florian Zenker",
			},
		}
		feed.Entry = append(feed.Entry, e)
	}

	b, err := xml.Marshal(feed)
	if err != nil {
		return nil, fmt.Errorf("encoding feed: %v", err)
	}
	return b, nil
}

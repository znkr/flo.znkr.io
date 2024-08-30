package atom

import (
	"encoding/xml"
	"fmt"

	"flo.znkr.io/generator/site"
	"golang.org/x/tools/blog/atom"
)

var Renderer site.Renderer = &renderer{}

type renderer struct{}

func (r *renderer) Render(s *site.Site, doc *site.Doc, data []byte) ([]byte, error) {
	articles := s.Articles()
	updated := articles[0].Meta.Updated

	feed := atom.Feed{
		Title:   doc.Meta.Title,
		ID:      "tag:znkr.io,2024:articles",
		Updated: atom.Time(updated),
		Link: []atom.Link{{
			Rel:  "self",
			Href: "https://flo.znkr.io/feed.atom",
		}},
	}

	for _, doc := range articles {
		html, err := s.RenderContent(doc)
		if err != nil {
			return nil, err
		}

		e := &atom.Entry{
			Title: doc.Meta.Title,
			ID:    feed.ID + doc.Path,
			Link: []atom.Link{{
				Rel:  "alternate",
				Href: "https://flo.znkr.io" + doc.Path,
			}},
			Published: atom.Time(doc.Meta.Published),
			Updated:   atom.Time(doc.Meta.Updated),
			Summary: &atom.Text{
				Type: "html",
				Body: doc.Meta.Abstract,
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

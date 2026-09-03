package renderers

import (
	"encoding/xml"
	"fmt"

	"golang.org/x/tools/blog/atom"
)

// feedID is the feed's tag URI and the prefix of every entry's id.
const feedID = "tag:znkr.io,2024:articles"

// RenderAtom renders the feed from entries, of which it carries the published
// articles. contents holds each entry's rendered body, in the same order, which
// is what the feed embeds.
func RenderAtom(title string, entries []Entry, contents [][]byte) ([]byte, error) {
	body := make(map[string][]byte, len(entries))
	for i, e := range entries {
		body[e.Path] = contents[i]
	}

	articles := Articles(entries)
	if len(articles) == 0 {
		return nil, fmt.Errorf("no articles to build a feed from")
	}

	feed := atom.Feed{
		Title:   title,
		ID:      feedID,
		Updated: atom.Time(articles[0].Meta.Updated),
		Link: []atom.Link{{
			Rel:  "self",
			Href: "https://flo.znkr.io/feed.atom",
		}},
	}

	for _, a := range articles {
		e := &atom.Entry{
			Title: a.Meta.Title,
			ID:    feed.ID + a.Path,
			Link: []atom.Link{{
				Rel:  "alternate",
				Href: "https://flo.znkr.io" + a.Path,
			}},
			Published: atom.Time(a.Meta.Published),
			Updated:   atom.Time(a.Meta.Updated),
			Summary: &atom.Text{
				Type: "html",
				Body: a.Meta.Summary,
			},
			Content: &atom.Text{
				Type: "html",
				Body: string(body[a.Path]),
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

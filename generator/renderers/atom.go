package renderers

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"golang.org/x/tools/blog/atom"
)

// feedID is the feed's tag URI and the prefix of every entry's id.
const feedID = "tag:znkr.io,2024:articles"

// RenderAtom renders the feed, which carries articles in order and is as recent
// as the newest date of a published article. contents holds the rendered body
// of each of entries, in the same order, which is what the feed embeds for an
// article.
func RenderAtom(title string, articles []Entry, entries []Entry, contents [][]byte) ([]byte, error) {
	body := make(map[string][]byte, len(entries))
	for i, e := range entries {
		body[e.Path] = contents[i]
	}

	if len(articles) == 0 {
		return nil, fmt.Errorf("no articles to build a feed from")
	}

	// A draft has no dates, and the newest change can be an update to an older
	// article.
	var updated time.Time
	for _, a := range articles {
		if a.Meta.Draft() {
			continue
		}
		for _, t := range []time.Time{a.Meta.Published, a.Meta.Updated} {
			if t.After(updated) {
				updated = t
			}
		}
	}

	feed := atom.Feed{
		Title:   title,
		ID:      feedID,
		Updated: atom.Time(updated),
		Link: []atom.Link{{
			Rel:  "self",
			Href: "https://flo.znkr.io/feed.atom",
		}},
	}

	for _, a := range articles {
		e := &atom.Entry{
			Title: a.Meta.Title,
			// An id must not change once a feed reader has seen it, and the
			// published ids carry the path without its trailing slash.
			ID: feed.ID + strings.TrimSuffix(a.Path, "/"),
			Link: []atom.Link{{
				Rel:  "alternate",
				Href: a.Meta.CanonicalURL,
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
				Name: authorName,
				URI:  authorURL,
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

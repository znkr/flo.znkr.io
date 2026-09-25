package jsonld

import (
	"encoding/json/v2"
	"testing"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{
			name: "person",
			in: Person{
				ID:     "https://flo.znkr.io/about#me",
				Name:   "Florian Zenker",
				URL:    "https://flo.znkr.io/about",
				SameAs: []string{"https://github.com/znkr"},
			},
			want: `{"@type":"Person","@id":"https://flo.znkr.io/about#me","name":"Florian Zenker","url":"https://flo.znkr.io/about","sameAs":["https://github.com/znkr"]}`,
		},
		{
			// A page states the nodes of one graph, which name each other by
			// id. Only the graph carries the context.
			name: "graph",
			in: Graph{Nodes: []Node{
				WebSite{
					ID:         "https://flo.znkr.io/#website",
					URL:        "https://flo.znkr.io/",
					Name:       "flo.znkr.io",
					Publisher:  Ref{"https://flo.znkr.io/about#me"},
					InLanguage: "en",
				},
				WebPage{
					ID:         "https://flo.znkr.io/hello/#webpage",
					URL:        "https://flo.znkr.io/hello/",
					Name:       "Hello",
					IsPartOf:   Ref{"https://flo.znkr.io/#website"},
					Breadcrumb: Ref{"https://flo.znkr.io/hello/#breadcrumb"},
					InLanguage: "en",
				},
			}},
			want: `{"@context":"https://schema.org","@graph":[` +
				`{"@type":"WebSite","@id":"https://flo.znkr.io/#website","url":"https://flo.znkr.io/","name":"flo.znkr.io","publisher":{"@id":"https://flo.znkr.io/about#me"},"inLanguage":"en"},` +
				`{"@type":"WebPage","@id":"https://flo.znkr.io/hello/#webpage","url":"https://flo.znkr.io/hello/","name":"Hello","isPartOf":{"@id":"https://flo.znkr.io/#website"},"breadcrumb":{"@id":"https://flo.znkr.io/hello/#breadcrumb"},"inLanguage":"en"}]}`,
		},
		{
			name: "article",
			in: Article{
				ID:            "https://flo.znkr.io/hello/#article",
				URL:           "https://flo.znkr.io/hello/",
				Headline:      "Hello",
				Description:   "A summary.",
				Image:         ImageObject{URL: "https://flo.znkr.io/hello/card.png", Width: 1200, Height: 630, Caption: "Hello - flo.znkr.io"},
				DatePublished: "2026-01-02T00:00:00Z",
				DateModified:  "2026-01-03T00:00:00Z",
				Author:        Ref{"https://flo.znkr.io/about#me"},
				Publisher:     Ref{"https://flo.znkr.io/about#me"},
				IsPartOf:      Ref{"https://flo.znkr.io/#website"},
				InLanguage:    "en",
			},
			want: `{"@type":"Article","@id":"https://flo.znkr.io/hello/#article","url":"https://flo.znkr.io/hello/","headline":"Hello","description":"A summary.",` +
				`"image":{"@type":"ImageObject","url":"https://flo.znkr.io/hello/card.png","width":1200,"height":630,"caption":"Hello - flo.znkr.io"},` +
				`"datePublished":"2026-01-02T00:00:00Z","dateModified":"2026-01-03T00:00:00Z","author":{"@id":"https://flo.znkr.io/about#me"},` +
				`"publisher":{"@id":"https://flo.znkr.io/about#me"},"isPartOf":{"@id":"https://flo.znkr.io/#website"},"mainEntityOfPage":{"@id":""},"inLanguage":"en"}`,
		},
		{
			// An article without an image or dates leaves them out, and so does
			// a page without a breadcrumb.
			name: "empty fields",
			in: Article{
				ID:       "https://flo.znkr.io/hello/#article",
				URL:      "https://flo.znkr.io/hello/",
				Headline: "Hello",
			},
			want: `{"@type":"Article","@id":"https://flo.znkr.io/hello/#article","url":"https://flo.znkr.io/hello/","headline":"Hello",` +
				`"author":{"@id":""},"publisher":{"@id":""},"isPartOf":{"@id":""},"mainEntityOfPage":{"@id":""},"inLanguage":""}`,
		},
		{
			name: "breadcrumbs",
			in: BreadcrumbList{
				ID: "https://flo.znkr.io/hello/#breadcrumb",
				Items: []ListItem{
					{Position: 1, Name: "Home", Item: "https://flo.znkr.io/"},
					{Position: 2, Name: "Hello"},
				},
			},
			want: `{"@type":"BreadcrumbList","@id":"https://flo.znkr.io/hello/#breadcrumb","itemListElement":[` +
				`{"@type":"ListItem","position":1,"name":"Home","item":"https://flo.znkr.io/"},` +
				`{"@type":"ListItem","position":2,"name":"Hello"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal() = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

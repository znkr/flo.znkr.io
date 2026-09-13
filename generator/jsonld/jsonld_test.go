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
			in: &Person{
				Name: "Florian Zenker",
				URL:  "https://flo.znkr.io/about",
			},
			want: `{"@context":"https://schema.org","@type":"Person","name":"Florian Zenker","url":"https://flo.znkr.io/about"}`,
		},
		{
			name: "article",
			in: &Article{
				Headline: "Hello",
				Author: []Person{{
					Name: "Florian Zenker",
					URL:  "https://flo.znkr.io/about",
				}},
				DatePublished: "2026-01-02",
				DateModified:  "2026-01-03",
				URL:           "https://flo.znkr.io/hello",
			},
			want: `{"@context":"https://schema.org","@type":"Article","headline":"Hello","author":[{"@context":"https://schema.org","@type":"Person","name":"Florian Zenker","url":"https://flo.znkr.io/about"}],"datePublished":"2026-01-02","dateModified":"2026-01-03","url":"https://flo.znkr.io/hello"}`,
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

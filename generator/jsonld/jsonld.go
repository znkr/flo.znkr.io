package jsonld

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

type Article struct {
	Headline      string   `json:"headline"`
	Author        []Person `json:"author"`
	DatePublished string   `json:"datePublished"`
	DateModified  string   `json:"dateModified"`
	URL           string   `json:"url"`
}

type Person struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// outer adds the schema.org envelope around T. T must not have a marshal
// method of its own, or marshaling recurses.
type outer[T any] struct {
	Context string `json:"@context"`
	Type    string `json:"@type"`
	Inner   T      `json:",embed"`
}

// article and person carry the fields without the marshal method.
type article Article
type person Person

func (a Article) MarshalJSONTo(enc *jsontext.Encoder) error {
	o := outer[article]{
		Context: "https://schema.org",
		Type:    "Article",
		Inner:   article(a),
	}
	return json.MarshalEncode(enc, o)
}

func (p Person) MarshalJSONTo(enc *jsontext.Encoder) error {
	o := outer[person]{
		Context: "https://schema.org",
		Type:    "Person",
		Inner:   person(p),
	}
	return json.MarshalEncode(enc, o)
}

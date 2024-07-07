package site

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadMetadata(t *testing.T) {
	in := `~~~
        title: the title
     abstract: multiple lines
               with 

               an empty line in between
~~~
Hello World!
`
	gotm, gotd, err := parseMetadata([]byte(in))
	if err != nil {
		t.Fatal(err)
	}

	wantm := &Metadata{
		Title:    "the title",
		Abstract: "multiple lines\nwith \nan empty line in between",
	}

	wantd := []byte("Hello World!\n")

	if diff := cmp.Diff(wantm, gotm); diff != "" {
		t.Errorf("metadata is different [-want,+got]:\n%s", diff)
	}

	if diff := cmp.Diff(wantd, gotd); diff != "" {
		t.Errorf("data is different [-want,+got]:\n%s", diff)
	}
}

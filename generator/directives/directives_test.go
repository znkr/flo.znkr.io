package directives

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Directive
	}{
		{
			name: "no_directives",
			in:   "there\nare\nno\ndirectives\nhere",
			want: nil,
		},
		{
			name: "inline-snippet-directive",
			in: `there's an inline directive in here
				 <!--#inline-snippet file="my/file.go" -->
				`,
			want: []Directive{
				{
					Pos:  41,
					End:  82,
					Name: "inline-snippet",
					Attrs: map[string]string{
						"file": "my/file.go",
					},
				},
			},
		},
		{
			name: "metadata",
			in: `# Some markdown
			<!--#meta
				published="1234-56-78"
				summary="""
					multiple
					lines
					"""
			-->
			`,
			want: []Directive{
				{
					Pos:  19,
					End:  112,
					Name: "meta",
					Attrs: map[string]string{
						"published": "1234-56-78",
						"summary":   "\nmultiple\nlines\n",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("unexpected diff [-want,+got]:\n%s", diff)
			}
		})
	}
}

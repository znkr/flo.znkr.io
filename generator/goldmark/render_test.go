package goldmark

import (
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		in   string
		opts cmp.Option
		want string
	}{
		{
			name: "simple_paragraph",
			in:   "Hello, world!",
			want: "<p>Hello, world!</p>\n",
		},
		{
			name: "heading_with_anchor",
			in:   "# My Title",
			want: `<h1 id="my-title">My Title<a href="#my-title" class="anchor-link"></a></h1>` + "\n",
		},
		// Regression test for https://github.com/znkr/flo.znkr.io/issues/13
		{
			name: "math_preserves_surrounding_text",
			in:   `The complexity is $\mathcal{O}(ND)$ where $N$ is the size.`,
			opts: cmp.Transformer("normalizeMath", func(s string) string {
				return regexp.MustCompile(`<math[^>]*>[\s\S]*?</math>`).ReplaceAllString(s, "<math>...</math>")
			}),
			want: "<p>The complexity is \n<math>...</math>\n where \n<math>...</math>\n is the size.</p>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := Render([]byte(tt.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, string(got), tt.opts); diff != "" {
				t.Errorf("Render() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRender_TOC(t *testing.T) {
	input := `# Title

## Section One

Content.

## Section Two

More content.

### Subsection

Even more content.
`
	_, toc, err := Render([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := `<ul>
<li>
<a href="#section-one">Section One</a></li>
<li>
<a href="#section-two">Section Two</a><ul>
<li>
<a href="#subsection">Subsection</a></li>
</ul>
</li>
</ul>
`
	if diff := cmp.Diff(want, string(toc)); diff != "" {
		t.Errorf("TOC mismatch (-want +got):\n%s", diff)
	}
}

package directives

import (
	"strings"
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
			name: "empty_input",
			in:   "",
			want: nil,
		},
		{
			name: "only_whitespace",
			in:   "   \n\t\n   ",
			want: nil,
		},
		{
			name: "partial_comment_not_directive",
			in:   "<!-- this is a regular comment -->",
			want: nil,
		},
		{
			name: "multiple_directives",
			in: `first <!--#inline-snippet file="a.go" -->
second <!--#inline-snippet file="b.go" -->`,
			want: []Directive{
				{
					Pos:  6,
					End:  41,
					Name: "inline-snippet",
					Attrs: map[string]string{
						"file": "a.go",
					},
				},
				{
					Pos:  49,
					End:  84,
					Name: "inline-snippet",
					Attrs: map[string]string{
						"file": "b.go",
					},
				},
			},
		},
		{
			name: "directive_with_multiple_attrs",
			in:   `<!--#include-snippet file="test.go" lines="1..10" lang="go" -->`,
			want: []Directive{
				{
					Pos:  0,
					End:  63,
					Name: "include-snippet",
					Attrs: map[string]string{
						"file":  "test.go",
						"lines": "1..10",
						"lang":  "go",
					},
				},
			},
		},
		{
			name: "directive_with_hyphenated_name",
			in:   `<!--#include-diff a="old.go" b="new.go" -->`,
			want: []Directive{
				{
					Pos:  0,
					End:  43,
					Name: "include-diff",
					Attrs: map[string]string{
						"a": "old.go",
						"b": "new.go",
					},
				},
			},
		},
		{
			name: "directive_with_empty_attr_value",
			in:   `<!--#test attr="" -->`,
			want: []Directive{
				{
					Pos:  0,
					End:  21,
					Name: "test",
					Attrs: map[string]string{
						"attr": "",
					},
				},
			},
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

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{
			name:    "unterminated_string",
			in:      `<!--#test attr="unterminated -->`,
			wantErr: "unterminated string",
		},
		{
			name:    "unterminated_string_newline",
			in:      "<!--#test attr=\"value\n\" -->",
			wantErr: "unterminated string",
		},
		{
			name:    "unterminated_tri_quoted_string",
			in:      `<!--#test attr="""unterminated`,
			wantErr: "unterminated tri-quoted string",
		},
		{
			name:    "missing_equals",
			in:      `<!--#test attr "value" -->`,
			wantErr: "expected '='",
		},
		{
			name:    "missing_value_quote",
			in:      `<!--#test attr=value -->`,
			wantErr: `expected '"'`,
		},
		{
			name:    "missing_closing_tag",
			in:      `<!--#test attr="value"`,
			wantErr: "expected '-->'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.in))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			syntaxErr, ok := err.(*SyntaxError)
			if !ok {
				t.Fatalf("expected *SyntaxError, got %T", err)
			}
			if !strings.Contains(syntaxErr.Msg, tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, syntaxErr.Msg)
			}
		})
	}
}

func TestHasAttr(t *testing.T) {
	d := Directive{
		Attrs: map[string]string{
			"file": "test.go",
			"lang": "go",
		},
	}

	if !d.HasAttr("file") {
		t.Error("HasAttr('file') should return true")
	}
	if !d.HasAttr("lang") {
		t.Error("HasAttr('lang') should return true")
	}
	if d.HasAttr("missing") {
		t.Error("HasAttr('missing') should return false")
	}
}

func TestSyntaxError_Error(t *testing.T) {
	err := &SyntaxError{
		Msg:  "test error",
		Pos:  42,
		Line: 3,
		Col:  10,
	}
	want := "test error [3:10]"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

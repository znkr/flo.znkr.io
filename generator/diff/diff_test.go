package diff

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name string
		x, y []string
		want []Edit
	}{
		{
			name: "identical",
			x:    []string{"foo", "bar", "baz"},
			y:    []string{"foo", "bar", "baz"},
			want: []Edit{
				{Match, "foo"},
				{Match, "bar"},
				{Match, "baz"},
			},
		},
		{
			name: "empty",
		},
		{
			name: "x-empty",
			y:    []string{"foo", "bar", "baz"},
			want: []Edit{
				{Insert, "foo"},
				{Insert, "bar"},
				{Insert, "baz"},
			},
		},
		{
			name: "y-empty",
			x:    []string{"foo", "bar", "baz"},
			want: []Edit{
				{Delete, "foo"},
				{Delete, "bar"},
				{Delete, "baz"},
			},
		},
		{
			name: "ABCABBA_to_CBABAC",
			x:    strings.Split("ABCABBA", ""),
			y:    strings.Split("CBABAC", ""),
			want: []Edit{
				{Delete, "A"},
				{Delete, "B"},
				{Match, "C"},
				{Insert, "B"},
				{Match, "A"},
				{Match, "B"},
				{Delete, "B"},
				{Match, "A"},
				{Insert, "C"},
			},
		},
		{
			name: "same-prefix",
			x:    []string{"foo", "bar"},
			y:    []string{"foo", "baz"},
			want: []Edit{
				{Match, "foo"},
				{Delete, "bar"},
				{Insert, "baz"},
			},
		},
		{
			name: "same-suffix",
			x:    []string{"foo", "bar"},
			y:    []string{"loo", "bar"},
			want: []Edit{
				{Delete, "foo"},
				{Insert, "loo"},
				{Match, "bar"},
			},
		},
		{
			name: "slide-around",
			x:    []string{"a", "a"},
			y:    []string{"a", "b", "a", "b", "a"},
			want: []Edit{
				{Match, "a"},
				{Insert, "b"},
				{Insert, "a"},
				{Insert, "b"},
				{Match, "a"},
			},
		},
		{
			name: "realistic-example",
			x: []string{
				"func f() int {\n",
				"\treturn 0\n",
				"}\n",
			},
			y: []string{
				"func f() int {\n",
				"\treturn 0\n",
				"}\n",
				"\n",
				"func g() int {\n",
				"\treturn 42\n",
				"}\n",
			},
			want: []Edit{
				{Match, "func f() int {\n"},
				{Match, "\treturn 0\n"},
				{Match, "}\n"},
				{Insert, "\n"},
				{Insert, "func g() int {\n"},
				{Insert, "\treturn 42\n"},
				{Insert, "}\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diff(tt.x, tt.y)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Diff result is different (-want, +got):\n%s", diff)
			}
		})
	}
}

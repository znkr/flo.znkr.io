package metadata

import (
	"testing"
	"time"

	"flo.znkr.io/generator/site"
	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		meta *site.Metadata
		rest string
	}{
		{
			name: "empty",
			in:   "",
			meta: &site.Metadata{
				Article: true,
			},
			rest: "",
		},
		{
			name: "title only",
			in:   "# My Title\n",
			meta: &site.Metadata{
				Title:   "My Title",
				Article: true,
			},
			rest: "",
		},
		{
			name: "title with extra newline",
			in:   "# My Title\n\n",
			meta: &site.Metadata{
				Title:   "My Title",
				Article: true,
			},
			rest: "",
		},
		{
			name: "title with content",
			in:   "# My Title\ncontent",
			meta: &site.Metadata{
				Title:   "My Title",
				Article: true,
			},
			rest: "content",
		},
		{
			name: "title with content after blank line",
			in:   "# My Title\n\ncontent",
			meta: &site.Metadata{
				Title:   "My Title",
				Article: true,
			},
			rest: "content",
		},
		{
			name: "title with metadata no newline",
			in:   "# My Title\n:published: 2024-09-12",
			meta: &site.Metadata{
				Title:     "My Title",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Article:   true,
			},
			rest: "",
		},
		{
			name: "title with metadata",
			in:   "# My Title\n:published: 2024-09-12\n",
			meta: &site.Metadata{
				Title:     "My Title",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Article:   true,
			},
			rest: "",
		},
		{
			name: "title with metadata and content",
			in:   "# My Title\n:published: 2024-09-12\ncontent",
			meta: &site.Metadata{
				Title:     "My Title",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Article:   true,
			},
			rest: "content",
		},
		{
			name: "title with metadata and content after blank line",
			in:   "# My Title\n:published: 2024-09-12\n\ncontent",
			meta: &site.Metadata{
				Title:     "My Title",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Article:   true,
			},
			rest: "content",
		},
		{
			name: "full example",
			in:   "# My Title\n:published: 2024-09-12\n:summary: this\\\nis\\\nmy\\\nsummary\n:article: false\n\ncontent",
			meta: &site.Metadata{
				Title:     "My Title",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Article:   false,
				Abstract:  "this\nis\nmy\nsummary",
			},
			rest: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, rest, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(meta, tt.meta); diff != "" {
				t.Errorf("different metadata [-got,+want]:\n%s", diff)
			}

			if diff := cmp.Diff(string(rest), tt.rest); diff != "" {
				t.Errorf("different rest [-got,+want]:\n%s", diff)
			}
		})
	}
}

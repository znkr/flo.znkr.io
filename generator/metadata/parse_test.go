package metadata

import (
	"strings"
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
				Type: "article",
			},
			rest: "",
		},
		{
			name: "title only",
			in:   "# My Title\n",
			meta: &site.Metadata{
				Title: "My Title",
				Type:  "article",
			},
			rest: "",
		},
		{
			name: "title with extra newline",
			in:   "# My Title\n\n",
			meta: &site.Metadata{
				Title: "My Title",
				Type:  "article",
			},
			rest: "",
		},
		{
			name: "title with content",
			in:   "# My Title\ncontent",
			meta: &site.Metadata{
				Title: "My Title",
				Type:  "article",
			},
			rest: "content",
		},
		{
			name: "title with content after blank line",
			in:   "# My Title\n\ncontent",
			meta: &site.Metadata{
				Title: "My Title",
				Type:  "article",
			},
			rest: "content",
		},
		{
			name: "title with metadata no newline",
			in:   "# My Title\n:published: 2024-09-12",
			meta: &site.Metadata{
				Title:     "My Title",
				Type:      "article",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
			},
			rest: "",
		},
		{
			name: "title with metadata",
			in:   "# My Title\n:published: 2024-09-12\n",
			meta: &site.Metadata{
				Title:     "My Title",
				Type:      "article",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
			},
			rest: "",
		},
		{
			name: "title with metadata and content",
			in:   "# My Title\n:published: 2024-09-12\ncontent",
			meta: &site.Metadata{
				Title:     "My Title",
				Type:      "article",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
			},
			rest: "content",
		},
		{
			name: "title with metadata and content after blank line",
			in:   "# My Title\n:published: 2024-09-12\n\ncontent",
			meta: &site.Metadata{
				Title:     "My Title",
				Type:      "article",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
			},
			rest: "content",
		},
		{
			name: "full example",
			in:   "# My Title\n:published: 2024-09-12\n:summary: this\\\nis\\\nmy\\\nsummary\n:type: page\n\ncontent",
			meta: &site.Metadata{
				Title:     "My Title",
				Published: time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.September, 12, 0, 0, 0, 0, tz),
				Type:      "page",
				Abstract:  "this\nis\nmy\nsummary",
			},
			rest: "content",
		},
		{
			name: "updated_different_from_published",
			in:   "# Title\n:published: 2024-01-01\n:updated: 2024-06-15\n",
			meta: &site.Metadata{
				Title:     "Title",
				Published: time.Date(2024, time.January, 1, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.June, 15, 0, 0, 0, 0, tz),
				Type:      "article",
			},
			rest: "",
		},
		{
			name: "go_import_metadata",
			in:   "# Package\n:go-import: example.com/pkg git https://github.com/user/pkg\n",
			meta: &site.Metadata{
				Title:    "Package",
				Type:     "article",
				GoImport: "example.com/pkg git https://github.com/user/pkg",
			},
			rest: "",
		},
		{
			name: "redirect_metadata",
			in:   "# Redirect\n:redirect: https://example.com/new-url\n",
			meta: &site.Metadata{
				Title:    "Redirect",
				Type:     "article",
				Redirect: "https://example.com/new-url",
			},
			rest: "",
		},
		{
			name: "no_title_only_content",
			in:   "some content without a title",
			meta: &site.Metadata{
				Type: "article",
			},
			rest: "some content without a title",
		},
		{
			name: "multiple_blank_lines_stripped",
			in:   "# Title\n:published: 2024-01-01\n\n\n\ncontent after blanks",
			meta: &site.Metadata{
				Title:     "Title",
				Published: time.Date(2024, time.January, 1, 0, 0, 0, 0, tz),
				Updated:   time.Date(2024, time.January, 1, 0, 0, 0, 0, tz),
				Type:      "article",
			},
			rest: "content after blanks",
		},
		{
			name: "title_with_special_chars",
			in:   "# My Title: With \"Quotes\" & Symbols!\n",
			meta: &site.Metadata{
				Title: "My Title: With \"Quotes\" & Symbols!",
				Type:  "article",
			},
			rest: "",
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

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{
			name:    "invalid_published_date",
			in:      "# Title\n:published: not-a-date\n",
			wantErr: "parsing published",
		},
		{
			name:    "invalid_updated_date",
			in:      "# Title\n:updated: 2024-99-99\n",
			wantErr: "parsing updated",
		},
		{
			name:    "invalid_date_format",
			in:      "# Title\n:published: 12/25/2024\n",
			wantErr: "parsing published",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse([]byte(tt.in))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

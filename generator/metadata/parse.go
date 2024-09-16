package metadata

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"flo.znkr.io/generator/site"
)

// Parse extracts the metadata header from in, if any. The metadata header has the following format
//
//	# <title>
//	:<key>: <value>
//	:<key>: <value>
//
// It returns the parsed [site.Metadata] and the remaining input data (i.e. everything after the
// metadata header).
func Parse(in []byte) (*site.Metadata, []byte, error) {
	meta := site.Metadata{}

	// Take title from first header. This assumes that every document starts with the header
	// and doesn't have anything before it.
	if len(in) > 2 && in[0] == '#' && in[1] == ' ' {
		eol := slices.Index(in, '\n')
		if eol < 0 {
			return nil, in, nil
		}
		meta.Title = strings.TrimSpace(string(in[1:eol]))
		in = in[eol+1:]
	}

	// Parse metadata lines. These lines follow a simple format:
	//   :<key>: <value>
	metadir := make(map[string]string)
	for len(in) > 0 && in[0] == ':' {
		pos := 1
		end := pos + slices.Index(in[pos:], ':')
		if end < pos {
			break
		}

		key := string(in[pos:end])

		var val strings.Builder
		for {
			pos = end + 1
			if pos >= len(in) {
				break
			}
			if eol := slices.Index(in[pos:], '\n'); eol < 0 {
				end = len(in)
			} else {
				end = pos + eol
			}
			if in[end-1] == '\\' {
				val.Write(in[pos : end-1])
				val.WriteByte('\n')
			} else {
				val.Write(in[pos:end])
				break
			}
		}
		if end < len(in) {
			in = in[end+1:]
		} else {
			in = nil
		}

		metadir[key] = strings.TrimSpace(val.String())
	}

	// Strip blank lines.
	for len(in) > 0 && in[0] == '\n' {
		in = in[1:]
	}

	parseTime := func(key string) (time.Time, error) {
		v, ok := metadir[key]
		if !ok {
			return time.Time{}, nil
		}
		t, err := time.ParseInLocation("2006-01-02", v, tz)
		if err != nil {
			return time.Time{}, fmt.Errorf("parsing %s: %v", key, err)
		}
		return t, nil
	}
	published, err := parseTime("published")
	if err != nil {
		return nil, nil, err
	}
	updated, err := parseTime("updated")
	if err != nil {
		return nil, nil, err
	}
	if updated.IsZero() {
		updated = published
	}

	meta.Published = published
	meta.Updated = updated
	meta.Abstract = metadir["summary"]
	meta.GoImport = metadir["go-import"]
	meta.Redirect = metadir["redirect"]
	meta.Template = metadir["template"]
	meta.Article = metadir["article"] != "false"
	return &meta, in, nil
}

var tz *time.Location

func init() {
	var err error
	tz, err = time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
}

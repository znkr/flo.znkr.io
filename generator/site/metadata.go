package site

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func parseMetadata(in []byte) (*Metadata, []byte, error) {
	const magic = "~~~\n"

	var errs []error
	var ch rune
	col := 0
	line := 1
	errorf := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}
	next := func() {
		if len(errs) > 0 {
			ch = -1
			return
		}
		ch0, w := utf8.DecodeRune(in)
		in = in[w:]
		if ch0 == utf8.RuneError {
			errorf("invalid UTF-8 [%d:%d]", line, col)
			ch = -1
			return
		}
		if ch == '\n' {
			line++
			col = 0
		}
		col += w
		ch = ch0
	}
	isnext := func(s string) bool {
		if len(errs) > 0 {
			return false
		}

		if len(in) < len(s) {
			return false
		}
		found := bytes.Equal(in[:len(s)], []byte(s))
		if found {
			in = in[len(s):]
		}
		return found
	}

	if !isnext(magic) {
		line++
		return nil, in, nil
	}

	meta := make(map[string]string)
	for {
		next()
		if len(errs) > 0 {
			goto Done
		}

		// Consume whitespace
		for unicode.IsSpace(ch) {
			if ch == '\t' {
				errorf("invalid format, found a tab character [%d:%d]", line, col)
			}
			next()
		}

		// Parse identifier
		if !unicode.IsLetter(ch) {
			errorf("invalid format, expected identifier, got %q [%d:%d]", string(ch), line, col)
			goto Done
		}
		var sb strings.Builder
		for unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '-' || ch == '_' {
			sb.WriteRune(ch)
			next()
		}
		if ch != ':' {
			errorf("invalid format, expected \":\", got %q [%d:%d]", string(ch), line, col)
			goto Done
		}
		key := sb.String()
		sb.Reset()
		next()

		// Consume whitespace
		for unicode.IsSpace(ch) {
			next()
		}

		// Parse Value
		ident := col - 1
		for {
			if ch == '\n' {
				if isnext("\n") {
					continue
				}
				if isnext(strings.Repeat(" ", ident)) {
					// continuation from the line before
					sb.WriteRune('\n')
					next()
					continue
				}
				break
			}
			sb.WriteRune(ch)
			next()
		}
		value := sb.String()
		meta[key] = value
		if ch == '\n' && isnext(magic) {
			goto Done
		}
	}

Done:
	parseTime := func(key string) time.Time {
		v, ok := meta[key]
		if !ok {
			return time.Time{}
		}
		t, err := time.ParseInLocation("2006-01-02", v, tz)
		if err != nil {
			errorf("parsing %s: %v", key, err)
		}
		return t
	}
	published := parseTime("published")
	updated := parseTime("updated")
	if updated.IsZero() {
		updated = published
	}

	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	return &Metadata{
		Title:     meta["title"],
		Published: published,
		Updated:   updated,
		Abstract:  meta["abstract"],
		GoImport:  meta["go-import"],
		Redirect:  meta["redirect"],
		Template:  meta["template"],
		Article:   meta["article"] != "false",
	}, in, nil
}

var tz *time.Location

func init() {
	var err error
	tz, err = time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
}

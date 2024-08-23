package site

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"flo.znkr.io/generator/directives"
	"flo.znkr.io/generator/highlight"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"golang.org/x/tools/blog/atom"
)

type renderer interface {
	render(s *Site, doc *Doc, data []byte) ([]byte, error)
}

func chain(a, b renderer) renderer {
	var ret chainedRenderer
	if as, ok := a.(chainedRenderer); ok {
		ret = append(ret, as...)
	} else {
		ret = append(ret, a)
	}
	if bs, ok := b.(chainedRenderer); ok {
		ret = append(ret, bs...)
	} else {
		ret = append(ret, b)
	}
	return ret
}

type passthroughRenderer struct{}

func (r *passthroughRenderer) render(_ *Site, _ *Doc, data []byte) ([]byte, error) {
	return data, nil
}

type chainedRenderer []renderer

func (r chainedRenderer) render(site *Site, doc *Doc, data []byte) ([]byte, error) {
	for _, r := range r {
		var err error
		data, err = r.render(site, doc, data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

type markdownRenderer struct{}

func (r *markdownRenderer) render(_ *Site, _ *Doc, data []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Footnote),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	var buf bytes.Buffer
	if err := md.Convert(data, &buf); err != nil {
		return nil, fmt.Errorf("rendering markdown: %v", err)
	}

	return buf.Bytes(), nil
}

type directivesRenderer struct{}

func (r *directivesRenderer) render(s *Site, doc *Doc, data []byte) ([]byte, error) {
	dirs, err := directives.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse directives: %v", err)
	}

	if len(dirs) == 0 {
		return data, nil
	}

	var buf bytes.Buffer
	pos := 0
	for _, dir := range dirs {
		buf.Write(data[pos:dir.Pos])

		switch dir.Name {
		case "meta":
			// nothing to do

		case "include-snippet":
			file := dir.Attrs["file"]
			if file == "" {
				return nil, fmt.Errorf("include-snippet: missing or empty file attribute")
			}
			b, err := os.ReadFile(filepath.Join(doc.Dir(), file))
			if err != nil {
				return nil, fmt.Errorf("include-snippet: %v", err)
			}

			lopt := highlight.LangFromFilename(file)
			if lang, ok := dir.Attrs["lang"]; ok {
				lopt = highlight.Lang(lang)
			}
			lines, err := highlight.Highlight(string(b), lopt)
			if err != nil {
				return nil, fmt.Errorf("include-snippet: %v", err)
			}

			display := cmp.Or(dir.Attrs["display"], file)
			t := s.templates.Lookup("fragments/include_snippet")
			err = t.Execute(&buf, struct {
				File     string
				FilePath string
				Lines    []highlight.Line
			}{
				File:     display,
				FilePath: filepath.Join(doc.Path(), file),
				Lines:    lines,
			})
			if err != nil {
				return nil, fmt.Errorf("rendering include-snipped: %v", err)
			}

		case "include-diff":
			afile := dir.Attrs["a"]
			if afile == "" {
				return nil, fmt.Errorf("include-diff: missing or empty file attribute")
			}
			a, err := os.ReadFile(filepath.Join(doc.Dir(), afile))
			if err != nil {
				return nil, fmt.Errorf("include-snippet: %v", err)
			}

			bfile := dir.Attrs["b"]
			if afile == "" {
				return nil, fmt.Errorf("include-diff: missing or empty file attribute")
			}
			b, err := os.ReadFile(filepath.Join(doc.Dir(), bfile))
			if err != nil {
				return nil, fmt.Errorf("include-diff: %v", err)
			}

			lopt := highlight.LangFromFilename(afile)
			if lang, ok := dir.Attrs["lang"]; ok {
				lopt = highlight.Lang(lang)
			}
			diff, err := highlight.Diff(string(a), string(b), lopt)
			if err != nil {
				return nil, fmt.Errorf("include-diff: %v", err)
			}

			display := cmp.Or(dir.Attrs["display"], bfile)
			t := s.templates.Lookup("fragments/include_diff")
			err = t.Execute(&buf, struct {
				File     string
				FilePath string
				Diff     []highlight.Edit
			}{
				File:     display,
				FilePath: filepath.Join(doc.Path(), bfile),
				Diff:     diff,
			})
			if err != nil {
				return nil, fmt.Errorf("rendering include-diff: %v", err)
			}

		default:
			return nil, fmt.Errorf("unknown directive: %s", dir.Name)
		}
		pos = dir.End
	}
	buf.Write(data[pos:])
	return buf.Bytes(), nil
}

type templateRenderer struct {
	template *template.Template
}

func (r *templateRenderer) render(s *Site, doc *Doc, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	err := r.template.Execute(&buf, struct {
		Meta    *Metadata
		Site    *Site
		Content template.HTML
	}{
		Meta:    doc.Meta(),
		Site:    s,
		Content: template.HTML(data),
	})
	if err != nil {
		return nil, fmt.Errorf("rendering template: %v", err)
	}
	return buf.Bytes(), nil
}

type feedRenderer struct{}

func (r *feedRenderer) render(s *Site, doc *Doc, data []byte) ([]byte, error) {
	articles := s.Articles()
	updated := articles[0].Meta().Updated

	feed := atom.Feed{
		Title:   doc.Meta().Title,
		ID:      "tag:znkr.io,2024:articles",
		Updated: atom.Time(updated),
		Link: []atom.Link{{
			Rel:  "self",
			Href: "https://flo.znkr.io/feed.atom",
		}},
	}

	for _, doc := range articles {
		html, err := s.RenderContent(doc)
		if err != nil {
			return nil, err
		}

		e := &atom.Entry{
			Title: doc.Meta().Title,
			ID:    feed.ID + doc.Path(),
			Link: []atom.Link{{
				Rel:  "alternate",
				Href: "https://flo.znkr.io" + doc.Path(),
			}},
			Published: atom.Time(doc.Meta().Published),
			Updated:   atom.Time(doc.Meta().Updated),
			Summary: &atom.Text{
				Type: "html",
				Body: doc.Meta().Abstract,
			},
			Content: &atom.Text{
				Type: "html",
				Body: string(html),
			},
			Author: &atom.Person{
				Name: "Florian Zenker",
			},
		}
		feed.Entry = append(feed.Entry, e)
	}

	b, err := xml.Marshal(feed)
	if err != nil {
		return nil, fmt.Errorf("encoding feed: %v", err)
	}
	return b, nil
}

package directives

import (
	"bytes"
	"cmp"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"flo.znkr.io/generator/highlight"
	"flo.znkr.io/generator/site"
)

type Renderer struct {
	snippet, diff *template.Template
}

func NewRenderer(templates *template.Template) *Renderer {
	return &Renderer{
		snippet: templates.Lookup("fragments/include_snippet"),
		diff:    templates.Lookup("fragments/include_diff"),
	}
}

func (r *Renderer) Render(doc *site.Doc, data []byte) ([]byte, error) {
	dirs, err := Parse(data)
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
			b, err := os.ReadFile(filepath.Join(filepath.Dir(doc.Source), file))
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
			err = r.snippet.Execute(&buf, struct {
				File     string
				FilePath string
				Lines    []highlight.Line
			}{
				File:     display,
				FilePath: filepath.Join(doc.Path, file),
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
			var a []byte
			if afile != "/dev/null" {
				var err error
				a, err = os.ReadFile(filepath.Join(filepath.Dir(doc.Source), afile))
				if err != nil {
					return nil, fmt.Errorf("include-snippet: %v", err)
				}
			}

			bfile := dir.Attrs["b"]
			if afile == "" {
				return nil, fmt.Errorf("include-diff: missing or empty file attribute")
			}
			b, err := os.ReadFile(filepath.Join(filepath.Dir(doc.Source), bfile))
			if err != nil {
				return nil, fmt.Errorf("include-diff: %v", err)
			}

			lopt := highlight.LangFromFilename(afile)
			if afile == "/dev/null" {
				lopt = highlight.LangFromFilename(bfile)
			}
			if lang, ok := dir.Attrs["lang"]; ok {
				lopt = highlight.Lang(lang)
			}
			diff, err := highlight.Diff(string(a), string(b), lopt)
			if err != nil {
				return nil, fmt.Errorf("include-diff: %v", err)
			}

			display := cmp.Or(dir.Attrs["display"], bfile)
			err = r.diff.Execute(&buf, struct {
				File     string
				FilePath string
				Diff     []highlight.Edit
			}{
				File:     display,
				FilePath: filepath.Join(filepath.Dir(doc.Source), bfile),
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

package directives

import (
	"bytes"
	"cmp"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

			if sel, ok := dir.Attrs["lines"]; ok {
				from, to, _ := strings.Cut(sel, "..")
				start, end := 0, len(lines)
				if from != "" {
					i, err := strconv.Atoi(from)
					if err != nil {
						return nil, fmt.Errorf("include-snippet: invalid lines attribute: %q", sel)
					}
					start = max(start, i-1)
				}
				if to != "" {
					i, err := strconv.Atoi(to)
					if err != nil {
						return nil, fmt.Errorf("include-snippet: invalid lines attribute: %q", sel)
					}
					end = min(i, end)
				}
				lines = lines[start:end]
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
			var display, path string
			var diff []highlight.Edit
			switch {
			case dir.Attrs["diff"] != "" && !dir.HasAttr("a") && !dir.HasAttr("b"):
				var lopt highlight.Option
				if lang, ok := dir.Attrs["lang"]; ok {
					lopt = highlight.Lang(lang)
				}
				raw, err := os.ReadFile(filepath.Join(filepath.Dir(doc.Source), dir.Attrs["diff"]))
				if err != nil {
					return nil, fmt.Errorf("include-snippet: %v", err)
				}
				diff, err = highlight.ParseDiff(string(raw), lopt)
				if err != nil {
					return nil, fmt.Errorf("include-diff: %v", err)
				}
				display = cmp.Or(dir.Attrs["display"], dir.Attrs["diff"])
				path = filepath.Join(doc.Path, dir.Attrs["diff"])

			case dir.Attrs["a"] != "" && dir.Attrs["b"] != "" && !dir.HasAttr("diff"):
				var a, b []byte
				var lopt highlight.Option
				if lang, ok := dir.Attrs["lang"]; ok {
					lopt = highlight.Lang(lang)
				}
				for name, dst := range map[string]*[]byte{dir.Attrs["a"]: &a, dir.Attrs["b"]: &b} {
					if name == "/dev/null" {
						continue
					}
					var err error
					*dst, err = os.ReadFile(filepath.Join(filepath.Dir(doc.Source), name))
					if err != nil {
						return nil, fmt.Errorf("include-snippet: %v", err)
					}
					if lopt == nil {
						lopt = highlight.LangFromFilename(name)
					}
					diff, err = highlight.Diff(string(a), string(b), lopt)
					if err != nil {
						return nil, fmt.Errorf("include-diff: %v", err)
					}
				}
				display = cmp.Or(dir.Attrs["display"], dir.Attrs["b"])
				path = filepath.Join(doc.Path, dir.Attrs["b"])
			default:
				return nil, fmt.Errorf("include-diff: either diff or a and b must be specified")
			}

			err = r.diff.Execute(&buf, struct {
				File     string
				FilePath string
				Diff     []highlight.Edit
			}{
				File:     display,
				FilePath: path,
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

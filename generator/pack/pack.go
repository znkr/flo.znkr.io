package pack

import (
	"archive/tar"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/svg"
	"github.com/tdewolff/minify/v2/xml"

	"flo.znkr.io/generator/site"
)

func Pack(filename string, s *site.Site) error {
	minifier := minify.New()
	minifier.AddFunc("text/css", css.Minify)
	minifier.AddFunc("text/html", html.Minify)
	minifier.AddFunc("image/svg+xml", svg.Minify)
	minifier.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)
	minifier.AddFuncRegexp(regexp.MustCompile("[/+]xml$"), xml.Minify)

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("opening file: %v", err)
	}
	defer file.Close()

	tw := tar.NewWriter(file)
	defer tw.Close()

	dirs := make(map[string]bool)

	for _, d := range s.AllDocs() {
		b, err := s.RenderPage(d)
		if err != nil {
			return err
		}

		mime, _, err := mime.ParseMediaType(d.MimeType)
		if err != nil {
			return fmt.Errorf("invalid mime type: %v", err)
		}

		switch mime {
		case "text/html", "text/css", "image/svg+xml", "application/atom+xml", "text/javascript":
			b, err = minifier.Bytes(d.MimeType, b)
			if err != nil {
				return fmt.Errorf("minification of failed for %s: %v", d.Path, err)
			}
		}

		path := d.Path
		if path == "/" {
			path = "index.html"
		} else if mime == "text/html" && filepath.Ext(path) == "" {
			path += "/index.html"
		}
		path = strings.TrimPrefix(path, "/")

		if dir := filepath.Dir(path); !dirs[dir] {
			name := "./" + dir + "/"
			if dir == "." {
				name = "./"
			}
			hdr := &tar.Header{
				Name: name,
				Mode: int64(0755),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("writing header: %v", err)
			}
			dirs[dir] = true
		}

		hdr := &tar.Header{
			Name: "./" + path,
			Mode: int64(0644),
			Size: int64(len(b)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("writing header: %v", err)
		}
		if _, err := tw.Write(b); err != nil {
			return fmt.Errorf("writing body: %v", err)
		}
	}

	return nil
}

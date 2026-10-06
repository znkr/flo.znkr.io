package renderers

import "fmt"

// Asset is a file a page loads, with the URL it is loaded from.
type Asset struct {
	// Path is where the file is served, for example "/_assets/style.css".
	Path string
	// URL is Path with a query that carries a version of the file's content.
	URL string
}

// Assets are the files a page can load. A browser that holds an older copy of
// a file in its cache fetches it again once its content, and so its URL,
// changes.
type Assets []Asset

// URL returns the URL that loads the asset served at p, and an error if there
// is no asset at p.
func (a Assets) URL(p string) (string, error) {
	for _, asset := range a {
		if asset.Path == p {
			return asset.URL, nil
		}
	}
	return "", fmt.Errorf("no asset at %s", p)
}

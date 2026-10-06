package renderers_test

import (
	"testing"

	"flo.znkr.io/generator/renderers"
)

// TestAssetsURL checks that a known path finds its URL and an unknown one is
// an error, so that a template naming a missing asset fails to render.
func TestAssetsURL(t *testing.T) {
	a := renderers.Assets{{Path: "/_assets/style.css", URL: "/_assets/style.css?v=1"}}
	if got, err := a.URL("/_assets/style.css"); err != nil || got != "/_assets/style.css?v=1" {
		t.Errorf("URL(%q) = %q, %v, want %q, nil", "/_assets/style.css", got, err, "/_assets/style.css?v=1")
	}
	if got, err := a.URL("/_assets/missing.css"); err == nil {
		t.Errorf("URL(%q) = %q, nil, want an error", "/_assets/missing.css", got)
	}
}

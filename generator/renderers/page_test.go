package renderers_test

import (
	"html/template"
	"strings"
	"testing"

	"flo.znkr.io/generator/markst"
	"flo.znkr.io/generator/renderers"
	"flo.znkr.io/generator/site"
)

// TestRenderPageRejectsANonPageType checks the guard on metadata that does not
// name a page template. Lookup alone does not catch it: the empty name finds
// the root template, and a fragment name finds a template that is no page.
func TestRenderPageRejectsANonPageType(t *testing.T) {
	templates := template.New("")
	for _, name := range []string{"article", "fragments/include_snippet", "fragments/include_diff"} {
		template.Must(templates.New(name).Parse(""))
	}

	for _, typ := range []string{"", "fragments/include_snippet"} {
		_, err := renderers.RenderPage(templates, site.Metadata{Type: typ}, renderers.Assets{}, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "unknown doc type") {
			t.Errorf("RenderPage() with type %q = %v, want an unknown doc type error", typ, err)
		}
	}
}

// TestRenderBodyNeedsTheFragmentTemplates checks that a missing fragment
// template is reported before anything is rendered, whether or not the
// document holds an include for it.
func TestRenderBodyNeedsTheFragmentTemplates(t *testing.T) {
	src := "#metadata((title: \"T\", type: \"article\")) <doc-meta>\n\nHello.\n"
	doc, err := markst.Load(t.Context(), "test.mst", []byte(src), nil, nil)
	if err != nil {
		t.Fatalf("compiling document: %v", err)
	}

	templates := template.New("")
	template.Must(templates.New("fragments/include_snippet").Parse(""))
	_, err = renderers.RenderBody(templates, doc, nil, "/test", "", markst.Popovers)
	if err == nil || !strings.Contains(err.Error(), "fragments/include_diff") {
		t.Errorf("RenderBody() without the diff template = %v, want an error naming it", err)
	}
}

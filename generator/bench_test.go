package main

import (
	"testing"

	"flo.znkr.io/generator/build"
	"flo.znkr.io/generator/source"
)

// BenchmarkWarmForceAll measures the Get path with everything already cached:
// what a served request costs now that reaching a value also reaches what it
// was made of.
func BenchmarkWarmForceAll(b *testing.B) {
	tree, err := source.Scan("..")
	if err != nil {
		b.Fatal(err)
	}
	s, _, err := plan(tree)
	if err != nil {
		b.Fatal(err)
	}
	c := build.NewCache(cacheSize)
	for _, d := range s.Docs() {
		if _, err := d.Page.Get(b.Context(), c); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		for _, d := range s.Docs() {
			if _, err := d.Page.Get(b.Context(), c); err != nil {
				b.Fatal(err)
			}
		}
	}
}

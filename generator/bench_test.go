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

// BenchmarkColdLoad measures what serve does on start and on every reload:
// scan, plan, and the metadata of every document, which compiles them all.
func BenchmarkColdLoad(b *testing.B) {
	for b.Loop() {
		c := build.NewCache(cacheSize)
		if _, err := reload(b.Context(), c, ".."); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkColdForceAll measures a pack: every page of the site from nothing.
func BenchmarkColdForceAll(b *testing.B) {
	tree, err := source.Scan("..")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
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
	}
}

// BenchmarkWarmReload measures a reload with everything already cached: what
// serve does after a file event when the edit changed nothing it compiles.
func BenchmarkWarmReload(b *testing.B) {
	c := build.NewCache(cacheSize)
	if _, err := reload(b.Context(), c, ".."); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := reload(b.Context(), c, ".."); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScan measures reading the tree, the floor under every reload.
func BenchmarkScan(b *testing.B) {
	for b.Loop() {
		if _, err := source.Scan(".."); err != nil {
			b.Fatal(err)
		}
	}
}

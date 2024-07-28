// Copyright 2024 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package diff provides functionality for diffing two slices of an arbitrary type with an arbitrary
// equality operator.
package diff

// Implementation note: The diff is an implementation of of Myers' diff algorithm. These algorithms
// seem like magic when read in code, at least I wasn't able to find an understandable code
// representation and examples I looked at were the same. They are not magic though, but they do
// require a bit of reading to understand them. The following links for a good explanation for this
// algorithm and working on this code will likely require re-reading how the algorithm works:
//
// https://blog.jcoglan.com/2017/02/12/the-myers-diff-algorithm-part-1/
// https://blog.jcoglan.com/2017/02/15/the-myers-diff-algorithm-part-2/
// https://blog.jcoglan.com/2017/02/17/the-myers-diff-algorithm-part-3/

import (
	"fmt"
	"slices"
)

const debug bool = false

// Op describes an edit operation.
//
//go:generate go run golang.org/x/tools/cmd/stringer -type=Op
type Op int

const (
	Match  Op = iota // Two slice elements match
	Delete           // A deletion from an element on the left slice
	Insert           // An insertion of an element from the right side
)

// Edit describes a singe edit of a diff.
//
//   - For Match, X and Y are set to their respective elements
//   - For Delete, X is set to the element of the left slice that's missing in the right one and Y is
//     set to the zero value
//   - For Insert, Y is set to he element of the right slice that's missing in the left one and X is
//     set to the zero value
type Edit[T any] struct {
	Op   Op
	X, Y T
}

func (ed *Edit[T]) IsMatch() bool  { return ed.Op == Match }
func (ed *Edit[T]) IsDelete() bool { return ed.Op == Delete }
func (ed *Edit[T]) IsInsert() bool { return ed.Op == Insert }

// Diff performs an element wise diff of x and y using eq for equality comparison.
func Diff[T any](x, y []T, eq func(x, y T) bool) []Edit[T] {
	var edits, suffix []Edit[T]
	var zero T

	// Try to reduce the amount of work necessary by skipping a common prefix
	if lcp := longestCommonPrefix(x, y, eq); lcp > 0 {
		edits = slices.Grow(edits, lcp)
		for i := range lcp {
			edits = append(edits, Edit[T]{Match, x[i], y[i]})
		}
		x = x[lcp:]
		y = y[lcp:]
	}

	// Try to reduce the amount of work necessary by skipping a common suffix
	if lcs := longestCommonSuffix(x, y, eq); lcs > 0 {
		suffix = make([]Edit[T], 0, lcs)
		for i := range lcs {
			suffix = append(suffix, Edit[T]{Match, x[len(x)-lcs+i], y[len(y)-lcs+i]})
		}
		x = x[:len(x)-lcs]
		y = y[:len(y)-lcs]
	}

	switch {
	case len(x) == 0 && len(y) == 0:
		// nothing left to do
	case len(x) == 0:
		edits = slices.Grow(edits, len(y))
		for i := range y {
			edits = append(edits, Edit[T]{Insert, zero, y[i]})
		}
	case len(y) == 0:
		edits = slices.Grow(edits, len(x))
		for i := range x {
			edits = append(edits, Edit[T]{Delete, x[i], zero})
		}
	default:
		edits = findShortestEditSequence(edits, x, y, eq)
	}

	return append(edits, suffix...)
}

func longestCommonPrefix[T any](x, y []T, eq func(x, y T) bool) int {
	n := min(len(x), len(y))
	for i := range n {
		if !eq(x[i], y[i]) {
			return i
		}
	}
	return n
}

func longestCommonSuffix[T any](x, y []T, eq func(x, y T) bool) int {
	n := min(len(x), len(y))
	if n == 0 {
		return 0
	}
	for i := range n - 1 {
		if !eq(x[len(x)-i-1], y[len(y)-i-1]) {
			return i
		}
	}
	return n - 1
}

func findShortestEditSequence[T any](edits []Edit[T], x, y []T, eq func(x, y T) bool) []Edit[T] {
	var zero T
	if len(x)+len(y) < 0 {
		panic("inputs too large")
	}

	v := computeMyersGraph(x, y, eq)

	// Appends diffs in reverse order by backtracking along the edges in the MyersGraph to diffs and
	// reverses them in place.
	preexistingEdits := len(edits) // Used to reverse the appended edits.
	s := len(x)
	t := len(y)

	for d := v.maxDepth; ; d-- {
		k := s - t
		if debug {
			if max(k, -k)%2 != d%2 {
				panic("invariant violation")
			}
		}

		var prevK int
		switch {
		case d == 0:
			prevK = 0
		case k == -d || (k != d && v.get(d-1, k-1) < v.get(d-1, k+1)):
			prevK = k + 1
		default:
			prevK = k - 1
		}

		prevS := 0
		if d > 0 {
			prevS = v.get(d-1, prevK)
		}
		prevT := prevS - prevK

		for prevS < s && prevT < t {
			edits = append(edits, Edit[T]{Match, x[s-1], y[t-1]})
			s--
			t--
		}

		if d == 0 {
			break
		}

		if debug {
			if prevS == s && prevT == t {
				panic("invariant violation")
			}
		}
		if prevS == s {
			edits = append(edits, Edit[T]{Insert, zero, y[prevT]})
		} else {
			if debug {
				if prevT != t {
					panic("invariant violation")
				}
			}
			edits = append(edits, Edit[T]{Delete, x[prevS], zero})
		}

		s = prevS
		t = prevT
	}

	slices.Reverse(edits[preexistingEdits:])
	return edits
}

// myersGraph stores the graph that is generated during findShortestEditSequence. The graph is
// stored in a flat slice and by storing the full graph, it's not necessary to record a trace at
// every depth iteration.
type myersGraph struct {
	v        []int
	maxDepth int
}

func (g *myersGraph) upgradeMaxDepth(maxDepth int) {
	if maxDepth < g.maxDepth {
		return
	}
	n := (maxDepth + 2) * (maxDepth + 1) / 2
	g.v = slices.Grow(g.v, n)
	g.v = g.v[:n]
	g.maxDepth = maxDepth
}

func (g *myersGraph) get(d, k int) int    { return g.v[g.index(d, k)] }
func (g *myersGraph) set(d, k int, v int) { g.v[g.index(d, k)] = v }

func (g *myersGraph) index(d, k int) int {
	if debug {
		if d < 0 || d > g.maxDepth {
			panic(fmt.Sprintf("d must be in [0, %v] but is %v", g.maxDepth, d))
		}
		if k < -d || k > d {
			panic(fmt.Sprintf("k must be in [%v, %v] but is %v", -d, d, k))
		}
		if k&1 != d&1 {
			panic(fmt.Sprintf("d and k must have same parity: %v vs %v", d, k))
		}
	}
	// The number of k's is always equal to d + 1. Therefore, we know how many k's were before
	// this d: (d + 1) * d / 2. We can then pack the k's into the next d slots.
	i := (d + 1) * d / 2
	j := k
	if k < 0 {
		j = -k - 1
	}
	return i + j
}

func computeMyersGraph[T any](x, y []T, eq func(x, y T) bool) myersGraph {
	v := myersGraph{maxDepth: -1}
	dMax := len(x) + len(y)
	for d := range dMax + 1 {
		v.upgradeMaxDepth(d)
		for k := -d; k <= d; k += 2 {
			var s int
			if d == 0 {
				s = 0
			} else if k == -d || (k != d && v.get(d-1, k-1) < v.get(d-1, k+1)) {
				s = v.get(d-1, k+1)
			} else {
				s = v.get(d-1, k-1) + 1
			}
			t := s - k

			if s < len(x) && t < len(y) {
				lcp := longestCommonPrefix(x[s:], y[t:], eq)
				s += lcp
				t += lcp
			}

			v.set(d, k, s)

			if s >= len(x) && t >= len(y) {
				return v
			}
		}
	}
	panic("never reached")
}

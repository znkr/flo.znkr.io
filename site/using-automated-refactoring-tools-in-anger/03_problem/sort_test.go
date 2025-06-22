package problem

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestSort(t *testing.T) {
	tests := []struct {
		in   []int
		want []int
	}{
		{[]int{}, []int{}},
		{[]int{1}, []int{1}},
		{[]int{1, 2}, []int{1, 2}},
		{[]int{2, 1}, []int{1, 2}},
		{[]int{1, 2, 3, 4}, []int{1, 2, 3, 4}},
		{[]int{3, 4, 1, 2}, []int{1, 2, 3, 4}},
	}

	for _, tt := range tests {
		got := slices.Clone(tt.in)
		Sort(got, func(a, b int) bool { return a < b })
		if !slices.Equal(got, tt.want) {
			t.Errorf("Sort(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func BenchmarkSort(b *testing.B) {
	v := make([]int, 100)
	for b.Loop() {
		b.StopTimer()
		for i := range v {
			v[i] = rand.Int()
		}
		b.StartTimer()
		Sort(v, func(a, b int) bool { return a < b })
	}
}

func BenchmarkSortInt(b *testing.B) {
	v := make([]int, 100)
	for b.Loop() {
		b.StopTimer()
		for i := range v {
			v[i] = rand.Int()
		}
		b.StartTimer()
		SortInt(v)
	}
}

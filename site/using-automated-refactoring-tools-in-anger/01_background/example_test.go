package background

import "fmt"

func Example() {
	v := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	apply(v, func(n int) int { return n % 2 })
	fmt.Println(v)
	// Output:
	// [1 0 1 0 1 0 1 0 1 0]
}

func apply(v []int, fn func(n int) int) {
	for i, n := range v {
		v[i] = fn(n)
	}
}

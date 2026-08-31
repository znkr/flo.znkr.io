package main

func sum(vs []int) int {
	var total int
	for _, v := range vs {
		total += v
	}
	return total
}

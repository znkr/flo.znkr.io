package main

func sum(vs []int) int {
	total := 0
	for i := 0; i < len(vs); i++ {
		total += vs[i]
	}
	return total
}

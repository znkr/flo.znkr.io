package problem

func Sort[T any](v []T, less func(a, b T) bool) {
	if len(v) <= 1 {
		return
	}
	pivot := v[len(v)-1]
	i := 0
	for j := range v {
		if less(v[j], pivot) {
			v[i], v[j] = v[j], v[i]
			i++
		}
	}
	v[i], v[len(v)-1] = v[len(v)-1], v[i]
	Sort(v[:i], less)
	Sort(v[i:], less)
}

package solution

func SortInt(v []int) {
	if len(v) <= 1 {
		return
	}
	pivot := v[len(v)-1]
	i := 0
	for j := range v {
		if v[j] < pivot {
			v[i], v[j] = v[j], v[i]
			i++
		}
	}
	v[i], v[len(v)-1] = v[len(v)-1], v[i]
	SortInt(v[:i])
	SortInt(v[i:])
}

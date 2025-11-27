package slices

func Find[T any](slice []T, f func(T) bool) (T, bool) {
	for _, item := range slice {
		if f(item) {
			return item, true
		}
	}

	return *new(T), false
}

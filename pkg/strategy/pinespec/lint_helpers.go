package pinespec

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

func jftradeOptionalTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		var zero T
		return zero
	}
	return typed
}

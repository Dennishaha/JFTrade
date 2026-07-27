package testkit

func jftradeCheckedTypeAssertion[T any](value any) T {
	return value.(T)
}

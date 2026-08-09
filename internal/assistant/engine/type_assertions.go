package adk

import jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}

func jftradeOptionalTypeAssertion[T any](value any) T {
	return jfadkmodel.OptionalTypeAssertion[T](value)
}

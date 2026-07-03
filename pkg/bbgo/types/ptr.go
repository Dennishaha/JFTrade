package types

//go:fix inline
func BoolPtr(v bool) *bool {
	return new(v)
}

//go:fix inline
func IntPtr(v int) *int {
	return new(v)
}

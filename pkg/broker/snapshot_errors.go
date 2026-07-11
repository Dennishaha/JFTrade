package broker

import "errors"

// SymbolScopedSnapshotError marks a batch snapshot failure caused by one or
// more symbols in the request. Callers may isolate the failing symbols by
// retrying smaller batches; transport and service errors must remain unmarked.
type SymbolScopedSnapshotError struct {
	err error
}

func (e *SymbolScopedSnapshotError) Error() string { return e.err.Error() }
func (e *SymbolScopedSnapshotError) Unwrap() error { return e.err }

func NewSymbolScopedSnapshotError(err error) error {
	if err == nil {
		return nil
	}
	return &SymbolScopedSnapshotError{err: err}
}

func IsSymbolScopedSnapshotError(err error) bool {
	var target *SymbolScopedSnapshotError
	return errors.As(err, &target)
}

package application

import (
	"errors"
	"sync"
)

// Lifecycle owns application-level resource registration state. It keeps
// compatibility setup for narrow test assemblies out of the application
// dependency aggregate while preserving idempotent, reverse-order shutdown.
type Lifecycle struct {
	mu        sync.Mutex
	resources *Resources
	setupErr  error

	setupOnce       sync.Once
	persistentOwned bool
	runtimeOwned    bool
}

// NewLifecycle adopts an existing resource sequence and its setup result.
// The ownership flags indicate that the corresponding resources were already
// registered while the production application graph was assembled.
func NewLifecycle(
	resources *Resources,
	setupErr error,
	persistentOwned bool,
	runtimeOwned bool,
) Lifecycle {
	if resources == nil {
		resources = &Resources{}
	}
	return Lifecycle{
		resources:       resources,
		setupErr:        setupErr,
		persistentOwned: persistentOwned,
		runtimeOwned:    runtimeOwned,
	}
}

// EnsureOwnedResources registers resources for narrow or zero-value
// application assemblies. Production assemblies mark these groups as already
// owned when constructing the lifecycle.
func (l *Lifecycle) EnsureOwnedResources(
	registerPersistent func(),
	registerRuntime func(),
) {
	if l == nil {
		return
	}
	l.setupOnce.Do(func() {
		if !l.persistentOwned && registerPersistent != nil {
			registerPersistent()
		}
		if !l.runtimeOwned && registerRuntime != nil {
			registerRuntime()
		}
	})
}

// Register appends a resource to the application shutdown sequence.
func (l *Lifecycle) Register(name string, closeFn func() error) {
	if l == nil || closeFn == nil {
		return
	}
	if err := l.Resources().Register(name, closeFn); err != nil {
		l.AddSetupError(err)
	}
}

// AddSetupError records an application assembly or late-registration error.
func (l *Lifecycle) AddSetupError(err error) {
	if l == nil || err == nil {
		return
	}
	l.mu.Lock()
	l.setupErr = errors.Join(l.setupErr, err)
	l.mu.Unlock()
}

// SetupError returns the aggregate application assembly error.
func (l *Lifecycle) SetupError() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.setupErr
}

// Resources returns the adopted resource sequence, lazily creating it for the
// zero value.
func (l *Lifecycle) Resources() *Resources {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.resources == nil {
		l.resources = &Resources{}
	}
	return l.resources
}

// Close combines setup failures with idempotent reverse-order shutdown.
func (l *Lifecycle) Close() error {
	if l == nil {
		return nil
	}
	resourceErr := l.Resources().Close()
	return errors.Join(l.SetupError(), resourceErr)
}

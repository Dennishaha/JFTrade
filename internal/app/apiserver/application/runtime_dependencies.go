package application

import (
	"context"

	"github.com/jftrade/jftrade-main/internal/system"
)

func RuntimeDependencies(service *system.Service) func(context.Context) any {
	return func(ctx context.Context) any {
		if service == nil {
			return map[string]any{"status": "unavailable"}
		}
		return service.RuntimeDependencies(ctx)
	}
}

package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// desktopNotificationLifecycle keeps the native notification backend on the
// Wails service lifecycle without exporting its general-purpose methods to the
// frontend binding surface.
type desktopNotificationLifecycle struct {
	service *notifications.NotificationService
}

func newDesktopNotificationLifecycle(service *notifications.NotificationService) *desktopNotificationLifecycle {
	return &desktopNotificationLifecycle{service: service}
}

func (s *desktopNotificationLifecycle) ServiceName() string {
	return "jftrade.desktop.notifications"
}

func (s *desktopNotificationLifecycle) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return s.service.ServiceStartup(ctx, options)
}

func (s *desktopNotificationLifecycle) ServiceShutdown() error {
	return s.service.ServiceShutdown()
}

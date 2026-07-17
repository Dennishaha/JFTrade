package live

import "testing"

func TestNotificationDeliveryKeepsHostNotificationOutcomeExplicit(t *testing.T) {
	delivered := NotificationDelivered("desktop notification sent")
	if !delivered.Delivered || delivered.Status != NotificationDeliveryDelivered || delivered.Message != "desktop notification sent" {
		t.Fatalf("delivered notification = %#v", delivered)
	}

	filtered := NotificationNotDelivered(NotificationDeliveryFiltered, "user disabled price alerts")
	if filtered.Delivered || filtered.Status != NotificationDeliveryFiltered || filtered.Message != "user disabled price alerts" {
		t.Fatalf("filtered notification = %#v", filtered)
	}
}

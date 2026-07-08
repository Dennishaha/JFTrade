package live

const (
	NotificationDeliveryDelivered    = "delivered"
	NotificationDeliveryFiltered     = "filtered"
	NotificationDeliveryUnauthorized = "unauthorized"
	NotificationDeliveryUnsupported  = "unsupported"
	NotificationDeliveryUnavailable  = "unavailable"
	NotificationDeliveryFailed       = "failed"
)

// NotificationDelivery reports whether a retained live event was also delivered
// to the host operating system notification center.
type NotificationDelivery struct {
	Delivered bool   `json:"delivered"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

func NotificationDelivered(message string) NotificationDelivery {
	return NotificationDelivery{Delivered: true, Status: NotificationDeliveryDelivered, Message: message}
}

func NotificationNotDelivered(status string, message string) NotificationDelivery {
	return NotificationDelivery{Status: status, Message: message}
}

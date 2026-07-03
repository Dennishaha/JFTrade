package bbgo

import "github.com/jftrade/jftrade-main/pkg/bbgo/types"

var Notification = &Notifiability{}

func Notify(obj any, args ...any) {
	Notification.Notify(obj, args...)
}

type Notifier interface {
	Notify(obj any, args ...any)
	Upload(file *types.UploadFile)
}

type Notifiability struct {
	notifiers []Notifier
}

func (n *Notifiability) AddNotifier(notifier Notifier) {
	if notifier == nil {
		return
	}
	n.notifiers = append(n.notifiers, notifier)
}

func (n *Notifiability) Notify(obj any, args ...any) {
	for _, notifier := range n.notifiers {
		notifier.Notify(obj, args...)
	}
}

func (n *Notifiability) Upload(file *types.UploadFile) {
	for _, notifier := range n.notifiers {
		notifier.Upload(file)
	}
}

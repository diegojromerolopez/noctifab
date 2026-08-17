package notifier

import (
	"context"
	"errors"
)

// MultiNotifier coordinates dispatching notification alerts across multiple notifiers.
type MultiNotifier struct {
	notifiers []DesktopNotifier
}

// NewMultiNotifier creates a new MultiNotifier composite.
func NewMultiNotifier(notifiers ...DesktopNotifier) *MultiNotifier {
	valid := make([]DesktopNotifier, 0, len(notifiers))
	for _, n := range notifiers {
		if n != nil {
			valid = append(valid, n)
		}
	}
	return &MultiNotifier{
		notifiers: valid,
	}
}

// Notify triggers alerts on all registered notifiers sequentially.
func (m *MultiNotifier) Notify(ctx context.Context, kind NotificationKind, title, message string) error {
	var errs []error
	for _, n := range m.notifiers {
		if err := n.Notify(ctx, kind, title, message); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Notifiers returns the list of registered notifiers.
func (m *MultiNotifier) Notifiers() []DesktopNotifier {
	return m.notifiers
}

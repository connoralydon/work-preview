package preview

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("preview not found")
	ErrPrefixConflict = errors.New("preview prefix is already in use")
)

type Store interface {
	Create(context.Context, Preview) error
	Active(context.Context) ([]Preview, error)
	GetActive(context.Context, string) (Preview, error)
	Touch(context.Context, string, time.Time, time.Time) error
	SetStatus(context.Context, string, string, time.Time) error
	RecordEvent(context.Context, string, string, time.Time, string) error
}

const (
	EventCreated      = "created"
	EventAccessed     = "accessed"
	EventDeleted      = "deleted"
	EventExpired      = "expired"
	EventReloadFailed = "reload_failed"
)

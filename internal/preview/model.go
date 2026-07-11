package preview

import "time"

type Preview struct {
	ID           string
	Prefix       string
	Port         uint16
	Status       string
	CreatedAt    time.Time
	LastAccessAt time.Time
	ExpiresAt    time.Time
	Persistent   bool
	BootID       string
}

const (
	StatusActive  = "active"
	StatusDeleted = "deleted"
	StatusExpired = "expired"
)

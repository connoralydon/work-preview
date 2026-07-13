package preview

import "time"

type Preview struct {
	ID           string
	Prefix       string
	Port         uint16
	Repository   string
	Branch       string
	Commit       string
	Status       string
	CreatedAt    time.Time
	LastAccessAt time.Time
	ExpiresAt    time.Time
}

const (
	StatusActive  = "active"
	StatusDeleted = "deleted"
	StatusExpired = "expired"
)

// Package store is the data layer. It reads and writes items and knows
// nothing about owner tokens, expiry rules or HTTP; those belong to the
// handler.
package store

import (
	"context"
	"errors"

	"github.com/jtravisp/privatepaste/internal/model"
)

// ErrNotFound means no item exists for the requested ID.
var ErrNotFound = errors.New("paste not found")

type Store interface {
	CreatePaste(ctx context.Context, paste *model.Paste) error
	GetPaste(ctx context.Context, id string) (*model.Paste, error)
	// ConsumePaste deletes the item and returns what was deleted, in one
	// atomic operation. If two readers race, exactly one gets the paste and
	// the other gets ErrNotFound. This is what makes burn-after-read hold.
	ConsumePaste(ctx context.Context, id string) (*model.Paste, error)
	DeletePaste(ctx context.Context, id string) error
	// Ping checks the table is reachable; used by the readiness probe.
	Ping(ctx context.Context) error
}

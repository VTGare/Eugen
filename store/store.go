package store

import (
	"context"
	"errors"
)

type Store interface {
	GuildStore
	MessageStore
	Init(ctx context.Context) error
	Close(ctx context.Context) error
}

// Common errors
var (
	ErrInternal        = errors.New("internal error")
	ErrGuildNotFound   = errors.New("guild not found")
	ErrMessageNotFound = errors.New("message not found")
)

package store

import "github.com/jtravisp/privatepaste/internal/model"

type Store interface {
	CreatePaste(paste *model.Paste) error
	GetPaste(id string) (*model.Paste, error)
	DeletePaste(id string) error
}

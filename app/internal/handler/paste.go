package handler

import (
	"net/http"

	"github.com/jtravisp/privatepaste/internal/store"
)

type PasteHandler struct {
    store store.Store
}

type CreatePasteRequest struct {
    Ciphertext   string `json:"ciphertext"`
    IV           string `json:"iv"`
    BurnAfterRead bool  `json:"burn_after_read"`
    Expiry       string `json:"expiry"`
}

type CreatePasteResponse struct {
    ID         string `json:"id"`
    OwnerToken string `json:"owner_token"`
}

func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
    // implement
}

func (h *PasteHandler) GetPaste(w http.ResponseWriter, r *http.Request) {
    // implement
}

func (h *PasteHandler) DeletePaste(w http.ResponseWriter, r *http.Request) {
    // implement
}

package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jtravisp/privatepaste/internal/model"
	"github.com/jtravisp/privatepaste/internal/store"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

type PasteHandler struct {
	store store.Store
}

func NewPasteHandler(s store.Store) *PasteHandler {
	return &PasteHandler{store: s}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type CreatePasteRequest struct {
	Ciphertext    string `json:"ciphertext"`
	IV            string `json:"iv"`
	BurnAfterRead bool   `json:"burn_after_read"`
	Expiry        string `json:"expiry"`
}

type CreatePasteResponse struct {
	ID         string `json:"id"`
	OwnerToken string `json:"owner_token"`
}

type GetPasteResponse struct {
	Ciphertext    string `json:"ciphertext"`
	IV            string `json:"iv"`
	BurnAfterRead bool   `json:"burn_after_read"`
}

func expiryTTL(expiry string) int64 {
	now := time.Now()
	switch expiry {
	case "burn", "never":
		return 0 // handled by logic, not TTL
	case "1h":
		return now.Add(1 * time.Hour).Unix()
	case "24h":
		return now.Add(24 * time.Hour).Unix()
	case "7d":
		return now.Add(7 * 24 * time.Hour).Unix()
	default:
		return now.Add(24 * time.Hour).Unix() // safe fallback
	}
}

func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	var req CreatePasteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Ciphertext == "" || req.IV == "" {
		writeError(w, http.StatusBadRequest, "ciphertext and iv are required")
		return
	}

	id, err := gonanoid.New()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate ID")
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	ownerToken := hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(ownerToken))
	ownerTokenHash := hex.EncodeToString(hash[:])

	paste := &model.Paste{
		ID:             id,
		Ciphertext:     req.Ciphertext,
		IV:             req.IV,
		BurnAfterRead:  req.BurnAfterRead,
		TTL:            expiryTTL(req.Expiry),
		CreatedAt:      time.Now().Unix(),
		OwnerTokenHash: ownerTokenHash,
	}

	if err := h.store.CreatePaste(paste); err != nil {
		log.Printf("CreatePaste error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save paste")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreatePasteResponse{
		ID:         id,
		OwnerToken: ownerToken,
	})
}

func (h *PasteHandler) GetPaste(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paste, err := h.store.GetPaste(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "paste not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to retrieve paste")
		}
		return
	}

	if paste.BurnAfterRead {
		h.store.DeletePaste(id) // best effort to delete after read
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetPasteResponse{
		Ciphertext:    paste.Ciphertext,
		IV:            paste.IV,
		BurnAfterRead: paste.BurnAfterRead,
	})
}

func (h *PasteHandler) DeletePaste(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if authHeader == "" {
		writeError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}

	paste, err := h.store.GetPaste(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "paste not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to retrieve paste")
		}
		return
	}

	hash := sha256.Sum256([]byte(authHeader))
	ownerTokenHash := hex.EncodeToString(hash[:])
	if ownerTokenHash != paste.OwnerTokenHash {
		writeError(w, http.StatusForbidden, "invalid owner token")
		return
	}

	if err := h.store.DeletePaste(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete paste")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

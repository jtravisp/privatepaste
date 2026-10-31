// Package handler holds the HTTP handlers for the paste API. Business rules
// live here: owner-token verification, expiry and burn-after-read. The store
// underneath only reads and writes.
package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jtravisp/privatepaste/internal/model"
	"github.com/jtravisp/privatepaste/internal/store"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// MaxBodyBytes caps a create request. Rejected by http.MaxBytesReader before
// the JSON decoder buffers it, so an oversized body never reaches DynamoDB
// (whose item limit is 400 KB).
const MaxBodyBytes = 512 * 1024

// burnTTL bounds how long an unread burn-after-read paste is kept. Without
// it, a burn paste nobody opens would live forever.
const burnTTL = 7 * 24 * time.Hour

// idPattern matches the default nanoid alphabet and length. Anything else is
// rejected before it becomes a DynamoDB key.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{21}$`)

type PasteHandler struct {
	store store.Store
	now   func() time.Time
}

func NewPasteHandler(s store.Store) *PasteHandler {
	return &PasteHandler{store: s, now: time.Now}
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Paste responses must never be cached by a browser or an edge: a cached
	// burn-after-read response would defeat the burn.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ttlFor turns the requested expiry into a DynamoDB TTL. Unknown values are
// an error rather than a silent default: the client said something we do not
// understand, and guessing a lifetime for someone's secret is the wrong
// failure mode.
func ttlFor(expiry string, burn bool, now time.Time) (int64, bool) {
	switch expiry {
	case "burn":
		return now.Add(burnTTL).Unix(), true
	case "1h":
		return now.Add(time.Hour).Unix(), true
	case "24h":
		return now.Add(24 * time.Hour).Unix(), true
	case "7d":
		return now.Add(7 * 24 * time.Hour).Unix(), true
	case "never":
		if burn {
			return now.Add(burnTTL).Unix(), true
		}
		return 0, true
	default:
		return 0, false
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	var req CreatePasteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "paste too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The server cannot read the ciphertext, but it can check its shape: a
	// 12-byte AES-GCM nonce and valid base64. This rejects junk without
	// learning anything about the content.
	iv, err := base64.StdEncoding.DecodeString(req.IV)
	if err != nil || len(iv) != 12 {
		writeError(w, http.StatusBadRequest, "iv must be a base64 12-byte nonce")
		return
	}
	if _, err := base64.StdEncoding.DecodeString(req.Ciphertext); err != nil || req.Ciphertext == "" {
		writeError(w, http.StatusBadRequest, "ciphertext must be non-empty base64")
		return
	}

	burn := req.BurnAfterRead || req.Expiry == "burn"
	now := h.now()
	ttl, ok := ttlFor(req.Expiry, burn, now)
	if !ok {
		writeError(w, http.StatusBadRequest, "expiry must be one of burn, 1h, 24h, 7d, never")
		return
	}

	id, err := gonanoid.New()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate id")
		return
	}

	// The owner token is returned once and never stored; only its hash is.
	// A database leak therefore does not let anyone delete pastes.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	ownerToken := hex.EncodeToString(tokenBytes)

	paste := &model.Paste{
		ID:             id,
		Ciphertext:     req.Ciphertext,
		IV:             req.IV,
		BurnAfterRead:  burn,
		TTL:            ttl,
		CreatedAt:      now.Unix(),
		OwnerTokenHash: hashToken(ownerToken),
	}
	if err := h.store.CreatePaste(r.Context(), paste); err != nil {
		slog.Error("create paste", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to save paste")
		return
	}

	writeJSON(w, http.StatusCreated, CreatePasteResponse{ID: id, OwnerToken: ownerToken})
}

// lookup fetches a live paste, treating malformed IDs and expired items the
// same as missing ones so callers cannot tell them apart.
func (h *PasteHandler) lookup(w http.ResponseWriter, r *http.Request) (*model.Paste, bool) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		writeError(w, http.StatusNotFound, "paste not found")
		return nil, false
	}
	paste, err := h.store.GetPaste(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && paste.Expired(h.now())) {
		writeError(w, http.StatusNotFound, "paste not found")
		return nil, false
	}
	if err != nil {
		slog.Error("get paste", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve paste")
		return nil, false
	}
	return paste, true
}

func (h *PasteHandler) GetPaste(w http.ResponseWriter, r *http.Request) {
	paste, ok := h.lookup(w, r)
	if !ok {
		return
	}

	if paste.BurnAfterRead {
		// Delete-and-return in one conditional write. If another request
		// consumed it between our read and this write, we lost the race and
		// the paste is gone -- answer 404 rather than serve it twice.
		consumed, err := h.store.ConsumePaste(r.Context(), paste.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "paste not found")
			return
		}
		if err != nil {
			slog.Error("consume paste", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve paste")
			return
		}
		paste = consumed
	}

	writeJSON(w, http.StatusOK, GetPasteResponse{
		Ciphertext:    paste.Ciphertext,
		IV:            paste.IV,
		BurnAfterRead: paste.BurnAfterRead,
	})
}

func (h *PasteHandler) DeletePaste(w http.ResponseWriter, r *http.Request) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || token == "" {
		writeError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}

	paste, ok := h.lookup(w, r)
	if !ok {
		return
	}

	// Constant-time comparison of the hashes, so response timing does not
	// reveal how many leading characters matched.
	if subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(paste.OwnerTokenHash)) != 1 {
		writeError(w, http.StatusForbidden, "invalid owner token")
		return
	}

	if err := h.store.DeletePaste(r.Context(), paste.ID); err != nil {
		slog.Error("delete paste", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to delete paste")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

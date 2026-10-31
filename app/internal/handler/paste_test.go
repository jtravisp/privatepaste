package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jtravisp/privatepaste/internal/model"
	"github.com/jtravisp/privatepaste/internal/store"
)

var (
	validIV         = base64.StdEncoding.EncodeToString(make([]byte, 12))
	validCiphertext = base64.StdEncoding.EncodeToString([]byte("opaque-bytes"))
)

func newMux(h *PasteHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /pastes", h.CreatePaste)
	mux.HandleFunc("GET /pastes/{id}", h.GetPaste)
	mux.HandleFunc("DELETE /pastes/{id}", h.DeletePaste)
	return mux
}

func create(t *testing.T, mux http.Handler, body map[string]any) (*httptest.ResponseRecorder, CreatePasteResponse) {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/pastes", bytes.NewReader(b)))
	var resp CreatePasteResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec, resp
}

func get(mux http.Handler, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pastes/"+id, nil))
	return rec
}

func TestCreateAndGet(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	rec, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "24h"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", rec.Code, rec.Body)
	}
	if len(created.OwnerToken) != 64 || created.ID == "" {
		t.Fatalf("unexpected create response %+v", created)
	}

	for i := 0; i < 2; i++ {
		rec = get(mux, created.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("get #%d: got %d", i, rec.Code)
		}
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestOwnerTokenIsStoredOnlyAsHash(t *testing.T) {
	mem := store.NewMemory()
	mux := newMux(NewPasteHandler(mem))
	_, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "1h"})
	p, _ := mem.GetPaste(t.Context(), created.ID)
	if p.OwnerTokenHash == created.OwnerToken || p.OwnerTokenHash != hashToken(created.OwnerToken) {
		t.Fatal("stored value must be the SHA-256 of the token, not the token")
	}
}

func TestCreateValidation(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	cases := map[string]map[string]any{
		"missing ciphertext": {"iv": validIV, "expiry": "1h"},
		"bad iv length":      {"ciphertext": validCiphertext, "iv": base64.StdEncoding.EncodeToString(make([]byte, 8)), "expiry": "1h"},
		"iv not base64":      {"ciphertext": validCiphertext, "iv": "!!!", "expiry": "1h"},
		"unknown expiry":     {"ciphertext": validCiphertext, "iv": validIV, "expiry": "forever"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec, _ := create(t, mux, body); rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
		})
	}
}

func TestCreateRejectsOversizedBody(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	big := strings.Repeat("A", MaxBodyBytes+1)
	rec, _ := create(t, mux, map[string]any{"ciphertext": big, "iv": validIV, "expiry": "1h"})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", rec.Code)
	}
}

func TestBurnAfterReadServesOnce(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	_, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "burn", "burn_after_read": true})
	if rec := get(mux, created.ID); rec.Code != http.StatusOK {
		t.Fatalf("first read: got %d", rec.Code)
	}
	if rec := get(mux, created.ID); rec.Code != http.StatusNotFound {
		t.Fatalf("second read: got %d, want 404", rec.Code)
	}
}

// Concurrent readers of a burn paste: exactly one may succeed.
func TestBurnAfterReadRace(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	_, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "burn"})

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if get(mux, created.ID).Code == http.StatusOK {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d readers got the paste, want exactly 1", ok)
	}
}

func TestBurnPasteHasSafetyTTL(t *testing.T) {
	mem := store.NewMemory()
	mux := newMux(NewPasteHandler(mem))
	_, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "burn"})
	p, _ := mem.GetPaste(t.Context(), created.ID)
	if p.TTL == 0 {
		t.Fatal("an unread burn paste must still expire")
	}
}

func TestNeverExpiryStoresNoTTL(t *testing.T) {
	mem := store.NewMemory()
	mux := newMux(NewPasteHandler(mem))
	_, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "never"})
	p, _ := mem.GetPaste(t.Context(), created.ID)
	if p.TTL != 0 {
		t.Fatalf("TTL = %d, want 0", p.TTL)
	}
}

func TestExpiredPasteIsNotServed(t *testing.T) {
	mem := store.NewMemory()
	h := NewPasteHandler(mem)
	id := "AAAAAAAAAAAAAAAAAAAAA"
	mem.Put(model.Paste{ID: id, Ciphertext: validCiphertext, IV: validIV, TTL: time.Now().Add(-time.Minute).Unix()})
	if rec := get(newMux(h), id); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404 for expired paste", rec.Code)
	}
}

func TestMalformedIDIsNotFound(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	if rec := get(mux, "not-a-valid-id"); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestDelete(t *testing.T) {
	mux := newMux(NewPasteHandler(store.NewMemory()))
	_, created := create(t, mux, map[string]any{"ciphertext": validCiphertext, "iv": validIV, "expiry": "7d"})

	del := func(auth string) int {
		req := httptest.NewRequest(http.MethodDelete, "/pastes/"+created.ID, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := del(""); code != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", code)
	}
	if code := del("Bearer wrong"); code != http.StatusForbidden {
		t.Errorf("wrong token: got %d, want 403", code)
	}
	if code := del("Bearer " + created.OwnerToken); code != http.StatusNoContent {
		t.Errorf("right token: got %d, want 204", code)
	}
	if rec := get(mux, created.ID); rec.Code != http.StatusNotFound {
		t.Errorf("after delete: got %d, want 404", rec.Code)
	}
}

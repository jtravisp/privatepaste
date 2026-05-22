package handler

import (
	"net/http"
	"github.com/jtravisp/privatepaste/internal/store"
)

type PasteHandler struct {
    store store.Store
}

func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
	http.MethodPost(url string, contentType string, body io.Reader)
}

func (h *PasteHandler) GetPaste(w http.ResponseWriter, r *http.Request) {
	http.MethodGet(url string, contentType string, body io.Reader)

}

func (h *PasteHandler) DeletePaste(w http.ResponseWriter, r *http.Request) {
	http.MethodDelete(url string, contentType string, body io.Reader)
}

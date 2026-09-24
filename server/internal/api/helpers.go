package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/1morshed1/prompt-hub/server/internal/template"
)

// writeJSON encodes v as JSON with the given status. Centralizing this keeps the
// content-type and encoding consistent across every handler.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// errorBody is the uniform error envelope clients can rely on.
type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// writeStoreError maps store sentinel errors to HTTP status codes so handlers
// don't each re-implement the mapping. Anything unrecognized is a 500.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "already exists / conflicts with an existing record")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// readJSON decodes the request body into v, rejecting unknown fields so typos in
// client payloads surface as errors instead of being silently ignored.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

// writeValidationError renders a template.ValidationError as a 422 (the input was
// well-formed JSON but semantically invalid), and anything else as a 400.
func writeRenderError(w http.ResponseWriter, err error) {
	var ve *template.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

// newUUID returns a random RFC-4122 v4 UUID string, used for request ids and
// assignment unit keys when the caller doesn't supply one. Implemented with
// crypto/rand so we avoid a dependency just for id generation.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

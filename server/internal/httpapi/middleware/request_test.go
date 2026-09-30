package middleware

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCorrelatePreservesOnlyValidRequestIDs(t *testing.T) {
	tests := []struct {
		name     string
		supplied string
		wantSame bool
	}{
		{name: "valid", supplied: "client-request_123", wantSame: true},
		{name: "missing"},
		{name: "invalid", supplied: "contains spaces"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var contextID string
			handler := Correlate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextID = RequestID(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set(HeaderRequestID, test.supplied)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			responseID := response.Header().Get(HeaderRequestID)
			if responseID == "" || responseID != contextID {
				t.Fatalf("response ID %q does not match context ID %q", responseID, contextID)
			}
			if test.wantSame && responseID != test.supplied {
				t.Fatalf("valid ID changed from %q to %q", test.supplied, responseID)
			}
			if !test.wantSame && responseID == test.supplied {
				t.Fatalf("invalid or missing ID was not replaced: %q", responseID)
			}
		})
	}
}

func TestLimitBodyRejectsReadsPastMaximum(t *testing.T) {
	handler := LimitBody(4, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := DrainBody(r); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(
					w,
					http.StatusRequestEntityTooLarge,
					"REQUEST_TOO_LARGE",
					"The request body exceeds the allowed size.",
					RequestID(r.Context()),
				)
				return
			}
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/future", bytes.NewBufferString("12345"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got status %d, want 413", response.Code)
	}
}

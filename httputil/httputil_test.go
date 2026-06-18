package httputil_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mrz1836/go-foundation/constants"
	"github.com/mrz1836/go-foundation/httputil"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		data       any
		wantStatus int
		wantBody   string
		wantCT     string
	}{
		{
			name:       "200 with struct",
			status:     http.StatusOK,
			data:       map[string]string{"key": "value"},
			wantStatus: http.StatusOK,
			wantBody:   `{"key":"value"}`,
			wantCT:     constants.ContentTypeJSON,
		},
		{
			name:       "201 created",
			status:     http.StatusCreated,
			data:       map[string]int{"id": 42},
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":42}`,
			wantCT:     constants.ContentTypeJSON,
		},
		{
			name:       "nil data produces null",
			status:     http.StatusOK,
			data:       nil,
			wantStatus: http.StatusOK,
			wantBody:   "null",
			wantCT:     constants.ContentTypeJSON,
		},
		{
			name:       "unmarshalable data falls back to 500",
			status:     http.StatusOK,
			data:       make(chan int), // channels cannot be marshaled
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"` + constants.ErrorMessageEncodingFailed + `","code":"` + constants.ErrorCodeInternalError + `"}`,
			wantCT:     constants.ContentTypeJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			httputil.WriteJSON(w, tt.status, tt.data)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if ct := w.Header().Get("Content-Type"); ct != tt.wantCT {
				t.Errorf("Content-Type = %q, want %q", ct, tt.wantCT)
			}

			if w.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusBadRequest, "MY_CODE", "something went wrong", "req-abc")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var resp httputil.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Error != "something went wrong" {
		t.Errorf("error = %q, want 'something went wrong'", resp.Error)
	}

	if resp.Code != "MY_CODE" {
		t.Errorf("code = %q, want 'MY_CODE'", resp.Code)
	}

	if resp.RequestID != "req-abc" {
		t.Errorf("request_id = %q, want 'req-abc'", resp.RequestID)
	}
}

func TestWriteErrorOmitsEmptyRequestID(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusInternalServerError, constants.ErrorCodeInternalError, constants.ErrorMessageInternalError, "")

	var resp httputil.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.RequestID != "" {
		t.Errorf("request_id should be omitted when empty, got %q", resp.RequestID)
	}

	// Confirm request_id key is absent from the raw JSON
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	if _, ok := raw["request_id"]; ok {
		t.Error("request_id key should be omitted from JSON when empty")
	}
}

func assertErrorResponse(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode, wantRequestID string) {
	t.Helper()

	if w.Code != wantStatus {
		t.Errorf("status = %d, want %d", w.Code, wantStatus)
	}

	if ct := w.Header().Get("Content-Type"); ct != constants.ContentTypeJSON {
		t.Errorf("Content-Type = %q, want %q", ct, constants.ContentTypeJSON)
	}

	var resp httputil.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Code != wantCode {
		t.Errorf("code = %q, want %q", resp.Code, wantCode)
	}

	if resp.RequestID != wantRequestID {
		t.Errorf("request_id = %q, want %q", resp.RequestID, wantRequestID)
	}
}

func TestNamedHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fn         func(w http.ResponseWriter, requestID string)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "WriteNotFound",
			fn:         httputil.WriteNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   constants.ErrorCodeNotFound,
		},
		{
			name: "WriteBadRequest",
			fn: func(w http.ResponseWriter, requestID string) {
				httputil.WriteBadRequest(w, "invalid field", requestID)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   constants.ErrorCodeBadRequest,
		},
		{
			name:       "WriteInternalError",
			fn:         httputil.WriteInternalError,
			wantStatus: http.StatusInternalServerError,
			wantCode:   constants.ErrorCodeInternalError,
		},
		{
			name:       "WriteMethodNotAllowed",
			fn:         httputil.WriteMethodNotAllowed,
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   constants.ErrorCodeMethodNotAllowed,
		},
		{
			name:       "WriteUnauthorized",
			fn:         httputil.WriteUnauthorized,
			wantStatus: http.StatusUnauthorized,
			wantCode:   constants.ErrorCodeUnauthorized,
		},
		{
			name:       "WriteForbidden",
			fn:         httputil.WriteForbidden,
			wantStatus: http.StatusForbidden,
			wantCode:   constants.ErrorCodeForbidden,
		},
		{
			name: "WriteConflict",
			fn: func(w http.ResponseWriter, requestID string) {
				httputil.WriteConflict(w, "resource already exists", requestID)
			},
			wantStatus: http.StatusConflict,
			wantCode:   constants.ErrorCodeConflict,
		},
		{
			name:       "WriteTooManyRequests",
			fn:         httputil.WriteTooManyRequests,
			wantStatus: http.StatusTooManyRequests,
			wantCode:   constants.ErrorCodeTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			tt.fn(w, "test-req-id")
			assertErrorResponse(t, w, tt.wantStatus, tt.wantCode, "test-req-id")
		})
	}
}

func BenchmarkWriteJSON(b *testing.B) {
	data := map[string]string{"message": "success", "id": "12345"}

	b.ResetTimer()

	for range b.N {
		w := httptest.NewRecorder()
		httputil.WriteJSON(w, http.StatusOK, data)
	}
}

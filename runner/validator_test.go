package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestValidator builds a jevValidator pointed at srv with a sleep stub so
// retry backoff does not slow the suite down.
func newTestValidator(t *testing.T, srv *httptest.Server, threshold float64) *jevValidator {
	t.Helper()
	v := NewJevValidator(srv.Client(), srv.URL, "test-key", "jev-latest", threshold, time.Second).(*jevValidator)
	v.sleep = func(time.Duration) {}
	return v
}

// respondNoul returns a handler that answers with the given noul probability.
func respondNoul(p float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"safe": map[string]any{"type": "noul", "noul": p}},
		})
	}
}

func TestValidateSafe(t *testing.T) {
	srv := httptest.NewServer(respondNoul(0.97))
	defer srv.Close()

	result, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls -la")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Safe {
		t.Error("expected the command to be safe")
	}
	if !strings.Contains(result.Reason, "0.97") {
		t.Errorf("reason = %q, want it to mention the probability", result.Reason)
	}
}

func TestValidateUnsafe(t *testing.T) {
	srv := httptest.NewServer(respondNoul(0.13))
	defer srv.Close()

	result, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "rm -rf /")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Safe {
		t.Error("expected the command to be unsafe")
	}
}

// TestValidateThresholdIsInclusive pins the boundary so a probability exactly
// at the threshold still runs.
func TestValidateThresholdIsInclusive(t *testing.T) {
	srv := httptest.NewServer(respondNoul(0.8))
	defer srv.Close()

	result, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "pwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Safe {
		t.Error("expected a probability equal to the threshold to be safe")
	}
}

// TestValidateSendsRequest verifies the wire format the API expects.
func TestValidateSendsRequest(t *testing.T) {
	var got jevRequest
	var auth, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		respondNoul(0.9)(w, r)
	}))
	defer srv.Close()

	if _, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "uname -a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", auth)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if got.Model != "jev-latest" {
		t.Errorf("model = %q, want jev-latest", got.Model)
	}
	if got.State != "uname -a" {
		t.Errorf("state = %q, want the submitted command", got.State)
	}
	question, ok := got.Questions["safe"]
	if !ok {
		t.Fatal(`expected a "safe" question`)
	}
	if question.Type != "noul" {
		t.Errorf("question type = %q, want noul", question.Type)
	}
	if question.Criteria["true"] == "" || question.Criteria["false"] == "" {
		t.Error("expected both criteria to be filled in")
	}
}

// TestValidateTransportErrorIsUnavailable verifies that an unreachable API is
// reported as unavailable rather than as a rejection.
func TestValidateTransportErrorIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	client := srv.Client()
	srv.Close()

	v := NewJevValidator(client, url, "k", "jev-latest", 0.8, time.Second).(*jevValidator)
	v.sleep = func(time.Duration) {}

	_, err := v.Validate(context.Background(), "ls")
	var unavailable *ValidationUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v, want ValidationUnavailableError", err)
	}
}

func TestValidateUnauthorizedIsUnavailable(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
	var unavailable *ValidationUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v, want ValidationUnavailableError", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1: a bad key is not worth retrying", calls)
	}
}

func TestValidateUnprocessableIsTerminal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
	if err == nil {
		t.Fatal("expected an error")
	}
	var unavailable *ValidationUnavailableError
	if errors.As(err, &unavailable) {
		t.Fatal("a malformed request is our fault, not the API's")
	}
}

func TestValidateRetriesRateLimitThenSucceeds(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		respondNoul(0.95)(w, r)
	}))
	defer srv.Close()

	result, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Safe {
		t.Error("expected the retried call to succeed")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestValidateRetriesExhausted(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(529)
	}))
	defer srv.Close()

	_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
	var unavailable *ValidationUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v, want ValidationUnavailableError", err)
	}
	if calls != jevMaxRetries+1 {
		t.Errorf("calls = %d, want %d", calls, jevMaxRetries+1)
	}
}

// TestValidateBacksOffExponentially pins the delays so a busy API is not
// hammered at a fixed interval.
func TestValidateBacksOffExponentially(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	v := newTestValidator(t, srv, 0.8)
	var delays []time.Duration
	v.sleep = func(d time.Duration) { delays = append(delays, d) }

	if _, err := v.Validate(context.Background(), "ls"); err == nil {
		t.Fatal("expected an error")
	}
	want := []time.Duration{jevRetryBaseDelay, 2 * jevRetryBaseDelay}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Errorf("delays[%d] = %v, want %v", i, delays[i], want[i])
		}
	}
}

func TestValidateMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	if _, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestValidateMissingAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer srv.Close()

	_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
	if err == nil || !strings.Contains(err.Error(), "no \"safe\" answer") {
		t.Fatalf("error = %v, want a missing-answer error", err)
	}
}

func TestValidateWrongAnswerShape(t *testing.T) {
	cases := map[string]string{
		"not a noul":  `{"answers":{"safe":{"type":"choice","choice":"yes"}}}`,
		"noul absent": `{"answers":{"safe":{"type":"noul"}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
			if err == nil || !strings.Contains(err.Error(), "is not a noul") {
				t.Fatalf("error = %v, want a shape error", err)
			}
		})
	}
}

func TestValidateProbabilityOutOfRange(t *testing.T) {
	for _, body := range []string{
		`{"answers":{"safe":{"type":"noul","noul":1.5}}}`,
		`{"answers":{"safe":{"type":"noul","noul":-0.1}}}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))

		_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "outside [0,1]") {
			t.Fatalf("error = %v, want a range error", err)
		}
	}
}

// TestValidateBuildRequestError covers the unusable-endpoint path, which
// http.NewRequestWithContext rejects before any call is made.
func TestValidateBuildRequestError(t *testing.T) {
	v := NewJevValidator(http.DefaultClient, "://not a url", "k", "jev-latest", 0.8, time.Second).(*jevValidator)
	v.sleep = func(time.Duration) {}

	if _, err := v.Validate(context.Background(), "ls"); err == nil {
		t.Fatal("expected an error")
	}
}

// TestValidateReadBodyError covers a response whose body dies mid-read.
func TestValidateReadBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()

	_, err := newTestValidator(t, srv, 0.8).Validate(context.Background(), "ls")
	var unavailable *ValidationUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v, want ValidationUnavailableError", err)
	}
}

func TestValidationUnavailableError(t *testing.T) {
	cause := errors.New("boom")
	err := &ValidationUnavailableError{Cause: cause}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("Error() = %q, want it to mention the cause", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("expected Unwrap to expose the cause")
	}
}

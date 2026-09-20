package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// jevEndpoint is the TypeSafe System One inference endpoint.
const jevEndpoint = "https://api.typesafe.ai/v1/systemone"

// jevQuestionKey names the single noul question the runner asks. Jev answers
// every question independently, so one key is enough to carry the verdict.
const jevQuestionKey = "safe"

// jevInstructions is the question Jev answers for each submitted command.
const jevInstructions = "Is this shell command safe to run in a disposable demo container?"

// jevSafeCriteria and jevUnsafeCriteria pin down what "safe" means for this
// deployment so the returned probability is calibrated against our threat
// model rather than a generic one.
const (
	jevSafeCriteria   = "Read-only inspection, or a change confined to the container that cannot destroy data, leak secrets, or abuse the network"
	jevUnsafeCriteria = "Destructive to the filesystem, exfiltrates credentials or data, abuses the network, escalates privilege, or exhausts resources"
)

// jevMaxRetries is the number of retries after the initial attempt. Only 429
// and 529 are retried; every other failure is terminal.
const jevMaxRetries = 2

// jevRetryBaseDelay is the first backoff interval. It doubles per retry.
const jevRetryBaseDelay = 200 * time.Millisecond

// ValidationResult is the verdict for a single command.
type ValidationResult struct {
	// Safe reports whether the command may be executed.
	Safe bool
	// Reason explains the verdict in a form that can be shown to the user.
	Reason string
}

// Validator decides whether a command may run.
type Validator interface {
	Validate(ctx context.Context, command string) (ValidationResult, error)
}

// ValidationUnavailableError marks a failure that originates outside the
// runner: the Jev API was unreachable, overloaded, or rejected our credentials.
// The runner still refuses to execute, but reports 503 rather than 403 so the
// caller can tell "we could not judge" from "we judged and said no".
type ValidationUnavailableError struct {
	Cause error
}

// Error implements the error interface.
func (e *ValidationUnavailableError) Error() string {
	return fmt.Sprintf("validation unavailable: %v", e.Cause)
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *ValidationUnavailableError) Unwrap() error {
	return e.Cause
}

// jevQuestion is one typed question in a System One request.
type jevQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// jevRequest is the System One request body.
type jevRequest struct {
	Model     string                 `json:"model"`
	State     string                 `json:"state"`
	Questions map[string]jevQuestion `json:"questions"`
}

// jevAnswer is one typed answer. Only the noul shape is requested, whose value
// is itself the probability of "yes", so there is no separate confidence field.
type jevAnswer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

// jevResponse is the System One response body.
type jevResponse struct {
	Answers map[string]jevAnswer `json:"answers"`
}

// jevValidator asks Jev whether a command is safe.
type jevValidator struct {
	client    *http.Client
	endpoint  string
	apiKey    string
	model     string
	threshold float64
	timeout   time.Duration
	sleep     func(time.Duration)
}

// NewJevValidator builds a Validator backed by the Jev System One model.
func NewJevValidator(client *http.Client, endpoint, apiKey, model string, threshold float64, timeout time.Duration) Validator {
	return &jevValidator{
		client:    client,
		endpoint:  endpoint,
		apiKey:    apiKey,
		model:     model,
		threshold: threshold,
		timeout:   timeout,
		sleep:     time.Sleep,
	}
}

// Validate asks Jev for the probability that command is safe and compares it
// against the configured threshold.
func (v *jevValidator) Validate(ctx context.Context, command string) (ValidationResult, error) {
	// jevRequest holds only strings and string maps, so json.Marshal cannot fail.
	body, _ := json.Marshal(jevRequest{
		Model: v.model,
		State: command,
		Questions: map[string]jevQuestion{
			jevQuestionKey: {
				Type:         "noul",
				Instructions: jevInstructions,
				Criteria:     map[string]string{"true": jevSafeCriteria, "false": jevUnsafeCriteria},
			},
		},
	})

	for attempt := 0; ; attempt++ {
		payload, retryable, err := v.post(ctx, body)
		if err == nil {
			return v.verdict(payload)
		}
		if !retryable || attempt == jevMaxRetries {
			return ValidationResult{}, err
		}
		v.sleep(jevRetryBaseDelay << attempt)
	}
}

// post performs one request and returns the response body. The second return
// value reports whether the failure is worth retrying.
func (v *jevValidator) post(ctx context.Context, body []byte) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("jev: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+v.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, true, &ValidationUnavailableError{Cause: err}
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, &ValidationUnavailableError{Cause: err}
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		return payload, false, nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 529:
		return nil, true, &ValidationUnavailableError{Cause: fmt.Errorf("jev: status %d", resp.StatusCode)}
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusUnauthorized:
		return nil, false, &ValidationUnavailableError{Cause: fmt.Errorf("jev: status %d", resp.StatusCode)}
	default:
		return nil, false, fmt.Errorf("jev: status %d", resp.StatusCode)
	}
}

// verdict turns a successful response body into a ValidationResult.
func (v *jevValidator) verdict(payload []byte) (ValidationResult, error) {
	var parsed jevResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ValidationResult{}, fmt.Errorf("jev: decode response: %w", err)
	}
	answer, ok := parsed.Answers[jevQuestionKey]
	if !ok {
		return ValidationResult{}, fmt.Errorf("jev: response has no %q answer", jevQuestionKey)
	}
	if answer.Type != "noul" || answer.Noul == nil {
		return ValidationResult{}, fmt.Errorf("jev: %q answer is not a noul", jevQuestionKey)
	}
	p := *answer.Noul
	if p < 0 || p > 1 {
		return ValidationResult{}, fmt.Errorf("jev: noul %v is outside [0,1]", p)
	}
	return ValidationResult{
		Safe:   p >= v.threshold,
		Reason: fmt.Sprintf("safety probability %.2f (threshold %.2f)", p, v.threshold),
	}, nil
}

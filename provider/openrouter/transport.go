package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (p *Provider) send(ctx context.Context, method, path string, payload []byte, accept string) (*http.Response, error) {
	for attempt := 0; attempt < maximumAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("openrouter: create request: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
		request.Header.Set("Accept", accept)
		if len(payload) != 0 {
			request.Header.Set("Content-Type", "application/json")
		}
		if p.httpReferer != "" {
			request.Header.Set("HTTP-Referer", p.httpReferer)
		}
		if p.appTitle != "" {
			request.Header.Set("X-OpenRouter-Title", p.appTitle)
		}

		response, requestErr := p.client.Do(request)
		if requestErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A failed paid POST is ambiguous: the gateway may have accepted and
			// billed it before the connection failed. Retry only safe catalog-style
			// requests automatically; callers can explicitly retry a model turn.
			if safeToRetryTransportError(method) && attempt+1 < maximumAttempts {
				if err := waitForRetry(ctx, attempt, ""); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("openrouter: send request: %w", requestErr)
		}

		if retryableStatus(response.StatusCode) && attempt+1 < maximumAttempts {
			retryAfter := response.Header.Get("Retry-After")
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maximumErrorBody))
			_ = response.Body.Close()
			if err := waitForRetry(ctx, attempt, retryAfter); err != nil {
				return nil, err
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			defer response.Body.Close()
			return nil, decodeHTTPError(response)
		}
		return response, nil
	}
	return nil, fmt.Errorf("openrouter: request retries exhausted")
}

func safeToRetryTransportError(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusConflict ||
		status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
}

func waitForRetry(ctx context.Context, attempt int, retryAfter string) error {
	delay := retryDelay(retryAfter, time.Now())
	if delay <= 0 {
		delay = initialRetryBackoff * time.Duration(1<<attempt)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	if when, err := http.ParseTime(value); err == nil && when.After(now) {
		return when.Sub(now)
	}
	return 0
}

func decodeHTTPError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maximumErrorBody))
	apiError := &APIError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(body))}
	var envelope struct {
		Error     json.RawMessage `json:"error"`
		ErrorType string          `json:"error_type"`
		Message   string          `json:"message"`
		Code      json.RawMessage `json:"code"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		apiError.ErrorType = envelope.ErrorType
		apiError.Message = firstNonEmpty(envelope.Message, apiError.Message)
		apiError.Code = rawScalarString(envelope.Code)
		if len(envelope.Error) != 0 && string(envelope.Error) != "null" {
			var detail struct {
				Code      json.RawMessage `json:"code"`
				Message   string          `json:"message"`
				Type      string          `json:"type"`
				ErrorType string          `json:"error_type"`
				Metadata  struct {
					ErrorType string `json:"error_type"`
				} `json:"metadata"`
			}
			if json.Unmarshal(envelope.Error, &detail) == nil {
				apiError.Code = firstNonEmpty(rawScalarString(detail.Code), detail.Type, apiError.Code)
				apiError.Message = firstNonEmpty(detail.Message, apiError.Message)
				apiError.ErrorType = firstNonEmpty(
					detail.ErrorType, detail.Metadata.ErrorType, apiError.ErrorType,
				)
			}
		}
	}
	if apiError.Message == "" {
		apiError.Message = http.StatusText(response.StatusCode)
	}
	return apiError
}

func rawScalarString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return strings.TrimSpace(string(raw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

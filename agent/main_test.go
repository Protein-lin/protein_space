package main

import (
	"net/http"
	"testing"
)

func TestModelProtocol(t *testing.T) {
	tests := []struct {
		model    string
		protocol string
	}{
		{"gpt-5.6-terra", "responses"},
		{"gpt-5.6-sol", "responses"},
		{"gpt-5.5", "responses"},
		{"o3-mini", "responses"},
		{"qwen-plus", "chat_completions"},
		{"vendor-custom-model", "chat_completions"},
	}
	for _, test := range tests {
		if got := modelProtocol(test.model); got != test.protocol {
			t.Errorf("modelProtocol(%q) = %q, want %q", test.model, got, test.protocol)
		}
	}
}

func TestUpstreamErrorFallbackPolicy(t *testing.T) {
	tests := []struct {
		statusCode int
		fallback   bool
	}{
		{http.StatusNotFound, true},
		{http.StatusMethodNotAllowed, true},
		{http.StatusUnsupportedMediaType, true},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusServiceUnavailable, false},
	}
	for _, test := range tests {
		err := &upstreamError{statusCode: test.statusCode}
		if got := shouldFallbackProtocol(err); got != test.fallback {
			t.Errorf("shouldFallbackProtocol(%d) = %t, want %t", test.statusCode, got, test.fallback)
		}
	}
}

func TestParseStreamError(t *testing.T) {
	code, message, errorType := parseStreamError(map[string]any{
		"code":    "server_is_overloaded",
		"message": "Our servers are currently overloaded.",
		"type":    "service_unavailable",
	})
	if code != "server_is_overloaded" || message == "" || errorType != "service_unavailable" {
		t.Fatalf("unexpected parsed stream error: %q, %q, %q", code, message, errorType)
	}
}

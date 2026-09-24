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
		{"gpt-6-astra", "responses"},
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

func TestBuiltinModelRegistry(t *testing.T) {
	models := builtinModels()
	if len(models) != len(modelProtocolConfigs) {
		t.Fatalf("builtin model count = %d, want %d", len(models), len(modelProtocolConfigs))
	}
	for _, config := range modelProtocolConfigs {
		if got := modelProtocol(config.ID); got != string(config.Protocol) {
			t.Errorf("registry protocol for %q = %q, want %q", config.ID, got, config.Protocol)
		}
	}
}

func TestProtocolEndpointHelpers(t *testing.T) {
	tests := []struct {
		base        string
		responses   string
		completions string
		models      string
	}{
		{
			base:        "https://provider.example/v1/chat/completions",
			responses:   "https://provider.example/v1/responses",
			completions: "https://provider.example/v1/chat/completions",
			models:      "https://provider.example/v1/models",
		},
		{
			base:        "https://provider.example/v1/responses",
			responses:   "https://provider.example/v1/responses",
			completions: "https://provider.example/v1/chat/completions",
			models:      "https://provider.example/v1/models",
		},
		{
			base:        "https://provider.example",
			responses:   "https://provider.example/v1/responses",
			completions: "https://provider.example/v1/chat/completions",
			models:      "https://provider.example/v1/models",
		},
	}
	for _, test := range tests {
		if got := responsesURL(test.base); got != test.responses {
			t.Errorf("responsesURL(%q) = %q, want %q", test.base, got, test.responses)
		}
		if got := chatCompletionsURL(test.base); got != test.completions {
			t.Errorf("chatCompletionsURL(%q) = %q, want %q", test.base, got, test.completions)
		}
		if got := modelsURL(test.base); got != test.models {
			t.Errorf("modelsURL(%q) = %q, want %q", test.base, got, test.models)
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

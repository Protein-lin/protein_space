package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// proxyChatCompletions translates the common request into the legacy
// Chat Completions JSON shape and normalizes its delta stream.
func proxyChatCompletions(ctx context.Context, w http.ResponseWriter, upstream string, apiKey string, req ChatRequest) error {
	body, _ := json.Marshal(map[string]any{"model": req.Model, "messages": req.Messages, "stream": true})
	out, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream, bytes.NewReader(body))
	if err != nil {
		return err
	}
	out.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		out.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 0}).Do(out)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return newHTTPUpstreamError(string(ProtocolChatCompletions), upstream, resp, body)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			writeRaw(w, "data: [DONE]\n\n")
			return nil
		}
		var value map[string]any
		if json.Unmarshal([]byte(payload), &value) != nil {
			continue
		}
		if streamError, ok := value["error"]; ok {
			code, message, errorType := parseStreamError(streamError)
			statusCode := http.StatusBadGateway
			if code == "server_is_overloaded" || errorType == "service_unavailable" {
				statusCode = http.StatusServiceUnavailable
			}
			if code == "rate_limit_exceeded" || errorType == "rate_limit_error" {
				statusCode = http.StatusTooManyRequests
			}
			return &upstreamError{
				protocol: string(ProtocolChatCompletions), url: upstream, statusCode: statusCode,
				status: http.StatusText(statusCode), message: message,
			}
		}
		if choices, ok := value["choices"].([]any); ok && len(choices) > 0 {
			if delta, ok := choices[0].(map[string]any)["delta"].(map[string]any); ok {
				if text, ok := delta["content"].(string); ok {
					writeEvent(w, map[string]string{"delta": text, "model": req.Model})
				}
			}
		}
	}
	return scanner.Err()
}

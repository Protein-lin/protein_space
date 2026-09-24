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

// proxyResponses translates the app's common ChatRequest into the Responses
// input format and translates typed Responses SSE events back to the app's
// internal {"delta": "..."} stream.
func proxyResponses(ctx context.Context, w http.ResponseWriter, upstream, apiKey string, req ChatRequest) error {
	input := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		input = append(input, map[string]any{"role": m.Role, "content": responsesContent(m)})
	}
	body, _ := json.Marshal(map[string]any{"model": req.Model, "input": input, "stream": true})
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
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return newHTTPUpstreamError(string(ProtocolResponses), upstream, resp, b)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
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
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Error any    `json:"error"`
		}
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		if event.Error != nil {
			code, message, errorType := parseStreamError(event.Error)
			statusCode := http.StatusBadGateway
			if code == "server_is_overloaded" || errorType == "service_unavailable" {
				statusCode = http.StatusServiceUnavailable
			}
			if code == "rate_limit_exceeded" || errorType == "rate_limit_error" {
				statusCode = http.StatusTooManyRequests
			}
			return &upstreamError{
				protocol: string(ProtocolResponses), url: upstream, statusCode: statusCode,
				status: http.StatusText(statusCode), message: message,
			}
		}
		if event.Type == "response.output_text.delta" && event.Delta != "" {
			writeEvent(w, map[string]string{"delta": event.Delta, "model": req.Model})
		}
		if event.Type == "response.completed" {
			writeRaw(w, "data: [DONE]\n\n")
			return nil
		}
	}
	return scanner.Err()
}

func responsesContent(message Message) []map[string]string {
	var text string
	if json.Unmarshal(message.Content, &text) == nil {
		contentType := "input_text"
		if message.Role == "assistant" {
			contentType = "output_text"
		}
		return []map[string]string{{"type": contentType, "text": text}}
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if json.Unmarshal(message.Content, &parts) != nil {
		return []map[string]string{{"type": "input_text", "text": string(message.Content)}}
	}
	out := make([]map[string]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" {
			contentType := "input_text"
			if message.Role == "assistant" {
				contentType = "output_text"
			}
			out = append(out, map[string]string{"type": contentType, "text": part.Text})
		}
		if part.Type == "image_url" && part.ImageURL.URL != "" {
			out = append(out, map[string]string{"type": "input_image", "image_url": part.ImageURL.URL})
		}
	}
	if len(out) == 0 {
		return []map[string]string{{"type": "input_text", "text": ""}}
	}
	return out
}

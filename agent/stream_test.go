package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProtocolStreams(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for _, scenario := range []string{"success", "truncated", "error", "large"} {
			t.Run(protocol+"/"+scenario, func(t *testing.T) {
				text := "你好🌏"
				if scenario == "large" {
					text = strings.Repeat("文", 30000)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer test-key" {
						t.Error("missing upstream authentication")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["stream"] != true {
						t.Error("invalid stream request")
					}
					if scenario == "error" {
						io.WriteString(w, "data: {\"error\":{\"message\":\"overloaded\",\"code\":\"server_is_overloaded\"}}\n\n")
						return
					}
					var event any = map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": text}}}}
					if protocol == "responses" {
						event = map[string]string{"type": "response.output_text.delta", "delta": text}
					}
					b, _ := json.Marshal(event)
					io.WriteString(w, "data: "+string(b)+"\n\n")
					if scenario != "truncated" {
						if protocol == "responses" {
							io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
						} else {
							io.WriteString(w, "data: [DONE]\n\n")
						}
					}
				}))
				defer server.Close()
				proxy := proxyChatCompletions
				if protocol == "responses" {
					proxy = proxyResponses
				}
				w := httptest.NewRecorder()
				err := proxy(context.Background(), w, server.URL, "test-key", ChatRequest{Model: "test", Messages: []Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}})
				if scenario == "error" || scenario == "truncated" {
					if err == nil {
						t.Fatal("incomplete or failed stream reported success")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(w.Body.String(), text) || !strings.Contains(w.Body.String(), "[DONE]") {
					t.Fatal("missing content or completion")
				}
			})
		}
	}
}

func TestResponsesAttachments(t *testing.T) {
	m := Message{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"描述图片"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]`)}
	parts := responsesContent(m)
	if len(parts) != 2 || parts[0]["type"] != "input_text" || parts[1]["type"] != "input_image" || parts[1]["image_url"] != "data:image/png;base64,AA==" {
		t.Fatalf("invalid converted image: %v", parts)
	}
	parts = responsesContent(Message{Role: "assistant", Content: json.RawMessage(`"回复"`)})
	if parts[0]["type"] != "output_text" {
		t.Fatal("assistant role lost")
	}
}

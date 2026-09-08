package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatRequest struct {
	ConversationID string    `json:"conversation_id"`
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	UpstreamURL    string    `json:"upstream_url,omitempty"`
	APIKey         string    `json:"api_key,omitempty"`
}
type Service struct {
	models       []map[string]string
	upstream     string
	key          string
	defaultModel string
}

func main() {
	s := &Service{models: []map[string]string{{"id": "gpt-5.5", "name": "GPT-5.5"}, {"id": "gpt-4o-mini", "name": "GPT-4o mini"}, {"id": "qwen-plus", "name": "通义千问 Plus"}}, upstream: os.Getenv("AGENT_UPSTREAM_URL"), key: os.Getenv("AGENT_API_KEY"), defaultModel: env("AGENT_MODEL", "gpt-5.5")}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.health)
	mux.HandleFunc("/v1/models", s.modelsHandler)
	mux.HandleFunc("/v1/chat/stream", s.chat)
	server := &http.Server{Addr: env("AGENT_ADDR", ":8090"), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("agent listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
func (s *Service) health(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "agent"})
}
func (s *Service) modelsHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{"models": s.models, "default": s.defaultModel})
}
func (s *Service) chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req ChatRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil || len(req.Messages) == 0 {
		http.Error(w, "invalid chat request", 400)
		return
	}
	if req.Model == "" {
		req.Model = s.defaultModel
	}
	setupSSE(w)
	ctx := r.Context()
	if req.UpstreamURL != "" {
		if err := proxy(ctx, w, req.UpstreamURL, req.APIKey, req); err == nil {
			return
		}
	} else if s.upstream != "" {
		if err := proxy(ctx, w, s.upstream, s.key, req); err == nil {
			return
		}
	}
	last := req.Messages[len(req.Messages)-1].Content
	answer := demoAnswer(req.Model, last)
	for _, chunk := range chunks(answer, 18) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		writeEvent(w, map[string]string{"delta": chunk, "model": req.Model})
		time.Sleep(35 * time.Millisecond)
	}
	writeRaw(w, "data: [DONE]\n\n")
}
func proxy(ctx context.Context, w http.ResponseWriter, upstream string, apiKey string, req ChatRequest) error {
	b, _ := json.Marshal(map[string]any{"model": req.Model, "messages": req.Messages, "stream": true})
	out, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream, bytes.NewReader(b))
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
		return fmt.Errorf("upstream status %s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				writeRaw(w, "data: [DONE]\n\n")
				return nil
			}
			var v map[string]any
			if json.Unmarshal([]byte(payload), &v) == nil {
				if choices, ok := v["choices"].([]any); ok && len(choices) > 0 {
					if d, ok := choices[0].(map[string]any)["delta"].(map[string]any); ok {
						if t, ok := d["content"].(string); ok {
							writeEvent(w, map[string]string{"delta": t, "model": req.Model})
						}
					}
				}
			}
		}
	}
	return scanner.Err()
}
func setupSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
func writeEvent(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	writeRaw(w, "data: "+string(b)+"\n\n")
}
func writeRaw(w http.ResponseWriter, s string) {
	io.WriteString(w, s)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
func chunks(s string, n int) []string {
	r := []rune(s)
	out := []string{}
	for len(r) > 0 {
		z := n
		if len(r) < z {
			z = len(r)
		}
		out = append(out, string(r[:z]))
		r = r[z:]
	}
	return out
}
func demoAnswer(model, q string) string {
	return fmt.Sprintf("已使用 %s 完成分析。\n\n针对你的问题“%s”，建议先确认 request_id、命中规则和源站响应，再结合拦截日志定位。如果你提供具体日志或规则编号，我可以继续细化排查步骤。", model, q)
}
func env(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}

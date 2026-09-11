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
	models := s.models
	if s.upstream != "" {
		if upstreamModels, err := fetchUpstreamModels(r.Context(), s.upstream, s.key); err == nil && len(upstreamModels) > 0 {
			models = upstreamModels
		} else if err != nil {
			log.Printf("upstream models unavailable: %v", err)
		}
	}
	defaultModel := s.defaultModel
	defaultFound := false
	for _, model := range models {
		if model["id"] == defaultModel {
			defaultFound = true
			break
		}
	}
	if !defaultFound && len(models) > 0 {
		defaultModel = models[0]["id"]
	}
	json.NewEncoder(w).Encode(map[string]any{"models": models, "default": defaultModel})
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
		log.Printf("chat upstream=provider model=%s", req.Model)
		if err := s.proxyByModel(ctx, w, req.UpstreamURL, req.APIKey, req); err == nil {
			return
		} else {
			log.Printf("chat upstream provider failed model=%s error=%v", req.Model, err)
			writeEvent(w, map[string]string{"error": "模型服务调用失败，请检查 API 地址、Key 和模型名称"})
			writeRaw(w, "data: [DONE]\n\n")
			return
		}
	} else if s.upstream != "" {
		log.Printf("chat upstream=env model=%s", req.Model)
		if err := s.proxyByModel(ctx, w, s.upstream, s.key, req); err == nil {
			return
		} else {
			log.Printf("chat upstream env failed model=%s error=%v", req.Model, err)
			writeEvent(w, map[string]string{"error": "模型服务调用失败，请检查 API 地址、Key 和模型名称"})
			writeRaw(w, "data: [DONE]\n\n")
			return
		}
	}
	log.Printf("chat demo model=%s: AGENT_UPSTREAM_URL is empty", req.Model)
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
func (s *Service) proxyByModel(ctx context.Context, w http.ResponseWriter, upstream, apiKey string, req ChatRequest) error {
	if prefersResponses(req.Model) {
		if err := proxyResponses(ctx, w, responsesURL(upstream), apiKey, req); err == nil {
			return nil
		} else {
			log.Printf("responses protocol failed model=%s error=%v; trying chat completions", req.Model, err)
			return proxy(ctx, w, chatCompletionsURL(upstream), apiKey, req)
		}
	}
	if err := proxy(ctx, w, chatCompletionsURL(upstream), apiKey, req); err == nil {
		return nil
	} else {
		log.Printf("chat completions protocol failed model=%s error=%v; trying responses", req.Model, err)
		return proxyResponses(ctx, w, responsesURL(upstream), apiKey, req)
	}
}

func responsesURL(upstream string) string {
	if strings.HasSuffix(upstream, "/v1/responses") {
		return upstream
	}
	if strings.HasSuffix(upstream, "/v1/chat/completions") {
		return strings.TrimSuffix(upstream, "/v1/chat/completions") + "/v1/responses"
	}
	if strings.HasSuffix(upstream, "/chat/completions") {
		return strings.TrimSuffix(upstream, "/chat/completions") + "/responses"
	}
	return strings.TrimRight(upstream, "/") + "/v1/responses"
}

func chatCompletionsURL(upstream string) string {
	if strings.HasSuffix(upstream, "/v1/chat/completions") {
		return upstream
	}
	if strings.HasSuffix(upstream, "/v1/responses") {
		return strings.TrimSuffix(upstream, "/v1/responses") + "/v1/chat/completions"
	}
	if strings.HasSuffix(upstream, "/chat/completions") {
		return upstream
	}
	return strings.TrimRight(upstream, "/") + "/v1/chat/completions"
}

func modelsURL(upstream string) string {
	for _, suffix := range []string{"/v1/chat/completions", "/v1/responses", "/chat/completions", "/responses"} {
		if strings.HasSuffix(upstream, suffix) {
			return strings.TrimSuffix(upstream, suffix) + "/v1/models"
		}
	}
	return strings.TrimRight(upstream, "/") + "/v1/models"
}

func prefersResponses(model string) bool {
	model = strings.ToLower(model)
	return strings.HasPrefix(model, "gpt-5") ||
		strings.Contains(model, "responses") ||
		strings.Contains(model, "o1") ||
		strings.Contains(model, "o3") ||
		strings.Contains(model, "o4")
}

func fetchUpstreamModels(ctx context.Context, upstream, apiKey string) ([]map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL(upstream), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("models upstream status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Models []map[string]any `json:"models"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	raw := payload.Models
	if len(raw) == 0 {
		raw = payload.Data
	}
	models := make([]map[string]string, 0, len(raw))
	for _, item := range raw {
		id, ok := item["id"].(string)
		if !ok || id == "" {
			continue
		}
		name := id
		if value, ok := item["name"].(string); ok && value != "" {
			name = value
		}
		model := map[string]string{"id": id, "name": name}
		if protocol, ok := item["protocol"].(string); ok && protocol != "" {
			model["protocol"] = protocol
		}
		models = append(models, model)
	}
	return models, nil
}

func proxyResponses(ctx context.Context, w http.ResponseWriter, upstream, apiKey string, req ChatRequest) error {
	input := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		contentType := "input_text"
		if m.Role == "assistant" {
			// Responses API uses output_text for assistant turns; input_text is
			// only valid for user/system/developer input content.
			contentType = "output_text"
		}
		input = append(input, map[string]any{"role": m.Role, "content": []map[string]string{{"type": contentType, "text": m.Content}}})
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
		return fmt.Errorf("responses upstream status %s: %s", resp.Status, strings.TrimSpace(string(b)))
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
			return fmt.Errorf("responses error: %v", event.Error)
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

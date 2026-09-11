package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type upstreamError struct {
	protocol     string
	url          string
	statusCode   int
	status       string
	contentType  string
	message      string
	responseBody string
}

func (e *upstreamError) Error() string {
	parts := []string{fmt.Sprintf("%s upstream status %s", e.protocol, e.status)}
	if e.url != "" {
		parts = append(parts, "url="+e.url)
	}
	if e.contentType != "" {
		parts = append(parts, "content_type="+e.contentType)
	}
	if e.message != "" {
		parts = append(parts, "message="+e.message)
	}
	if e.responseBody != "" && e.responseBody != e.message {
		parts = append(parts, "body="+e.responseBody)
	}
	return strings.Join(parts, " ")
}

func (e *upstreamError) protocolMismatch() bool {
	return e.statusCode == http.StatusNotFound ||
		e.statusCode == http.StatusMethodNotAllowed ||
		e.statusCode == http.StatusUnsupportedMediaType
}

func main() {
	s := &Service{models: builtinModels(), upstream: os.Getenv("AGENT_UPSTREAM_URL"), key: os.Getenv("AGENT_API_KEY"), defaultModel: env("AGENT_MODEL", "gpt-5.6-terra")}
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
			models = mergeModels(upstreamModels, s.models)
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
			log.Printf("chat upstream provider failed model=%s protocol=%s error=%v", req.Model, modelProtocol(req.Model), err)
			writeEvent(w, map[string]string{"error": userFacingUpstreamError(err)})
			writeRaw(w, "data: [DONE]\n\n")
			return
		}
	} else if s.upstream != "" {
		log.Printf("chat upstream=env model=%s", req.Model)
		if err := s.proxyByModel(ctx, w, s.upstream, s.key, req); err == nil {
			return
		} else {
			log.Printf("chat upstream env failed model=%s protocol=%s error=%v", req.Model, modelProtocol(req.Model), err)
			writeEvent(w, map[string]string{"error": userFacingUpstreamError(err)})
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
	protocol := modelProtocol(req.Model)
	if protocol == "responses" {
		if err := proxyResponses(ctx, w, responsesURL(upstream), apiKey, req); err == nil {
			return nil
		} else if shouldFallbackProtocol(err) {
			log.Printf("responses protocol failed model=%s error=%v; trying chat completions", req.Model, err)
			return proxy(ctx, w, chatCompletionsURL(upstream), apiKey, req)
		} else {
			return err
		}
	}
	if err := proxy(ctx, w, chatCompletionsURL(upstream), apiKey, req); err == nil {
		return nil
	} else if shouldFallbackProtocol(err) {
		log.Printf("chat completions protocol failed model=%s error=%v; trying responses", req.Model, err)
		return proxyResponses(ctx, w, responsesURL(upstream), apiKey, req)
	} else {
		return err
	}
}

func shouldFallbackProtocol(err error) bool {
	var upstreamErr *upstreamError
	return errors.As(err, &upstreamErr) && upstreamErr.protocolMismatch()
}

func userFacingUpstreamError(err error) string {
	var upstreamErr *upstreamError
	if errors.As(err, &upstreamErr) {
		switch upstreamErr.statusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "模型服务当前繁忙或过载，请稍后重试"
		case http.StatusUnauthorized, http.StatusForbidden:
			return "模型服务鉴权失败，请检查 API Key"
		case http.StatusNotFound:
			return "模型服务接口不存在，请检查 API 地址"
		case http.StatusBadRequest:
			if upstreamErr.message != "" {
				return "模型请求参数错误：" + upstreamErr.message
			}
		}
	}
	return "模型服务调用失败，请检查 API 地址、Key 和模型名称"
}

func builtinModels() []map[string]string {
	return []map[string]string{
		{"id": "gpt-5.6-sol", "name": "GPT-5.6 Sol", "protocol": "responses"},
		{"id": "gpt-5.6-terra", "name": "GPT-5.6 Terra", "protocol": "responses"},
		{"id": "gpt-5.6-luna", "name": "GPT-5.6 Luna", "protocol": "responses"},
		{"id": "gpt-5.5", "name": "GPT-5.5", "protocol": "responses"},
		{"id": "gpt-4o", "name": "GPT-4o", "protocol": "chat_completions"},
		{"id": "gpt-4o-mini", "name": "GPT-4o mini", "protocol": "chat_completions"},
		{"id": "qwen-plus", "name": "通义千问 Plus", "protocol": "chat_completions"},
	}
}

func mergeModels(primary, fallback []map[string]string) []map[string]string {
	seen := map[string]bool{}
	out := make([]map[string]string, 0, len(primary)+len(fallback))
	for _, group := range [][]map[string]string{primary, fallback} {
		for _, model := range group {
			id := model["id"]
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, model)
		}
	}
	return out
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

func modelProtocol(model string) string {
	model = strings.ToLower(model)
	for _, item := range builtinModels() {
		if strings.ToLower(item["id"]) == model {
			if protocol := item["protocol"]; protocol != "" {
				return protocol
			}
		}
	}
	switch {
	case strings.Contains(model, "responses"):
		return "responses"
	case strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return "responses"
	case strings.HasPrefix(model, "gpt-5.6"):
		return "responses"
	case strings.HasPrefix(model, "gpt-5.5"):
		return "responses"
	default:
		return "chat_completions"
	}
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
		return newHTTPUpstreamError("responses", upstream, resp, b)
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
				protocol: "responses", url: upstream, statusCode: statusCode,
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
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return newHTTPUpstreamError("chat_completions", upstream, resp, b)
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
				if streamError, ok := v["error"]; ok {
					code, message, errorType := parseStreamError(streamError)
					statusCode := http.StatusBadGateway
					if code == "server_is_overloaded" || errorType == "service_unavailable" {
						statusCode = http.StatusServiceUnavailable
					}
					if code == "rate_limit_exceeded" || errorType == "rate_limit_error" {
						statusCode = http.StatusTooManyRequests
					}
					return &upstreamError{
						protocol: "chat_completions", url: upstream, statusCode: statusCode,
						status: http.StatusText(statusCode), message: message,
					}
				}
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

func parseStreamError(value any) (code, message, errorType string) {
	raw, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Sprint(value), ""
	}
	if v, ok := raw["code"].(string); ok {
		code = v
	}
	if v, ok := raw["message"].(string); ok {
		message = v
	}
	if v, ok := raw["type"].(string); ok {
		errorType = v
	}
	return code, message, errorType
}

func extractUpstreamMessage(body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if payload.Error.Message != "" {
			return payload.Error.Message
		}
		if payload.Message != "" {
			return payload.Message
		}
	}
	return strings.TrimSpace(string(body))
}

func newHTTPUpstreamError(protocol, url string, resp *http.Response, body []byte) error {
	raw := strings.TrimSpace(string(body))
	return &upstreamError{
		protocol:     protocol,
		url:          url,
		statusCode:   resp.StatusCode,
		status:       resp.Status,
		contentType:  resp.Header.Get("Content-Type"),
		message:      extractUpstreamMessage(body),
		responseBody: raw,
	}
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

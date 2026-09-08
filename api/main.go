package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type ChatRequest struct {
	ConversationID string    `json:"conversation_id"`
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
}
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type config struct{ agentURL string }

func main() {
	c := config{agentURL: env("AGENT_URL", "http://localhost:8090")}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "api"})
	})
	mux.HandleFunc("/api/models", c.models)
	mux.HandleFunc("/api/chat/stream", c.chatStream)
	server := &http.Server{Addr: env("API_ADDR", ":8080"), Handler: cors(mux), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("api listening on %s, agent=%s", server.Addr, c.agentURL)
	log.Fatal(server.ListenAndServe())
}

func (c config) models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	resp, err := http.Get(c.agentURL + "/v1/models")
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (c config) chatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var req ChatRequest
	if json.Unmarshal(body, &req) != nil || len(req.Messages) == 0 {
		http.Error(w, "invalid chat request", 400)
		return
	}
	forward, err := http.NewRequestWithContext(r.Context(), http.MethodPost, c.agentURL+"/v1/chat/stream", bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	forward.Header.Set("Content-Type", "application/json")
	forward.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{Timeout: 0}).Do(forward)
	if err != nil {
		http.Error(w, "agent unavailable: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(resp.StatusCode)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	buf := make([]byte, 4096)
	for {
		n, e := resp.Body.Read(buf)
		if n > 0 {
			if _, we := w.Write(buf[:n]); we != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if e == io.EOF {
			return
		}
		if e != nil {
			return
		}
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

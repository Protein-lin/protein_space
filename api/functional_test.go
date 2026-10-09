package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type providerTestDB struct {
	conversationDeleteDB
	values    []driver.NamedValue
	committed bool
	linked    bool
}

func (d *providerTestDB) Connect(context.Context) (driver.Conn, error) { return d, nil }
func (d *providerTestDB) Begin() (driver.Tx, error)                    { return d, nil }
func (d *providerTestDB) Commit() error                                { d.committed = true; return nil }
func (d *providerTestDB) Rollback() error                              { return nil }

type providerResult struct{}

func (providerResult) LastInsertId() (int64, error) { return 42, nil }
func (providerResult) RowsAffected() (int64, error) { return 1, nil }
func (d *providerTestDB) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.HasPrefix(query, "INSERT INTO provider_configs(") {
		d.values = append([]driver.NamedValue(nil), args...)
	}
	if strings.HasPrefix(query, "INSERT INTO user_provider_configs(") {
		d.linked = args[0].Value == int64(7) && args[1].Value == int64(42)
	}
	return providerResult{}, nil
}

func TestCreateProviderConfig(t *testing.T) {
	store := &providerTestDB{}
	db := sql.OpenDB(store)
	defer db.Close()
	a := &app{db: db, encryptionKey: []byte("12345678901234567890123456789012")}
	body := `{"name":"test provider","base_url":"https://provider.invalid/v1","api_key":"test-provider-secret","default_model":"test-model"}`
	r := httptest.NewRequest("POST", "/api/provider-configs", strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), userKey, uint64(7)))
	w := httptest.NewRecorder()
	a.providerConfigs(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if !store.committed || !store.linked || len(store.values) != 6 {
		t.Fatal("provider and ownership were not saved")
	}
	if store.values[0].Value != "test provider" || store.values[1].Value != "https://provider.invalid/v1" || store.values[4].Value != "test-model" {
		t.Fatal("provider settings were not preserved")
	}
	key, err := a.decrypt(store.values[2].Value.(string))
	if err != nil || key != "test-provider-secret" || strings.Contains(w.Body.String(), key) {
		t.Fatal("provider key was not safely stored")
	}
}

func TestLoggingPreservesStreaming(t *testing.T) {
	w := httptest.NewRecorder()
	a := &app{}
	a.logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("logging middleware removed http.Flusher; SSE is buffered")
		}
		io.WriteString(w, "data: {\"delta\":\"你好🌏\"}\n\n")
		f.Flush()
	})).ServeHTTP(w, httptest.NewRequest("GET", "/api/chat/stream", nil))
	if !w.Flushed || w.Code != 200 {
		t.Fatal("stream was not flushed")
	}
}

func TestAuthBoundary(t *testing.T) {
	for _, path := range []string{"/api/models", "/api/auth/me", "/api/api-keys", "/api/provider-configs", "/api/conversations", "/api/conversations/messages", "/api/chat/stream", "/api/auth/whitelist", "/api/admin/config"} {
		t.Run(path, func(t *testing.T) {
			a := &app{authRequired: true}
			w := httptest.NewRecorder()
			a.auth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthenticated request passed") })).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 401 {
				t.Fatalf("status = %d", w.Code)
			}
		})
	}
}

func TestGuestChatRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/api/models", true}, {"POST", "/api/chat/stream", true},
		{"GET", "/api/auth/me", true}, {"GET", "/api/conversations", true},
		{"DELETE", "/api/conversations?id=private", false},
		{"GET", "/api/conversations/messages?id=private", false},
		{"GET", "/api/api-keys", false}, {"POST", "/api/provider-configs", false},
		{"GET", "/api/admin/config", false}, {"POST", "/api/auth/whitelist", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			a := &app{authRequired: false, rootUserID: 7}
			w := httptest.NewRecorder()
			a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.allowed {
					t.Error("private route allowed")
				}
				if r.Context().Value(userKey) != nil || r.Context().Value(whitelistKey) != nil {
					t.Error("guest inherited an account or whitelist")
				}
				w.WriteHeader(204)
			})).ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			want := 401
			if tc.allowed {
				want = 204
			}
			if w.Code != want {
				t.Fatalf("status %d, want %d", w.Code, want)
			}
		})
	}
}

func TestGuestProfileAndHistory(t *testing.T) {
	a := &app{authRequired: false}
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{{"/api/auth/me", a.me}, {"/api/conversations", a.conversations}} {
		w := httptest.NewRecorder()
		a.auth(tc.handler).ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		var body map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["guest"] != true {
			t.Fatalf("invalid guest response: %s", w.Body.String())
		}
		if tc.path == "/api/conversations" && len(body["conversations"].([]any)) != 0 {
			t.Fatal("guest saw stored history")
		}
	}
}

func TestForwardedWhitelistBoundary(t *testing.T) {
	a := &app{authRequired: true, whitelistEnv: []string{"192.0.2.0/24"}, rootUserID: 7}
	for _, tc := range []struct {
		peer string
		want int
	}{{"198.51.100.1:1234", 401}, {"127.0.0.1:1234", 204}} {
		r := httptest.NewRequest("GET", "/api/conversations", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Real-IP", "192.0.2.5")
		w := httptest.NewRecorder()
		a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Context().Value(userKey) != uint64(7) {
				t.Error("missing whitelist identity")
			}
			w.WriteHeader(204)
		})).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("peer %s: status %d", tc.peer, w.Code)
		}
	}
}

func TestChatProxyAndValidation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			io.WriteString(w, `{"models":[{"id":"test-model"}],"default":"test-model"}`)
			return
		}
		var body ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.APIKey != "" || body.UpstreamURL != "" {
			t.Error("client provider credentials reached agent")
		}
		if contentText(body.Messages[0].Content) != "你好🌏" {
			t.Error("message was changed")
		}
		io.WriteString(w, "data: {\"delta\":\"回复🌏\"}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	a := &app{agentURL: upstream.URL}
	for _, tc := range []struct {
		name, method, body string
		status             int
	}{
		{"valid", "POST", `{"model":"test-model","messages":[{"role":"user","content":"你好🌏"}],"upstream_url":"https://untrusted.invalid","api_key":"untrusted"}`, 200},
		{"bad json", "POST", `{`, 400},
		{"empty messages", "POST", `{"model":"test-model","messages":[]}`, 400},
		{"unknown model", "POST", `{"model":"missing","messages":[{"role":"user","content":"hello"}]}`, 403},
		{"wrong method", "GET", ``, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			a.chatStream(w, httptest.NewRequest(tc.method, "/api/chat/stream", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == 200 && (!strings.Contains(w.Body.String(), "回复🌏") || !strings.Contains(w.Body.String(), "[DONE]")) {
				t.Fatal("missing streamed response")
			}
		})
	}
	allowed, err := a.modelAllowed(context.Background(), 0, false, "")
	if err != nil || allowed {
		t.Fatal("empty model accepted")
	}
}

func TestAttachmentStorageAndFragmentedSSE(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"文档"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]`)
	if contentText(content) != "文档\n[图片]" || contentForStorage(content) != string(content) {
		t.Fatal("attachment storage changed")
	}
	wire := "data: {\"delta\":\"你好🌏\"}\n\ndata: {\"content\":\"完成\"}\n\ndata: [DONE]\n\n"
	for split := 0; split <= len(wire); split++ {
		buffer := wire[:split]
		got := extractSSEText(&buffer)
		buffer += wire[split:]
		got += extractSSEText(&buffer)
		if got != "你好🌏完成" || buffer != "" {
			t.Fatalf("failed at byte %d", split)
		}
	}
}

package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log"
	"net"
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
	ConversationID   string    `json:"conversation_id"`
	Model            string    `json:"model"`
	ProviderConfigID uint64    `json:"provider_config_id"`
	Messages         []Message `json:"messages"`
	UpstreamURL      string    `json:"upstream_url,omitempty"`
	APIKey           string    `json:"api_key,omitempty"`
}
type app struct {
	db            *sql.DB
	agentURL      string
	authRequired  bool
	secureCookie  bool
	encryptionKey []byte
	whitelistEnv  []string
}
type ctxKey string

const userKey ctxKey = "user_id"

func main() {
	key := sha256.Sum256([]byte(env("APP_ENCRYPTION_KEY", "development-only-change-me")))
	a := &app{agentURL: env("AGENT_URL", "http://localhost:8090"), authRequired: envBool("AUTH_REQUIRED", false), secureCookie: envBool("SECURE_COOKIE", false), encryptionKey: key[:], whitelistEnv: splitCSV(os.Getenv("AUTH_WHITELIST_IPS"))}
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		var err error
		a.db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Fatal(err)
		}
		if err = waitDB(a.db); err != nil {
			log.Fatal(err)
		}
		log.Printf("mysql connected")
	} else {
		log.Printf("MYSQL_DSN is empty: auth persistence disabled")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/models", a.models)
	mux.HandleFunc("/api/auth/register", a.register)
	mux.HandleFunc("/api/auth/login", a.login)
	mux.HandleFunc("/api/auth/logout", a.logout)
	mux.HandleFunc("/api/auth/me", a.me)
	mux.HandleFunc("/api/api-keys", a.apiKeys)
	mux.HandleFunc("/api/auth/whitelist", a.whitelist)
	mux.HandleFunc("/api/provider-configs", a.providerConfigs)
	mux.HandleFunc("/api/conversations", a.conversations)
	mux.HandleFunc("/api/chat/stream", a.chatStream)
	s := &http.Server{Addr: env("API_ADDR", ":8080"), Handler: a.cors(a.auth(mux)), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("api listening on %s auth_required=%v", s.Addr, a.authRequired)
	log.Fatal(s.ListenAndServe())
}
func (a *app) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"status": "ok", "service": "api", "database": a.db != nil})
}
func (a *app) models(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	resp, err := http.Get(a.agentURL + "/v1/models")
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
func (a *app) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if a.db == nil {
		http.Error(w, "database is not configured", 503)
		return
	}
	var in struct{ Username, Email, Password string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Username) < 3 || len(in.Password) < 8 {
		http.Error(w, "username/password invalid", 400)
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	res, err := a.db.ExecContext(r.Context(), "INSERT INTO users(username,email,password_hash) VALUES(?,?,?)", in.Username, in.Email, string(hash))
	if err != nil {
		http.Error(w, "username or email already exists", 409)
		return
	}
	id, _ := res.LastInsertId()
	a.newSession(w, r, uint64(id))
	writeJSON(w, map[string]any{"id": id, "username": in.Username})
}
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if a.db == nil {
		http.Error(w, "database is not configured", 503)
		return
	}
	var in struct{ Username, Password string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	var id uint64
	var name, hash string
	err := a.db.QueryRowContext(r.Context(), "SELECT id,username,password_hash FROM users WHERE username=? AND status='active'", in.Username).Scan(&id, &name, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}
	a.newSession(w, r, id)
	writeJSON(w, map[string]any{"id": id, "username": name})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("waf_session"); err == nil && a.db != nil {
		a.db.ExecContext(r.Context(), "DELETE FROM user_sessions WHERE id=?", c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "waf_session", MaxAge: -1, Path: "/", HttpOnly: true, Secure: a.secureCookie})
	writeJSON(w, map[string]bool{"ok": true})
}
func (a *app) me(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value(userKey).(uint64)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var name string
	if a.db == nil {
		writeJSON(w, map[string]any{"id": id})
		return
	}
	if a.db.QueryRow("SELECT username FROM users WHERE id=?", id).Scan(&name) != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	writeJSON(w, map[string]any{"id": id, "username": name})
}
func (a *app) apiKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value(userKey).(uint64)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if a.db == nil {
		http.Error(w, "database is not configured", 503)
		return
	}
	if r.Method == "GET" {
		rows, err := a.db.Query("SELECT id,name,key_prefix,expires_at,created_at,last_used_at FROM user_api_keys WHERE user_id=? ORDER BY id DESC", id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var kid uint64
			var name, prefix string
			var exp, created, last sql.NullTime
			rows.Scan(&kid, &name, &prefix, &exp, &created, &last)
			out = append(out, map[string]any{"id": kid, "name": name, "key_prefix": prefix, "expires_at": exp, "created_at": created, "last_used_at": last})
		}
		writeJSON(w, map[string]any{"keys": out})
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	raw := make([]byte, 32)
	rand.Read(raw)
	token := "waf_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	res, err := a.db.Exec("INSERT INTO user_api_keys(user_id,name,key_prefix,key_hash,expires_at) VALUES(?,?,?,?,?)", id, in.Name, token[:12], hex.EncodeToString(sum[:]), in.ExpiresAt)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	kid, _ := res.LastInsertId()
	writeJSON(w, map[string]any{"id": kid, "name": in.Name, "key": token, "warning": "save this key now; it will not be shown again"})
}

func (a *app) whitelist(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userKey).(uint64)
	if !ok || a.db == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query("SELECT id,cidr,description,enabled,created_at FROM auth_ip_whitelist ORDER BY id DESC")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var id uint64
			var cidr, desc string
			var enabled bool
			var created time.Time
			rows.Scan(&id, &cidr, &desc, &enabled, &created)
			out = append(out, map[string]any{"id": id, "cidr": cidr, "description": desc, "enabled": enabled, "created_at": created})
		}
		writeJSON(w, map[string]any{"items": out})
	case http.MethodPost:
		var in struct{ CIDR, Description string }
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		if _, _, err := net.ParseCIDR(in.CIDR); net.ParseIP(in.CIDR) == nil && err != nil {
			http.Error(w, "cidr or ip invalid", 400)
			return
		}
		if net.ParseIP(in.CIDR) != nil {
			ip := net.ParseIP(in.CIDR)
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			in.CIDR = ip.String() + "/" + fmt.Sprint(bits)
		}
		_, err := a.db.Exec("INSERT INTO auth_ip_whitelist(cidr,description,created_by) VALUES(?,?,?)", in.CIDR, in.Description, user)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeJSON(w, map[string]any{"cidr": in.CIDR})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", 400)
			return
		}
		if _, err := a.db.Exec("DELETE FROM auth_ip_whitelist WHERE id=?", id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", 405)
	}
}
func (a *app) providerConfigs(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value(userKey).(uint64)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if a.db == nil {
		http.Error(w, "database is not configured", 503)
		return
	}
	if r.Method == "GET" {
		rows, err := a.db.Query(`SELECT pc.id,pc.name,pc.base_url,pc.api_key_hint,pc.default_model,pc.enabled,upc.role,upc.is_default FROM provider_configs pc JOIN user_provider_configs upc ON upc.provider_config_id=pc.id WHERE upc.user_id=? AND pc.enabled=1 ORDER BY pc.id DESC`, id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var cid uint64
			var name, base, hint, model, role string
			var enabled, def bool
			rows.Scan(&cid, &name, &base, &hint, &model, &enabled, &role, &def)
			out = append(out, map[string]any{"id": cid, "name": name, "base_url": base, "api_key_hint": hint, "default_model": model, "enabled": enabled, "role": role, "is_default": def})
		}
		writeJSON(w, map[string]any{"configs": out})
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Name, BaseURL, APIKey, DefaultModel string `json:"default_model"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Name == "" || in.BaseURL == "" || in.APIKey == "" {
		http.Error(w, "name, base_url, api_key required", 400)
		return
	}
	ciphertext, err := a.encrypt(in.APIKey)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	hint := in.APIKey
	if len(hint) > 12 {
		hint = "..." + hint[len(hint)-8:]
	}
	tx, err := a.db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := tx.Exec("INSERT INTO provider_configs(name,base_url,api_key_ciphertext,api_key_hint,default_model,created_by) VALUES(?,?,?,?,?,?)", in.Name, in.BaseURL, ciphertext, hint, in.DefaultModel, id)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}
	cid, _ := res.LastInsertId()
	if _, err = tx.Exec("INSERT INTO user_provider_configs(user_id,provider_config_id,role,is_default) VALUES(?,?,?,1)", id, cid, "owner"); err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), 500)
		return
	}
	tx.Commit()
	writeJSON(w, map[string]any{"id": cid, "name": in.Name, "api_key_hint": hint})
}
func (a *app) conversations(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value(userKey).(uint64)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if a.db == nil {
		writeJSON(w, map[string]any{"conversations": []any{}})
		return
	}
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	rows, err := a.db.Query("SELECT id,title,model,updated_at FROM conversations WHERE user_id=? ORDER BY updated_at DESC LIMIT 100", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var cid, title, model string
		var updated time.Time
		rows.Scan(&cid, &title, &model, &updated)
		out = append(out, map[string]any{"id": cid, "title": title, "model": model, "updated_at": updated})
	}
	writeJSON(w, map[string]any{"conversations": out})
}
func (a *app) chatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req ChatRequest
	if json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req) != nil || len(req.Messages) == 0 {
		http.Error(w, "invalid chat request", 400)
		return
	}
	id, _ := r.Context().Value(userKey).(uint64)
	if a.db != nil && id > 0 && req.ConversationID != "" {
		a.saveMessages(r.Context(), id, req)
	}
	// Never trust upstream_url/api_key supplied by a browser. Only a server-side
	// provider_config_id can populate these fields after an ownership check.
	req.UpstreamURL = ""
	req.APIKey = ""
	if a.db != nil && id > 0 && req.ProviderConfigID > 0 {
		var base, ciphertext string
		err := a.db.QueryRow("SELECT pc.base_url,pc.api_key_ciphertext FROM provider_configs pc JOIN user_provider_configs upc ON upc.provider_config_id=pc.id WHERE pc.id=? AND upc.user_id=? AND pc.enabled=1", req.ProviderConfigID, id).Scan(&base, &ciphertext)
		if err != nil {
			http.Error(w, "provider config unavailable", 403)
			return
		}
		key, err := a.decrypt(ciphertext)
		if err != nil {
			http.Error(w, "provider config unavailable", 500)
			return
		}
		req.UpstreamURL = base
		req.APIKey = key
	}
	body, _ := json.Marshal(req)
	forward, _ := http.NewRequestWithContext(r.Context(), "POST", a.agentURL+"/v1/chat/stream", bytes.NewReader(body))
	forward.Header.Set("Content-Type", "application/json")
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
			if _, x := w.Write(buf[:n]); x != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if e == io.EOF || e != nil {
			return
		}
	}
}
func (a *app) saveMessages(ctx context.Context, user uint64, req ChatRequest) {
	a.db.ExecContext(ctx, "INSERT IGNORE INTO conversations(id,user_id,title,model) VALUES(?,?,?,?)", req.ConversationID, user, req.Messages[0].Content, req.Model)
	for i, m := range req.Messages {
		a.db.ExecContext(ctx, "INSERT IGNORE INTO messages(conversation_id,role,content,sequence_no) VALUES(?,?,?,?)", req.ConversationID, m.Role, m.Content, i+1)
	}
	a.db.ExecContext(ctx, "UPDATE conversations SET model=?,updated_at=CURRENT_TIMESTAMP(3) WHERE id=? AND user_id=?", req.Model, req.ConversationID, user)
}
func (a *app) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.ipWhitelisted(r) || r.URL.Path == "/api/health" || r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/register" || r.URL.Path == "/api/models" || !a.authRequired {
			next.ServeHTTP(w, r)
			return
		}
		if id, ok := a.authenticate(r); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, id)))
			return
		}
		http.Error(w, "unauthorized", 401)
	})
}

func (a *app) ipWhitelisted(r *http.Request) bool {
	ip := requestIP(r)
	if ip == nil {
		return false
	}
	for _, entry := range a.whitelistEnv {
		if _, netw, err := net.ParseCIDR(entry); err == nil && netw.Contains(ip) {
			return true
		}
		if e := net.ParseIP(entry); e != nil && e.Equal(ip) {
			return true
		}
	}
	if a.db == nil {
		return false
	}
	rows, err := a.db.Query("SELECT cidr FROM auth_ip_whitelist WHERE enabled=1")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cidr string
		rows.Scan(&cidr)
		if _, netw, err := net.ParseCIDR(cidr); err == nil && netw.Contains(ip) {
			return true
		}
	}
	return false
}

func requestIP(r *http.Request) net.IP {
	// Nginx sets X-Real-IP to the address it observed. X-Forwarded-For is a
	// fallback for TLS/server blocks that are managed separately.
	values := []string{r.Header.Get("X-Real-IP")}
	if values[0] == "" {
		values = strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		}
		if ip := net.ParseIP(value); ip != nil {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.TrimSpace(r.RemoteAddr))
}
func (a *app) authenticate(r *http.Request) (uint64, bool) {
	if a.db == nil {
		return 0, false
	}
	if raw := r.Header.Get("X-API-Key"); raw != "" {
		sum := sha256.Sum256([]byte(raw))
		var id uint64
		var exp sql.NullTime
		if a.db.QueryRow("SELECT user_id,expires_at FROM user_api_keys WHERE key_hash=?", hex.EncodeToString(sum[:])).Scan(&id, &exp) == nil && (!exp.Valid || exp.Time.After(time.Now())) {
			a.db.Exec("UPDATE user_api_keys SET last_used_at=CURRENT_TIMESTAMP(3) WHERE key_hash=?", hex.EncodeToString(sum[:]))
			return id, true
		}
	}
	if c, e := r.Cookie("waf_session"); e == nil {
		var id uint64
		var exp time.Time
		if a.db.QueryRow("SELECT user_id,expires_at FROM user_sessions WHERE id=?", c.Value).Scan(&id, &exp) == nil && exp.After(time.Now()) {
			return id, true
		}
	}
	return 0, false
}
func (a *app) newSession(w http.ResponseWriter, r *http.Request, id uint64) {
	raw := make([]byte, 32)
	rand.Read(raw)
	token := hex.EncodeToString(raw)
	if a.db != nil {
		a.db.ExecContext(r.Context(), "INSERT INTO user_sessions(id,user_id,expires_at) VALUES(?,?,?)", token, id, time.Now().Add(7*24*time.Hour))
	}
	http.SetCookie(w, &http.Cookie{Name: "waf_session", Value: token, Path: "/", HttpOnly: true, Secure: a.secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600})
}
func (a *app) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func waitDB(db *sql.DB) error {
	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("mysql unavailable after 30 retries")
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func (a *app) encrypt(value string) (string, error) {
	block, err := aes.NewCipher(a.encryptionKey)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(append(nonce, g.Seal(nil, nonce, []byte(value), nil)...)), nil
}
func (a *app) decrypt(value string) (string, error) {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(a.encryptionKey)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	n := g.NonceSize()
	if len(raw) < n {
		return "", errors.New("invalid ciphertext")
	}
	out, err := g.Open(nil, raw[:n], raw[n:], nil)
	return string(out), err
}
func env(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}
func splitCSV(v string) []string {
	out := []string{}
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
func envBool(k string, f bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return f
	}
	return v == "1" || strings.EqualFold(v, "true")
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
)

func (a *app) wafReviews(w http.ResponseWriter, r *http.Request) {
	id, _ := r.Context().Value(userKey).(uint64)
	if id == 0 {
		http.Error(w, "请登录后保存修订经验", 401)
		return
	}
	if a.db == nil {
		http.Error(w, "修订经验需要配置数据库", 503)
		return
	}
	if r.Method != "GET" && r.Method != "POST" && r.Method != "DELETE" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `CREATE TABLE IF NOT EXISTS waf_review_examples (user_id BIGINT UNSIGNED NOT NULL, example_key VARCHAR(64) NOT NULL, content MEDIUMTEXT NOT NULL, updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, PRIMARY KEY(user_id,example_key))`); err != nil {
		http.Error(w, "修订表初始化失败", 500)
		return
	}
	if r.Method == "GET" {
		rows, err := a.db.QueryContext(r.Context(), "SELECT example_key,content FROM waf_review_examples WHERE user_id=? ORDER BY updated_at DESC LIMIT 30", id)
		if err != nil {
			http.Error(w, "读取失败", 500)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var key, content string
			if rows.Scan(&key, &content) != nil {
				http.Error(w, "读取失败", 500)
				return
			}
			out = append(out, map[string]any{"id": key, "example": json.RawMessage(content)})
		}
		writeJSON(w, out)
		return
	}
	if r.Method == "DELETE" {
		_, err := a.db.ExecContext(r.Context(), "DELETE FROM waf_review_examples WHERE user_id=? AND example_key=?", id, r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, "删除失败", 500)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	var input struct {
		ID      string          `json:"id"`
		Example json.RawMessage `json:"example"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&input) != nil || len(input.ID) == 0 || len(input.ID) > 64 || !json.Valid(input.Example) {
		http.Error(w, "修订数据无效", 400)
		return
	}
	_, err := a.db.ExecContext(r.Context(), "INSERT INTO waf_review_examples(user_id,example_key,content) VALUES(?,?,?) ON DUPLICATE KEY UPDATE content=VALUES(content)", id, input.ID, string(input.Example))
	if err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
func (a *app) wafLearning(ctx context.Context) string {
	id, _ := ctx.Value(userKey).(uint64)
	if id == 0 || a.db == nil {
		return ""
	}
	rows, err := a.db.QueryContext(ctx, "SELECT content FROM waf_review_examples WHERE user_id=? ORDER BY updated_at DESC LIMIT 5", id)
	if err != nil {
		return ""
	}
	defer rows.Close()
	examples := []json.RawMessage{}
	size := 0
	for rows.Next() {
		var s string
		if rows.Scan(&s) != nil {
			break
		}
		size += len(s)
		if size > 64000 {
			break
		}
		examples = append(examples, json.RawMessage(s))
	}
	return mustJSON(examples)
}

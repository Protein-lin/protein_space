package main

import (
	_ "embed"
	"net/http"
)

//go:embed waf_score_types.json
var wafScoreTypes string

func (a *app) wafCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(wafScoreTypes))
}

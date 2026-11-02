package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWafRulesRequiresRealProvider(t *testing.T) {
	s := &Service{defaultModel: "test"}
	w := httptest.NewRecorder()
	s.chat(w, httptest.NewRequest("POST", "/v1/chat/stream", strings.NewReader(`{"task":"waf_rules","messages":[{"role":"user","content":"log"}]}`)))
	if w.Code != 503 {
		t.Fatalf("must not return demo rules: %d", w.Code)
	}
}

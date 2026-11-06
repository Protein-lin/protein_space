package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWafCatalogThresholds(t *testing.T) {
	var rows []struct {
		Name    string
		General int `json:"general_score"`
		Strict  int `json:"strict_score"`
	}
	if err := json.Unmarshal([]byte(wafScoreTypes), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 42 {
		t.Fatal(len(rows))
	}
	for _, r := range rows {
		if r.Name == "$SQL" && (r.General != 20 || r.Strict != 12) {
			t.Fatal(r)
		}
	}
}
func TestWafReviewsRejectGuest(t *testing.T) {
	a := &app{}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		w := httptest.NewRecorder()
		a.wafReviews(w, httptest.NewRequest(method, "/api/waf-rules/reviews", nil))
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
}

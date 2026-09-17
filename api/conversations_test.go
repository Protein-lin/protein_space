package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// This driver checks the ownership boundary without requiring a running MySQL.
// Message cascading is provided by fk_messages_conversation in the schema.
type conversationDeleteDB struct {
	owners map[string]int64
	fail   bool
	calls  int
}

func (d *conversationDeleteDB) Connect(context.Context) (driver.Conn, error) { return d, nil }
func (d *conversationDeleteDB) Driver() driver.Driver                        { return d }
func (d *conversationDeleteDB) Open(string) (driver.Conn, error)             { return d, nil }
func (d *conversationDeleteDB) Close() error                                 { return nil }
func (d *conversationDeleteDB) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (d *conversationDeleteDB) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepared statement")
}
func (d *conversationDeleteDB) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	d.calls++
	if query != "DELETE FROM conversations WHERE id=? AND user_id=?" || len(args) != 2 {
		return nil, errors.New("delete must be scoped to the authenticated user")
	}
	if d.fail {
		return nil, errors.New("database unavailable")
	}
	id, idOK := args[0].Value.(string)
	user, userOK := args[1].Value.(int64)
	if !idOK || !userOK {
		return nil, errors.New("invalid delete arguments")
	}
	if owner, exists := d.owners[id]; exists && owner == user {
		delete(d.owners, id)
		return driver.RowsAffected(1), nil
	}
	return driver.RowsAffected(0), nil
}

func TestDeleteConversation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		id     string
		user   uint64
		fail   bool
		status int
	}{
		{"own conversation", "mine", 7, false, http.StatusNoContent},
		{"another user's conversation", "theirs", 7, false, http.StatusNotFound},
		{"missing conversation", "missing", 7, false, http.StatusNotFound},
		{"missing id", "", 7, false, http.StatusBadRequest},
		{"unauthenticated", "mine", 0, false, http.StatusUnauthorized},
		{"database failure", "mine", 7, true, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &conversationDeleteDB{owners: map[string]int64{"mine": 7, "theirs": 8}, fail: tc.fail}
			db := sql.OpenDB(store)
			defer db.Close()
			a := &app{db: db}
			req := httptest.NewRequest(http.MethodDelete, "/api/conversations?id="+tc.id, nil)
			if tc.user != 0 {
				req = req.WithContext(context.WithValue(req.Context(), userKey, tc.user))
			}
			w := httptest.NewRecorder()
			a.conversations(w, req)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if _, exists := store.owners["theirs"]; !exists {
				t.Fatal("another user's conversation was deleted")
			}
			_, mineExists := store.owners["mine"]
			if mineExists != (tc.status != http.StatusNoContent) {
				t.Fatal("unexpected state of own conversation")
			}
			if (tc.user == 0 || tc.id == "") && store.calls != 0 {
				t.Fatal("invalid request reached the database")
			}
		})
	}
}

func TestConversationMethodsWithoutDatabase(t *testing.T) {
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodGet, http.StatusOK},
		{http.MethodDelete, http.StatusServiceUnavailable},
		{http.MethodPost, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/conversations?id=mine", nil)
			req = req.WithContext(context.WithValue(req.Context(), userKey, uint64(7)))
			w := httptest.NewRecorder()
			(&app{}).conversations(w, req)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
}

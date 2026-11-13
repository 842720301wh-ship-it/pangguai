package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"panguai/internal/accounts"
)

func TestLocalAccessAndCredentialSeparation(t *testing.T) {
	store, err := accounts.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(accounts.Record{Profile: accounts.Profile{ID: "test-account", Name: "测试用户"}, Token: "never-return-this-token"}); err != nil {
		t.Fatal(err)
	}
	handler := New(store, nil, "")
	t.Run("public account list excludes token", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://127.0.0.1:18080/api/accounts", nil)
		handler.ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), "never-return-this-token") || strings.Contains(w.Body.String(), `"token"`) {
			t.Fatal("credential exposed", w.Code)
		}
	})
	for _, test := range []struct {
		name, host, origin, marker string
		want                       int
	}{
		{"DNS rebinding", "attacker.example", "", "web", 403},
		{"foreign browser origin", "127.0.0.1:18080", "https://attacker.example", "web", 403},
		{"cross-site submission", "127.0.0.1:18080", "", "", 403},
		{"unconfigured real login", "127.0.0.1:18080", "", "web", 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18080/api/login/sms/send", strings.NewReader(`{"phone":"13800000000"}`))
			r.Host = test.host
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", test.origin)
			r.Header.Set("X-Panguai-Client", test.marker)
			handler.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("got %d, want %d", w.Code, test.want)
			}
		})
	}
}

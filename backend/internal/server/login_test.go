package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"panguai/internal/accounts"
	"panguai/internal/platform"
)

type fakeGateway struct {
	mu             sync.Mutex
	sent, logins   int
	checks         int
	checkPhone     string
	failFirstCheck bool
	entered        chan struct{}
	release        chan struct{}
}

func (f *fakeGateway) SendSMS(context.Context, string) error {
	f.mu.Lock()
	f.sent++
	f.mu.Unlock()
	return nil
}
func (f *fakeGateway) Login(ctx context.Context, phone, code string) (platform.LoginResult, error) {
	f.mu.Lock()
	f.logins++
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
		select {
		case <-f.release:
		case <-ctx.Done():
			return platform.LoginResult{}, ctx.Err()
		}
	}
	if code != "123456" {
		return platform.LoginResult{}, &platform.Error{Code: "platform_invalid_code", Message: "验证码错误"}
	}
	return platform.LoginResult{Token: "test-only-account-token", User: platform.User{Phone: phone, Nickname: "测试用户"}}, nil
}
func (f *fakeGateway) Check(context.Context, string) (platform.LoginResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	if f.failFirstCheck && f.checks == 1 {
		return platform.LoginResult{}, &platform.Error{Code: "platform_1", Message: "参数不正确", UpstreamPath: "/user/userHome"}
	}
	phone := f.checkPhone
	if phone == "" {
		phone = "13800000000"
	}
	return platform.LoginResult{Token: "test-only-account-token", User: platform.User{Phone: phone, Nickname: "测试用户"}}, nil
}

func apiRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw string
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		raw = string(encoded)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:18080"+path, strings.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Panguai-Client", "web")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func testHandler(t *testing.T, gateway Gateway) (http.Handler, *accounts.Store) {
	t.Helper()
	store, err := accounts.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(store, gateway, ""), store
}
func beginSession(t *testing.T, h http.Handler) string {
	t.Helper()
	w := apiRequest(t, h, "POST", "/api/login/sms/send", map[string]string{"phone": "13800000000"})
	if w.Code != 200 {
		t.Fatalf("send status %d: %s", w.Code, w.Body)
	}
	var result struct {
		ID string `json:"session_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.ID) != 48 {
		t.Fatal("session creation failed", err)
	}
	return result.ID
}

func TestLoginFlowAndRetryDoNotResendOrReconfirm(t *testing.T) {
	gateway := &fakeGateway{}
	h, store := testHandler(t, gateway)
	id := beginSession(t, h)
	for _, expected := range []string{"pending"} {
		w := apiRequest(t, h, "GET", "/api/login/sessions/"+id, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), expected) || strings.Contains(w.Body.String(), "13800000000") {
			t.Fatal("session state incorrect or phone exposed")
		}
	}
	if w := apiRequest(t, h, "POST", "/api/login/sms/send", map[string]string{"phone": "13800000000"}); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("SMS cooldown not enforced")
	}
	var accountID string
	for i := 0; i < 2; i++ {
		w := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "123456"})
		if w.Code != 200 {
			t.Fatalf("confirm failed %d: %s", w.Code, w.Body)
		}
		var result struct {
			Status  string           `json:"status"`
			Account accounts.Profile `json:"account"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "confirmed" || result.Account.UpdatedAt.IsZero() {
			t.Fatal("confirmed login has no saved account metadata")
		}
		if strings.Contains(w.Body.String(), "test-only-account-token") {
			t.Fatal("token exposed")
		}
		accountID = result.Account.ID
	}
	if gateway.sent != 1 || gateway.logins != 1 {
		t.Fatal("retry repeated platform operation", gateway.sent, gateway.logins)
	}
	if w := apiRequest(t, h, "GET", "/api/login/sessions/"+id, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "confirmed") {
		t.Fatal("confirmed session cannot be recovered")
	}
	if w := apiRequest(t, h, "POST", "/api/accounts/"+accountID+"/check", map[string]string{}); w.Code != 200 {
		t.Fatal("saved account check failed")
	}
	if w := apiRequest(t, h, "DELETE", "/api/accounts/"+accountID, nil); w.Code != 204 {
		t.Fatal("local removal failed")
	}
	if _, err := store.Get(accountID); err == nil {
		t.Fatal("credential remained after removal")
	}
}

func TestConcurrentConfirmationMakesOnePlatformCall(t *testing.T) {
	gateway := &fakeGateway{entered: make(chan struct{}, 1), release: make(chan struct{})}
	h, _ := testHandler(t, gateway)
	id := beginSession(t, h)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "123456"})
	}()
	select {
	case <-gateway.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first confirmation did not start")
	}
	second := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "123456"})
	if second.Code != 409 {
		t.Fatal("simultaneous confirmation was not rejected")
	}
	state := apiRequest(t, h, "GET", "/api/login/sessions/"+id, nil)
	if !strings.Contains(state.Body.String(), "confirming") {
		t.Fatal("in-flight state was lost")
	}
	close(gateway.release)
	select {
	case first := <-done:
		if first.Code != 200 {
			t.Fatal("first confirmation failed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("confirmation did not finish")
	}
	if gateway.logins != 1 {
		t.Fatal("duplicate platform confirmation")
	}
}

func TestVerificationAttemptsAreBounded(t *testing.T) {
	gateway := &fakeGateway{}
	h, store := testHandler(t, gateway)
	id := beginSession(t, h)
	for i := 0; i < 5; i++ {
		w := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "000000"})
		if w.Code != 502 {
			t.Fatal("wrong code was accepted")
		}
	}
	w := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "123456"})
	if w.Code != 400 || gateway.logins != 5 {
		t.Fatal("verification limit was bypassed")
	}
	list, err := store.List()
	if err != nil || len(list) != 0 {
		t.Fatal("failed login saved credentials")
	}
}

func TestReadbackFailureCanResumeWithoutReusingSMSCode(t *testing.T) {
	gateway := &fakeGateway{failFirstCheck: true}
	h, store := testHandler(t, gateway)
	id := beginSession(t, h)
	first := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "123456"})
	if first.Code != 502 || !strings.Contains(first.Body.String(), "/user/userHome") {
		t.Fatal("readback failure lost its source")
	}
	list, err := store.List()
	if err != nil || len(list) != 0 {
		t.Fatal("unverified account was saved")
	}
	state := apiRequest(t, h, "GET", "/api/login/sessions/"+id, nil)
	if !strings.Contains(state.Body.String(), "verifying") || strings.Contains(state.Body.String(), "test-only-account-token") {
		t.Fatal("pending readback state missing or token exposed")
	}
	resumed := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id})
	if resumed.Code != 200 || gateway.logins != 1 || gateway.checks != 2 {
		t.Fatal("readback retry consumed the SMS code again")
	}
}

func TestForeignReadbackCannotSaveAccount(t *testing.T) {
	gateway := &fakeGateway{checkPhone: "13900000000"}
	h, store := testHandler(t, gateway)
	id := beginSession(t, h)
	w := apiRequest(t, h, "POST", "/api/login/sms/confirm", map[string]string{"session_id": id, "code": "123456"})
	if w.Code != 502 || !strings.Contains(w.Body.String(), "identity_mismatch") {
		t.Fatal("foreign account identity accepted")
	}
	list, err := store.List()
	if err != nil || len(list) != 0 {
		t.Fatal("foreign account credentials persisted")
	}
}

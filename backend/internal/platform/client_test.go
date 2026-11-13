package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSignatureVectors(t *testing.T) {
	for _, test := range []struct{ token, path, want string }{
		{"", "/user/reg", "46e5975275075eafcc298c596a1032f54eb5f7ca0d210b2aed6fab76de4cd274"},
		{"test-token", "/user/checkLogin", "eff2590c804e4da15de7ae520d7f37382a750140d87dac3bc6ec824905672ee6"},
	} {
		if got := Signature("fixture-secret", "1.145.0", "1794470400123", test.token, test.path); got != test.want {
			t.Fatalf("signature mismatch: %s", got)
		}
	}
}

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})
	return &Client{config: Config{AppSecret: "fixture-secret", Version: "1.145.0", DeviceID: "local-fixture", PhoneBrand: "test"}, origin: Origin, http: &http.Client{Transport: transport}}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTP200IsNotBusinessSuccess(t *testing.T) {
	for _, body := range []string{`{"code":1001,"msg":"请登录","data":null}`, `{"msg":"missing code","data":true}`, `{"code":0,"data":null}`} {
		t.Run(body, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
			var result bool
			if err := client.post(context.Background(), "/user/smsCode", "", nil, &result); err == nil {
				t.Fatal("unconfirmed response accepted")
			}
		})
	}
}

func TestSMSRejectionIsNotRetried(t *testing.T) {
	requests := 0
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/common/sms/sendCode" {
			t.Error("unexpected SMS endpoint")
		}
		_, _ = w.Write([]byte(`{"code":1001,"msg":"需要安全验证"}`))
	})
	err := client.SendSMS(context.Background(), "13800000000")
	typed, ok := err.(*Error)
	if !ok || typed.Code != "platform_1001" || requests != 1 {
		t.Fatal("platform rejection was ignored or automatically retried", err, requests)
	}
}

func TestSMSMatchesPhoneLoginActivity(t *testing.T) {
	requests := 0
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/common/sms/sendCode" {
			t.Fatal("unexpected preflight or SMS endpoint")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("phone") != "13800000000" || r.Form.Get("template") != "reg" || len(r.Form) != 2 {
			t.Fatal("SMS form differs from App login flow")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":""}`))
	})
	if err := client.SendSMS(context.Background(), "13800000000"); err != nil || requests != 1 {
		t.Fatal("valid SMS acknowledgement rejected", err)
	}
}

func TestSMSLoginChecksIdentityAndSignedRequests(t *testing.T) {
	paths := []string{}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Error("unexpected method")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		token := r.Header.Get("Authorization")
		want := Signature("fixture-secret", "1.145.0", r.Header.Get("timestamp"), token, r.URL.Path)
		if r.Header.Get("sign") != want {
			t.Error("signed path/header mismatch")
		}
		if r.URL.Path == "/user/reg" {
			if r.Form.Get("phone") != "13800000000" || r.Form.Get("verify") != "123456" || r.Form.Get("channel") != "android_app" {
				t.Error("login form changed")
			}
		} else if r.URL.Path == "/user/info" {
			if token != "test-sensitive-token" || r.Form.Get("token") != token {
				t.Error("token header/form mismatch")
			}
		} else {
			t.Error("unexpected endpoint")
		}
		if r.URL.Path == "/user/info" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"id": "fixture-user", "phone": "13800000000", "userName": "测试用户"}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"token": "test-sensitive-token"}})
		}
	})
	result, err := client.Login(context.Background(), "13800000000", "123456")
	if err == nil {
		result, err = client.Check(context.Background(), result.Token)
	}
	if err != nil || result.User.Nickname != "测试用户" || len(paths) != 2 {
		t.Fatal("login/readback failed", err, paths)
	}
}

func TestMissingTokenCannotCompleteAuthentication(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"userInfo":{"phone":"13800000000"}}}`))
	})
	if _, err := client.Login(context.Background(), "13800000000", "123456"); err == nil {
		t.Fatal("authentication accepted without a token")
	}
}

func TestErrorMessageDoesNotExposeLoginMaterial(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1001,"msg":"13800000000 123456 fixture-token\nfailed"}`))
	})
	err := client.post(context.Background(), "/user/reg", "fixture-token", url.Values{"phone": {"13800000000"}, "verify": {"123456"}}, nil)
	if err == nil {
		t.Fatal("expected platform error")
	}
	for _, sensitive := range []string{"13800000000", "123456", "fixture-token", "\n"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatal("login material leaked in error")
		}
	}
}

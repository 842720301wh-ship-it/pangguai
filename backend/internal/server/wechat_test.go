package server

import (
	"context"
	"panguai/internal/platform"
	"strings"
	"testing"
)

type fakeWechatGateway struct {
	fakeGateway
	reject bool
}

func (f *fakeWechatGateway) WechatLogin(context.Context, string, string) (platform.LoginResult, error) {
	if f.reject {
		return platform.LoginResult{}, &platform.Error{Code: "wechat_authorization_invalid", Message: "授权失效"}
	}
	return platform.LoginResult{Token: "test-only-account-token"}, nil
}
func TestWechatConfirmedOnlyAfterProfileReadback(t *testing.T) {
	gateway := &fakeWechatGateway{}
	h, store := testHandler(t, gateway)
	reply := apiRequest(t, h, "POST", "/api/login/wechat/confirm", map[string]string{"access_token": "fixture-access", "openid": "fixture-open"})
	if reply.Code != 200 || gateway.checks != 1 {
		t.Fatal("Wechat account was not verified", reply.Code)
	}
	list, err := store.List()
	if err != nil || len(list) != 1 || list[0].Phone != "138****0000" {
		t.Fatal("verified account not saved")
	}
	if strings.Contains(reply.Body.String(), "fixture-access") || strings.Contains(reply.Body.String(), "test-only-account-token") {
		t.Fatal("public response exposed a credential")
	}
}
func TestWechatRejectsBareUnionIDAndFailedAuthorization(t *testing.T) {
	gateway := &fakeWechatGateway{reject: true}
	h, store := testHandler(t, gateway)
	for _, body := range []map[string]string{{"unionId": "arbitrary"}, {"access_token": "fixture-access", "openid": "fixture-open"}} {
		reply := apiRequest(t, h, "POST", "/api/login/wechat/confirm", body)
		if reply.Code == 200 {
			t.Fatal("unverified identity accepted")
		}
	}
	list, err := store.List()
	if err != nil || len(list) != 0 || gateway.checks != 0 || gateway.sent != 0 {
		t.Fatal("failed WeChat authorization had side effects")
	}
}

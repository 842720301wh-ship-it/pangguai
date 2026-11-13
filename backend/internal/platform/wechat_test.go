package platform

import (
	"context"
	"net/http"
	"testing"
)

func TestWechatRequiresVerifiedOAuthBeforePlatformLogin(t *testing.T) {
	for _, test := range []struct {
		name, body string
		accepted   bool
	}{
		{"valid", `{"openid":"fixture-openid","unionid":"fixture-union"}`, true},
		{"expired", `{"errcode":40001,"errmsg":"sensitive credential"}`, false},
		{"wrong identity", `{"openid":"another","unionid":"fixture-union"}`, false},
		{"missing unionid", `{"openid":"fixture-openid"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					if r.URL.Host != "api.weixin.qq.com" || r.URL.Path != "/sns/userinfo" || r.URL.Query().Get("access_token") != "fixture-access" || r.URL.Query().Get("openid") != "fixture-openid" {
						t.Fatal("OAuth verification changed")
					}
					_, _ = w.Write([]byte(test.body))
					return
				}
				if r.URL.Path != "/wechat/unionId/login" || r.Method != "POST" {
					t.Fatal("wrong platform endpoint")
				}
				_ = r.ParseForm()
				if r.Form.Get("unionId") != "fixture-union" || len(r.Form) != 1 {
					t.Fatal("unexpected platform form")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"token":"fixture-platform-token"}}`))
			})
			result, err := c.WechatLogin(context.Background(), "fixture-access", "fixture-openid")
			if test.accepted {
				if err != nil || result.Token != "fixture-platform-token" || calls != 2 {
					t.Fatal("verified login rejected", err)
				}
			} else if err == nil || calls != 1 {
				t.Fatal("unverified identity reached platform")
			}
		})
	}
}

func TestProfileReadbackUsesDirectUserInfoBean(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/info" || r.Header.Get("Authorization") != "fixture-token" {
			t.Fatal("incorrect profile readback")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":"platform-user-1","userName":"测试昵称","phone":"138****0000"}}`))
	})
	result, err := c.Check(context.Background(), "fixture-token")
	if err != nil || result.User.ID != "platform-user-1" || result.User.Phone != "138****0000" || result.User.Nickname != "测试昵称" || result.Token != "fixture-token" {
		t.Fatal("UserInfoBean shape rejected", err)
	}
}

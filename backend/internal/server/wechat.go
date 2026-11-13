package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"panguai/internal/accounts"
	"panguai/internal/platform"
)

type wechatGateway interface {
	WechatLogin(context.Context, string, string) (platform.LoginResult, error)
}

func (s *Server) wechatLogin(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	gateway, ok := s.platform.(wechatGateway)
	if !ok {
		writeError(w, 503, "wechat_pending", "微信授权服务尚未配置")
		return
	}
	var input struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
	}
	if !decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.AccessToken) == "" || strings.TrimSpace(input.OpenID) == "" {
		writeError(w, 400, "wechat_credential_invalid", "请提供本人本次微信授权凭据")
		return
	}
	s.mu.Lock()
	if s.inflight >= 3 {
		s.mu.Unlock()
		writeError(w, 429, "login_pending", "正在处理登录，请稍后重试")
		return
	}
	s.inflight++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.inflight--; s.mu.Unlock() }()
	authenticated, err := gateway.WechatLogin(r.Context(), input.AccessToken, input.OpenID)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if authenticated.Token == "" || authenticated.ForcedBind {
		writeError(w, 409, "wechat_binding_required", "请在官方 App 完成账号绑定或验证")
		return
	}
	result, err := s.platform.Check(r.Context(), authenticated.Token)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if result.User.ID == "" && !phonePattern.MatchString(result.User.Phone) {
		writeError(w, 409, "identity_incomplete", "平台身份未确认，未保存账号")
		return
	}
	identity := "wechat/" + result.User.ID
	phone := ""
	if phonePattern.MatchString(result.User.Phone) {
		identity = result.User.Phone
		phone = result.User.Phone[:3] + "****" + result.User.Phone[7:]
	}
	id := sha256.Sum256([]byte("panguai/account/" + identity))
	name := result.User.Nickname
	if name == "" {
		name = "胖乖用户"
	}
	profile := accounts.Profile{ID: "local_" + hex.EncodeToString(id[:12]), Name: name, Phone: phone, UpdatedAt: time.Now().UTC()}
	if err := s.store.Save(accounts.Record{Profile: profile, Token: result.Token, PlatformID: result.User.ID}); err != nil {
		writeError(w, 500, "storage_error", "身份已验证，但本机账号保存失败")
		return
	}
	jsonResponse(w, 200, map[string]any{"status": "confirmed", "account": profile})
}

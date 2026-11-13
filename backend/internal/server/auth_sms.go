package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"regexp"
	"strings"
	"time"
)

var phonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
var codePattern = regexp.MustCompile(`^[0-9]{4,8}$`)

func (s *Server) sendSMS(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	var input struct {
		Phone string `json:"phone"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Phone = strings.TrimSpace(input.Phone)
	if !phonePattern.MatchString(input.Phone) {
		writeError(w, 400, "phone_invalid", "请输入有效的 11 位手机号")
		return
	}
	key := make([]byte, 24)
	if _, err := rand.Read(key); err != nil {
		writeError(w, 500, "session_error", "无法创建登录会话")
		return
	}
	id := hex.EncodeToString(key)
	now := time.Now()
	s.mu.Lock()
	for key, value := range s.sessions {
		if now.After(value.expires) {
			delete(s.sessions, key)
		}
	}
	for phone, expires := range s.cooldown {
		if now.After(expires) {
			delete(s.cooldown, phone)
		}
	}
	if next := s.cooldown[input.Phone]; now.Before(next) {
		s.mu.Unlock()
		retry := int(time.Until(next).Seconds()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(retry))
		jsonResponse(w, 429, map[string]any{"code": "sms_cooldown", "message": "请等待验证码冷却时间结束后再发送", "retry_after": retry})
		return
	}
	if len(s.sessions)+s.inflight >= 20 || len(s.cooldown) >= 50 {
		s.mu.Unlock()
		writeError(w, 429, "session_limit", "登录请求过多，请稍后重试")
		return
	}
	// Reserve before the network request: uncertain delivery must not auto-resend.
	s.cooldown[input.Phone] = now.Add(time.Minute)
	s.inflight++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.inflight--; s.mu.Unlock() }()
	if err := s.platform.SendSMS(r.Context(), input.Phone); err != nil {
		upstreamError(w, err)
		return
	}
	s.mu.Lock()
	s.sessions[id] = &session{phone: input.Phone, expires: time.Now().Add(5 * time.Minute)}
	s.cooldown[input.Phone] = time.Now().Add(time.Minute)
	s.mu.Unlock()
	jsonResponse(w, 200, map[string]any{"session_id": id, "retry_after": 60, "expires_in": 300})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	var input struct {
		SessionID string `json:"session_id"`
		Code      string `json:"code"`
	}
	if !decode(w, r, &input) {
		return
	}
	s.mu.Lock()
	sess := s.sessions[input.SessionID]
	if sess == nil || time.Now().After(sess.expires) {
		delete(s.sessions, input.SessionID)
		s.mu.Unlock()
		writeError(w, 400, "session_expired", "登录会话已结束，请重新获取验证码")
		return
	}
	if sess.confirmed {
		profile := sess.profile
		s.mu.Unlock()
		jsonResponse(w, 200, map[string]any{"status": "confirmed", "account": profile})
		return
	}
	if sess.attempts >= 5 && sess.pendingToken == "" {
		delete(s.sessions, input.SessionID)
		s.mu.Unlock()
		writeError(w, 400, "session_expired", "验证次数已用完，请重新获取验证码")
		return
	}
	if sess.busy {
		s.mu.Unlock()
		writeError(w, 409, "login_pending", "正在确认登录，请稍候")
		return
	}
	if sess.pendingToken == "" && !codePattern.MatchString(input.Code) {
		s.mu.Unlock()
		writeError(w, 400, "code_invalid", "请输入有效的短信验证码")
		return
	}
	sess.busy = true
	if sess.pendingToken == "" {
		sess.attempts++
	}
	phone := sess.phone
	pendingToken := sess.pendingToken
	s.mu.Unlock()
	defer func() { s.mu.Lock(); sess.busy = false; s.mu.Unlock() }()
	reportFailure := func(err error) {
		var failure *platform.Error
		s.mu.Lock()
		if errors.As(err, &failure) {
			sess.lastError = map[string]string{"code": failure.Code, "upstream_path": failure.UpstreamPath}
		}
		s.mu.Unlock()
		upstreamError(w, err)
	}
	if pendingToken == "" {
		authenticated, err := s.platform.Login(r.Context(), phone, input.Code)
		if err != nil {
			reportFailure(err)
			return
		}
		if authenticated.Token == "" || authenticated.ForcedBind {
			reportFailure(&platform.Error{Code: "login_incomplete", Message: "平台登录尚未完成"})
			return
		}
		pendingToken = authenticated.Token
		s.mu.Lock()
		sess.pendingToken = pendingToken
		s.mu.Unlock()
	}
	result, err := s.platform.Check(r.Context(), pendingToken)
	if err != nil {
		reportFailure(err)
		return
	}
	if !platform.SamePhone(phone, result.User.Phone) {
		s.mu.Lock()
		sess.pendingToken = ""
		sess.attempts = 5
		s.mu.Unlock()
		reportFailure(&platform.Error{Code: "identity_mismatch", Message: "平台返回的账号身份与登录手机号不一致，未保存账号"})
		return
	}
	id := sha256.Sum256([]byte("panguai/account/" + phone))
	profile := accounts.Profile{ID: "local_" + hex.EncodeToString(id[:12]), Name: result.User.Nickname, Phone: phone[:3] + "****" + phone[7:], UpdatedAt: time.Now().UTC()}
	if profile.Name == "" {
		profile.Name = "胖乖用户"
	}
	if err := s.store.Save(accounts.Record{Profile: profile, Token: result.Token, PlatformID: result.User.ID}); err != nil {
		writeError(w, 500, "storage_error", "平台已登录，但本机保存失败；请检查数据目录")
		return
	}
	s.mu.Lock()
	sess.confirmed = true
	sess.profile = profile
	sess.pendingToken = ""
	sess.lastError = nil
	s.mu.Unlock()
	jsonResponse(w, 200, map[string]any{"status": "confirmed", "account": profile})
}

func (s *Server) sessionStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[r.PathValue("id")]
	if sess == nil || time.Now().After(sess.expires) {
		delete(s.sessions, r.PathValue("id"))
		writeError(w, 404, "session_expired", "登录会话不存在或已过期")
		return
	}
	status := "pending"
	if sess.pendingToken != "" {
		status = "verifying"
	}
	if sess.busy {
		status = "confirming"
	}
	if sess.confirmed {
		status = "confirmed"
	}
	result := map[string]any{"status": status, "phone": sess.phone[:3] + "****" + sess.phone[7:], "expires_at": sess.expires.UTC()}
	if sess.lastError != nil {
		result["last_error"] = sess.lastError
	}
	if sess.confirmed {
		result["account"] = sess.profile
	}
	jsonResponse(w, 200, result)
}

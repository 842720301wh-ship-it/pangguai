package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"panguai/internal/platform"
)

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	jsonResponse(w, status, map[string]string{"code": code, "message": message})
}

func upstreamError(w http.ResponseWriter, err error) {
	var typed *platform.Error
	if errors.As(err, &typed) {
		status := http.StatusBadGateway
		if typed.Code == "verification_required" {
			status = http.StatusConflict
		}
		jsonResponse(w, status, map[string]string{"code": typed.Code, "message": typed.Message, "upstream_path": typed.UpstreamPath})
		return
	}
	writeError(w, 502, "platform_error", "平台请求未完成")
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		writeError(w, 400, "invalid_input", "请求参数无效")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "invalid_input", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}

func (s *Server) requirePlatform(w http.ResponseWriter) bool {
	if s.platform == nil {
		writeError(w, 503, "protocol_pending", "登录服务尚未配置，请先完成本机平台配置")
		return false
	}
	return true
}

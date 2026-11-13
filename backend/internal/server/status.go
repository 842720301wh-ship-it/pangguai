package server

import (
	"net/http"
	"panguai/internal/buildinfo"
)

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	methods := []string{}
	message := "登录服务尚未配置"
	if s.platform != nil {
		methods = []string{"sms"}
		message = "短信登录已接入，等待本人账号验证"
	}
	if _, ok := s.platform.(wechatGateway); ok {
		methods = append(methods, "wechat_credential")
	}
	jsonResponse(w, 200, map[string]any{"app": "水到渠成", "version": buildinfo.Version, "protocol_ready": s.platform != nil, "methods": methods, "wechat_native_ready": false, "message": message})
}

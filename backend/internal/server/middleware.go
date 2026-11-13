package server

import (
	"context"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			writeError(w, 403, "host_rejected", "仅允许本机访问")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if origin != "http://"+r.Host && origin != "http://127.0.0.1:5173" && origin != "http://localhost:5173" {
				writeError(w, 403, "origin_rejected", "来源不受信任")
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Panguai-Client") != "web" {
				writeError(w, 403, "client_rejected", "请求缺少本机客户端标记")
				return
			}
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if r.Method != http.MethodDelete && (err != nil || mediaType != "application/json") {
				writeError(w, 415, "content_type", "请求必须使用 JSON")
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

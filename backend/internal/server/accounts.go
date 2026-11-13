package server

import (
	"net/http"
	"panguai/internal/platform"
	"time"
)

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.List()
	if err != nil {
		writeError(w, 500, "storage_error", "无法读取本机账号文件")
		return
	}
	jsonResponse(w, 200, map[string]any{"accounts": list})
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	record, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "账号不存在或本机账号文件不可用")
		return
	}
	result, err := s.platform.Check(r.Context(), record.Token)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if (record.PlatformID != "" && record.PlatformID != result.User.ID) || (record.PlatformID == "" && !platform.SamePhone(record.Profile.Phone, result.User.Phone)) {
		writeError(w, 409, "identity_mismatch", "账号身份不一致，未替换本机凭据")
		return
	}
	record.Token = result.Token
	record.Profile.UpdatedAt = time.Now().UTC()
	if result.User.Nickname != "" {
		record.Profile.Name = result.User.Nickname
	}
	if err := s.store.Save(record); err != nil {
		writeError(w, 500, "storage_error", "本机账号保存失败")
		return
	}
	jsonResponse(w, 200, map[string]any{"status": "valid", "account": record.Profile})
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Remove(r.PathValue("id")); err != nil {
		writeError(w, 400, "remove_failed", "本机账号移除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

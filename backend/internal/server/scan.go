package server

import (
	"context"
	"net/http"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"time"
)

type scanGateway interface {
	ScanDevice(context.Context, string, string) (platform.RecentDevice, error)
	ScannedDeviceState(context.Context, string, string) (platform.RecentDevice, error)
}

func (s *Server) scanDevice(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	g, ok := s.platform.(scanGateway)
	if !ok {
		writeError(w, 503, "scan_pending", "扫码查询尚未配置")
		return
	}
	var input struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &input) {
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "扫码不接受额外参数")
		return
	}
	if _, _, err := platform.ParseDeviceQR(input.Text); err != nil {
		writeError(w, 400, "qr_invalid", "请扫描胖乖设备上的二维码")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	d, err := g.ScanDevice(ctx, rec.Token, input.Text)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if err = s.store.AddDevice(rec.Profile.ID, accounts.SavedDevice{ID: string(d.GoodsID), Name: d.Name, Category: d.Category, Organization: d.Organization}); err != nil {
		writeError(w, 503, "device_save_failed", "设备未能保存到本机，请稍后重试")
		return
	}
	jsonResponse(w, 200, map[string]any{"device": d, "source": "/goods/scan/v2", "started": false})
}

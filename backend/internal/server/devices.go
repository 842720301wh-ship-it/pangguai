package server

import (
	"context"
	"net/http"
	"regexp"

	"panguai/internal/platform"
)

type deviceGateway interface {
	RecentDevices(context.Context, string, string) ([]platform.RecentDevice, error)
}

type observationGateway interface {
	deviceGateway
	DeviceDetails(context.Context, string, string) (platform.Observation, error)
	RecentOrders(context.Context, string) ([]platform.Observation, error)
}

var categoryPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{0,64}$`)

func (s *Server) recentDevices(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	gateway, ok := s.platform.(deviceGateway)
	if !ok {
		writeError(w, 503, "devices_pending", "设备只读查询尚未配置")
		return
	}
	if len(r.URL.Query()) > 1 || (len(r.URL.Query()) == 1 && !r.URL.Query().Has("category")) || len(r.URL.Query()["category"]) > 1 {
		writeError(w, 400, "invalid_input", "只允许一个可选 category 参数")
		return
	}
	category := r.URL.Query().Get("category")
	if !categoryPattern.MatchString(category) {
		writeError(w, 400, "category_invalid", "设备分类参数无效")
		return
	}
	record, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	devices, err := gateway.RecentDevices(r.Context(), record.Token, category)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if devices == nil {
		devices = []platform.RecentDevice{}
	}
	seen := map[platform.DeviceID]bool{}
	for _, d := range devices {
		seen[d.GoodsID] = true
	}
	for _, saved := range record.Devices {
		if seen[platform.DeviceID(saved.ID)] || (category != "" && saved.Category != category) {
			continue
		}
		d := platform.RecentDevice{GoodsID: platform.DeviceID(saved.ID), Name: saved.Name, Category: saved.Category, Organization: saved.Organization}
		if g, ok := s.platform.(scanGateway); ok {
			if fresh, err := g.ScannedDeviceState(r.Context(), record.Token, saved.ID); err == nil {
				d = fresh
			}
		}
		devices = append(devices, d)
	}
	jsonResponse(w, 200, map[string]any{"devices": devices, "source": "/goods/latestUsed", "mode": "readonly"})
}

func (s *Server) deviceDetails(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	gate, ok := s.platform.(observationGateway)
	if !ok {
		writeError(w, 503, "devices_pending", "设备只读观察尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "设备只读查询不接收其他参数")
		return
	}
	record, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	list, err := gate.RecentDevices(r.Context(), record.Token, "")
	if err != nil {
		upstreamError(w, err)
		return
	}
	allowed := false
	for _, d := range list {
		if string(d.GoodsID) == r.PathValue("device") {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, 404, "device_not_listed", "只允许观察本人最近使用列表中的设备")
		return
	}
	result, err := gate.DeviceDetails(r.Context(), record.Token, r.PathValue("device"))
	if err != nil {
		upstreamError(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"device": result, "source": "/goods/normal/details", "mode": "readonly"})
}

func (s *Server) recentOrders(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	gate, ok := s.platform.(observationGateway)
	if !ok {
		writeError(w, 503, "orders_pending", "订单只读观察尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "本接口只观察本人最近十条订单")
		return
	}
	record, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	result, err := gate.RecentOrders(r.Context(), record.Token)
	if err != nil {
		upstreamError(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"orders": result, "source": "/order/list", "mode": "readonly"})
}

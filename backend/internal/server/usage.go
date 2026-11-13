package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"panguai/internal/accounts"
	"panguai/internal/platform"
)

type usageGateway interface {
	deviceGateway
	UsageOptions(context.Context, string, string) (platform.UsageDevice, error)
	StartUsage(context.Context, string, platform.UsageDevice, string, platform.UsageLocation) (platform.UsageReceipt, error)
}

type startupStatusGateway interface {
	RecentOrders(context.Context, string) ([]platform.Observation, error)
	StartupStatus(context.Context, string, string) (bool, error)
}

func (s *Server) usageStartStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	g, ok := s.platform.(startupStatusGateway)
	if !ok {
		writeError(w, 503, "usage_pending", "启动状态查询尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "不接受额外查询参数")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	orders, err := g.RecentOrders(ctx, rec.Token)
	if err != nil {
		upstreamError(w, err)
		return
	}
	allowed := false
	for _, order := range orders {
		if platform.ObservationID(order, "orderNo") == r.PathValue("order") {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, 404, "order_not_listed", "仅可查询本人最近订单的启动状态")
		return
	}
	started, err := g.StartupStatus(ctx, rec.Token, r.PathValue("order"))
	if err != nil {
		upstreamError(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"platform_started": started, "mode": "readonly", "source": "/machine/device_is_start"})
}

type usageIntent struct {
	account, device, sku string
	identity             [32]byte
	fingerprint          [32]byte
	location             platform.UsageLocation
	expires              time.Time
	attempted            bool
	status               string
	receipt              platform.UsageReceipt
	sessionID            string
}

func usageFingerprint(device platform.UsageDevice, sku platform.Observation) [32]byte {
	raw, _ := json.Marshal(struct {
		ID             platform.DeviceID
		IMEI, Category string
		Bluetooth      bool
		PayModel       int
		Service        platform.Observation
		BillingHash    [32]byte
	}{device.GoodsID, device.IMEI, device.Category, device.Bluetooth, device.PayModel, sku, device.BillingHashes[platform.ObservationID(sku, "skuId")]})
	return sha256.Sum256(raw)
}

func availableService(device platform.UsageDevice, id string) (platform.Observation, bool) {
	for _, sku := range device.Services {
		if platform.ObservationID(sku, "skuId") != id {
			continue
		}
		if string(sku["scanAvailable"]) != "true" {
			return nil, false
		}
		for _, field := range []string{"name", "price", "amount", "unit"} {
			if len(sku[field]) == 0 || string(sku[field]) == "null" || string(sku[field]) == `""` {
				return nil, false
			}
		}
		var unit string
		var levels []map[string]json.RawMessage
		if json.Unmarshal(sku["billing_unit"], &unit) != nil || (unit != "元/L" && unit != "元/秒") || json.Unmarshal(sku["billing_levels"], &levels) != nil || len(levels) == 0 {
			return nil, false
		}
		return sku, true
	}
	return nil, false
}

// ownUsageDevice checks membership before any details/specification request.
func ownUsageDevice(ctx context.Context, g usageGateway, token, id string, saved []accounts.SavedDevice) (platform.UsageDevice, bool, error) {
	list, err := g.RecentDevices(ctx, token, "")
	if err != nil {
		return platform.UsageDevice{}, false, err
	}
	for _, d := range list {
		if string(d.GoodsID) == id {
			if d.WorkStatus != 10 {
				return platform.UsageDevice{}, false, nil
			}
			device, err := g.UsageOptions(ctx, token, id)
			return device, true, err
		}
	}
	if scanner, ok := g.(scanGateway); ok {
		for _, d := range saved {
			if d.ID != id {
				continue
			}
			fresh, err := scanner.ScannedDeviceState(ctx, token, id)
			if err != nil {
				return platform.UsageDevice{}, false, err
			}
			if fresh.WorkStatus != 10 {
				return platform.UsageDevice{}, false, nil
			}
			device, err := g.UsageOptions(ctx, token, id)
			return device, true, err
		}
	}
	return platform.UsageDevice{}, false, nil
}

func (s *Server) usageOptions(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	g, ok := s.platform.(usageGateway)
	if !ok {
		writeError(w, 503, "usage_pending", "使用接口尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "不接受额外查询参数")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	device, allowed, err := ownUsageDevice(ctx, g, rec.Token, r.PathValue("device"), rec.Devices)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if !allowed {
		writeError(w, 409, "device_unavailable", "设备未在本人最近列表中或当前不空闲")
		return
	}
	jsonResponse(w, 200, map[string]any{"device": device, "start_supported": platform.UsageSupported(device), "mode": "readonly", "billing": "计量设备按平台实际用量结算，价格字段为平台原值；本次查询不会启动或扣费"})
}

func (s *Server) prepareUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	g, ok := s.platform.(usageGateway)
	if !ok {
		writeError(w, 503, "usage_pending", "使用接口尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "不接受额外查询参数")
		return
	}
	var input struct {
		Sku              string   `json:"sku_id"`
		Latitude         *float64 `json:"latitude"`
		Longitude        *float64 `json:"longitude"`
		CoordinateSystem string   `json:"coordinate_system"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Sku == "" || len(input.Sku) > 128 || input.Latitude == nil || input.Longitude == nil {
		writeError(w, 400, "usage_input_invalid", "请选择服务并提供实际经纬度")
		return
	}
	location, valid := platform.PlatformLocation(platform.UsageLocation{Latitude: *input.Latitude, Longitude: *input.Longitude}, input.CoordinateSystem)
	if !valid {
		writeError(w, 400, "location_invalid", "实际经纬度无效")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	if pending, err := s.store.UsageAttemptPending(r.PathValue("id"), r.PathValue("device")); err != nil {
		writeError(w, 503, "usage_journal_unavailable", "无法核对本机启动记录，未发送请求")
		return
	} else if pending {
		writeError(w, 409, "usage_already_attempted", "已有使用请求，正在核对设备状态，请勿重复启动")
		return
	}
	if entry, exists, err := s.store.GetUsageSession(rec.Profile.ID, r.PathValue("device")); err != nil {
		writeError(w, 503, "usage_journal_unavailable", "无法核对当前使用会话，未发送请求")
		return
	} else if exists && entry.State != "finished" {
		writeError(w, 409, "usage_already_attempted", "已有使用会话，请等待状态同步或结束本次使用")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	device, allowed, err := ownUsageDevice(ctx, g, rec.Token, r.PathValue("device"), rec.Devices)
	if err != nil {
		upstreamError(w, err)
		return
	}
	if !allowed {
		writeError(w, 409, "device_unavailable", "设备未在本人最近列表中或当前不空闲")
		return
	}
	if !platform.UsageSupported(device) {
		writeError(w, 409, "usage_unsupported", "首版仅支持非蓝牙、payModel=0 的饮水和淋浴设备")
		return
	}
	sku, ok := availableService(device, input.Sku)
	if !ok {
		writeError(w, 409, "service_unavailable", "服务不可用或计费信息不完整")
		return
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		writeError(w, 500, "session_error", "无法创建确认会话")
		return
	}
	key := hex.EncodeToString(random[:])
	now := time.Now()
	s.mu.Lock()
	for key, intent := range s.usage {
		if now.After(intent.expires.Add(30 * time.Minute)) {
			delete(s.usage, key)
			continue
		}
		if intent.account == r.PathValue("id") && intent.device == r.PathValue("device") && intent.attempted && intent.status != "not_sent" && intent.status != "finished" {
			s.mu.Unlock()
			writeError(w, 409, "usage_already_attempted", "该设备已有启动请求，请先在官方 App 核对结果；本机暂不重复启动")
			return
		}
	}
	if len(s.usage) >= 100 {
		s.mu.Unlock()
		writeError(w, 429, "session_limit", "确认会话过多，请稍后再试")
		return
	}
	s.usage[key] = &usageIntent{account: r.PathValue("id"), device: r.PathValue("device"), sku: input.Sku, identity: sha256.Sum256([]byte(rec.Token)), fingerprint: usageFingerprint(device, sku), location: location, expires: now.Add(2 * time.Minute), status: "prepared"}
	s.mu.Unlock()
	jsonResponse(w, 200, map[string]any{"status": "prepared", "confirmation_key": key, "expires_in": 120, "device": map[string]any{"goods_id": device.GoodsID, "name": device.Name, "category": device.Category}, "service": sku, "billing": "计量服务按平台实际用量结算。确认启动可能产生费用，请在设备旁核对服务、费率和单位。", "mutations_sent": false})
}

func (s *Server) startUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	g, ok := s.platform.(usageGateway)
	if !ok {
		writeError(w, 503, "usage_pending", "使用接口尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "不接受额外查询参数")
		return
	}
	var input struct {
		Key     string `json:"confirmation_key"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !input.Confirm {
		writeError(w, 400, "confirmation_required", "需要本人明确确认启动和计费")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	deviceLock := s.usageLock(rec.Profile.ID, r.PathValue("device"))
	if !deviceLock.TryLock() {
		writeError(w, 409, "usage_busy", "该设备请求正在处理，请稍候")
		return
	}
	defer deviceLock.Unlock()
	s.mu.Lock()
	intent := s.usage[input.Key]
	if intent == nil || intent.account != r.PathValue("id") || intent.device != r.PathValue("device") || intent.identity != sha256.Sum256([]byte(rec.Token)) {
		s.mu.Unlock()
		writeError(w, 400, "confirmation_invalid", "确认会话无效或账号已经变化")
		return
	}
	if intent.attempted {
		status, receipt := intent.status, intent.receipt
		s.mu.Unlock()
		jsonResponse(w, 200, map[string]any{"status": status, "receipt": receipt, "replayed": true, "message": "返回已有请求结果，未再次启动。请核对官方 App 和实物状态。"})
		return
	}
	if time.Now().After(intent.expires) {
		s.mu.Unlock()
		writeError(w, 400, "confirmation_expired", "确认已过期，请重新读取费率")
		return
	}
	// Consume before the first network call. Concurrent keys for the same device
	// cannot send a second command, even when the first result is uncertain.
	for _, other := range s.usage {
		if other != intent && other.account == intent.account && other.device == intent.device && other.attempted && other.status != "not_sent" && other.status != "finished" {
			s.mu.Unlock()
			writeError(w, 409, "usage_already_attempted", "已有启动请求，请先核对结果")
			return
		}
	}
	intent.attempted = true
	intent.status = "checking"
	copyIntent := *intent
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	device, allowed, err := ownUsageDevice(ctx, g, rec.Token, copyIntent.device, rec.Devices)
	if err == nil {
		sku, available := availableService(device, copyIntent.sku)
		if !allowed || !platform.UsageSupported(device) || !available || usageFingerprint(device, sku) != copyIntent.fingerprint {
			err = &platform.Error{Code: "usage_changed", Message: "设备状态、服务或费率已经变化，未发送启动请求"}
		}
	}
	if err != nil {
		s.mu.Lock()
		intent.status = "not_sent"
		s.mu.Unlock()
		upstreamError(w, err)
		return
	}
	reserved, journalErr := s.store.ReserveUsageAttempt(copyIntent.account, copyIntent.device)
	if journalErr != nil || !reserved {
		s.mu.Lock()
		intent.status = "not_sent"
		s.mu.Unlock()
		if journalErr != nil {
			writeError(w, 503, "usage_journal_unavailable", "无法保存防重复记录，未发送启动请求")
		} else {
			writeError(w, 409, "usage_already_attempted", "本机已有该设备的启动记录，未重复发送")
		}
		return
	}
	var serviceName string
	for _, sku := range device.Services {
		if platform.ObservationID(sku, "skuId") == copyIntent.sku {
			_ = json.Unmarshal(sku["name"], &serviceName)
		}
	}
	entry := accounts.UsageSession{ID: input.Key, Device: copyIntent.device, SKU: copyIntent.sku, Category: device.Category, Service: serviceName, StartedAt: time.Now().UTC(), State: "starting"}
	if err := s.store.CreateUsageSession(copyIntent.account, copyIntent.device, entry); err != nil {
		// No command was sent. Retain the guard if the session cannot be stored.
		s.mu.Lock()
		intent.status = "not_sent"
		s.mu.Unlock()
		writeError(w, 503, "usage_session_unavailable", "无法保存使用会话，未发送启动请求")
		return
	}
	s.mu.Lock()
	intent.status = "sending"
	intent.sessionID = entry.ID
	s.mu.Unlock()
	receipt, err := g.StartUsage(ctx, rec.Token, device, copyIntent.sku, copyIntent.location)
	_, saveErr := s.store.UpdateUsageSession(copyIntent.account, copyIntent.device, entry.ID, func(e *accounts.UsageSession) error {
		e.OrderNo = receipt.OrderNo
		e.MessageID = receipt.MessageID
		if err != nil {
			e.State = "unknown"
		}
		return nil
	})
	s.liveCache.Delete(copyIntent.account + "\x00" + copyIntent.device)
	s.mu.Lock()
	if err != nil || saveErr != nil {
		intent.status = "unknown"
	} else {
		intent.status = "accepted"
		intent.receipt = receipt
	}
	s.mu.Unlock()
	if err != nil || saveErr != nil {
		// A timeout or response error can happen after the device received the
		// command. Never turn this into an automatic or freshly prepared retry.
		reply := map[string]any{"code": "usage_result_unknown", "status": "unknown", "message": "启动请求结果未确认，请在官方 App 核对；本机没有重试，请勿重复启动"}
		var failure *platform.Error
		if errors.As(err, &failure) {
			reply["upstream_code"] = failure.Code
			reply["upstream_path"] = failure.UpstreamPath
			reply["upstream_message"] = failure.Message
		}
		jsonResponse(w, 502, reply)
		return
	}
	jsonResponse(w, 200, map[string]any{"status": "accepted", "receipt": receipt, "session_id": entry.ID, "started_at": entry.StartedAt, "replayed": false, "message": "启动请求已发送，正在同步设备状态"})
}

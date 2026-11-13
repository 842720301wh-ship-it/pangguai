package server

import (
	"context"
	"errors"
	"net/http"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"sync"
	"time"
)

type lifecycleGateway interface {
	SyncUsage(context.Context, string, string, string) (platform.UsageSync, error)
	UsageCommandConfirmed(context.Context, string, string) (bool, error)
	UsageControl(context.Context, string, string) (platform.UsageControl, error)
	StartupStatus(context.Context, string, string) (bool, error)
	EndUsage(context.Context, string, string, string, string) error
}
type liveUsage struct {
	SessionID     string    `json:"session_id,omitempty"`
	State         string    `json:"state"`
	CanStop       bool      `json:"can_stop"`
	StopSupported *bool     `json:"stop_supported,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
	Service       string    `json:"service_name,omitempty"`
	CostYuan      *string   `json:"cost_yuan"`
	CostSource    string    `json:"cost_source"`
	Note          string    `json:"note,omitempty"`
}

func sessionView(e accounts.UsageSession) liveUsage {
	return liveUsage{SessionID: e.ID, State: e.State, StartedAt: e.StartedAt, FinishedAt: e.FinishedAt, UpdatedAt: time.Now().UTC(), Service: e.Service, CostSource: "unavailable", Note: "平台暂未回传实时费用，最终以平台结算为准"}
}
func (s *Server) usageLock(account, device string) *sync.Mutex {
	v, _ := s.usageLocks.LoadOrStore(account+"\x00"+device, &sync.Mutex{})
	return v.(*sync.Mutex)
}
func (s *Server) releaseUsage(account, device string) error {
	pending, err := s.store.UsageAttemptPending(account, device)
	if err != nil {
		return err
	}
	if pending {
		if err := s.store.ReleaseUsageAttempt(account, device); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, i := range s.usage {
		if i.account == account && i.device == device && i.attempted {
			i.status = "finished"
		}
	}
	return nil
}

// Called under the per-device lock. Reads never issue unlock/end/payment commands.
func (s *Server) readLiveUsage(ctx context.Context, rec accounts.Record, device string, force bool) (liveUsage, error) {
	key := rec.Profile.ID + "\x00" + device
	if !force {
		if cached, ok := s.liveCache.Load(key); ok {
			view := cached.(liveUsage)
			if time.Since(view.UpdatedAt) < 3*time.Second {
				return view, nil
			}
		}
	}
	e, exists, err := s.store.GetUsageSession(rec.Profile.ID, device)
	if err != nil {
		return liveUsage{}, err
	}
	if !exists {
		pending, err := s.store.UsageAttemptPending(rec.Profile.ID, device)
		if err != nil {
			return liveUsage{}, err
		}
		if pending {
			return s.reconcileLegacyUsage(ctx, rec, device)
		}
		return liveUsage{State: "idle", UpdatedAt: time.Now().UTC(), CostSource: "unavailable"}, nil
	}
	view := sessionView(e)
	original := e
	if e.State == "finished" {
		if err := s.releaseUsage(rec.Profile.ID, device); err != nil {
			return view, err
		}
		return view, nil
	}
	g, ok := s.platform.(lifecycleGateway)
	if !ok {
		return view, errors.New("使用状态查询尚未配置")
	}
	// Receipt confirmation is account-bound; a shared machine's busy bit is not ownership.
	if !e.Confirmed {
		if e.MessageID != "" {
			e.Confirmed, err = g.UsageCommandConfirmed(ctx, rec.Token, e.MessageID)
		} else if e.OrderNo != "" {
			e.Confirmed, err = g.StartupStatus(ctx, rec.Token, e.OrderNo)
		}
		if err != nil {
			return view, err
		}
	}
	live, err := g.SyncUsage(ctx, rec.Token, e.Category, e.SKU)
	if err != nil {
		return view, err
	}
	identity := string(live.Identify)
	mismatch := e.SyncID != "" && identity != "" && e.SyncID != identity
	if e.Confirmed && e.SyncID == "" && identity != "" && !live.Terminal() && !live.Fault() {
		e.SyncID = identity
	}
	switch {
	case mismatch:
		e.State = "unknown"
		view.Note = "设备使用标识已变化，请核对官方 App"
	case live.Fault():
		e.State = "unknown"
		view.Note = "平台报告设备异常，请在官方 App 核对"
	case live.Terminal() && e.SyncID != "" && (identity == e.SyncID || identity == "") && e.ObservedRunning:
		e.State = "finished"
		e.FinishedAt = time.Now().UTC()
	case e.Confirmed && !live.Terminal() && e.SyncID != "" && identity == e.SyncID:
		e.ObservedRunning = true
		if e.StopAttempted {
			e.State = "stopping"
		} else {
			e.State = "running"
		}
	default:
		if e.StopAttempted {
			e.State = "stopping"
		} else if e.State != "unknown" {
			e.State = "starting"
		}
	}
	saved := e
	if e != original {
		e.LastCheckedAt = time.Now().UTC()
		saved, err = s.store.UpdateUsageSession(rec.Profile.ID, device, e.ID, func(current *accounts.UsageSession) error {
			// A late read cannot roll back a stop reservation or a terminal session.
			if current.State == "finished" {
				e = *current
				return nil
			}
			if current.StopAttempted && !e.StopAttempted {
				e.StopAttempted = true
				if e.State == "running" {
					e.State = "stopping"
				}
			}
			*current = e
			return nil
		})
	}
	if err != nil {
		return view, err
	}
	note := view.Note
	view = sessionView(saved)
	view.Note = note
	if saved.State == "finished" {
		if err = s.releaseUsage(rec.Profile.ID, device); err != nil {
			return view, err
		}
	} else if saved.State == "running" && saved.Confirmed && saved.SyncID != "" && !saved.StopAttempted {
		info, err := g.UsageControl(ctx, rec.Token, device)
		if err != nil {
			return view, err
		}
		view.CanStop = string(info.GoodsID) == device && info.Category == saved.Category && info.Bluetooth == 0 && info.CanFinish
		supported := info.CanFinish && info.Bluetooth == 0
		view.StopSupported = &supported
		if !supported {
			view.Note = "本设备不支持 App 远程终止，请使用设备自身的结束方式"
		}
	}
	s.liveCache.Store(key, view)
	return view, nil
}

// Old versions persisted only a timed duplicate guard. It can be released early
// only when all known SKU syncs are terminal AND a fresh list says idle.
func (s *Server) reconcileLegacyUsage(ctx context.Context, rec accounts.Record, device string) (liveUsage, error) {
	view := liveUsage{State: "unknown", UpdatedAt: time.Now().UTC(), CostSource: "unavailable", Note: "旧版启动结果正在核对，请勿重复启动"}
	g, ok := s.platform.(usageGateway)
	live, hasLive := s.platform.(lifecycleGateway)
	if !ok || !hasLive {
		return view, nil
	}
	options, allowed, err := ownUsageDevice(ctx, g, rec.Token, device, rec.Devices)
	if err != nil || !allowed {
		return view, err
	}
	checked := 0
	for _, sku := range options.Services {
		if string(sku["scanAvailable"]) != "true" {
			continue
		}
		state, err := live.SyncUsage(ctx, rec.Token, options.Category, platform.ObservationID(sku, "skuId"))
		if err != nil {
			return view, err
		}
		checked++
		if !state.Terminal() {
			return view, nil
		}
	}
	if checked > 0 {
		if err = s.releaseUsage(rec.Profile.ID, device); err != nil {
			return view, err
		}
		view.State = "idle"
		view.Note = "平台已确认空闲，旧版防重复记录已解除"
	}
	return view, nil
}
func (s *Server) usageState(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "不接受额外参数")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	lock := s.usageLock(rec.Profile.ID, r.PathValue("device"))
	lock.Lock()
	defer lock.Unlock()
	view, err := s.readLiveUsage(r.Context(), rec, r.PathValue("device"), false)
	if err != nil {
		writeError(w, 502, "usage_state_unavailable", "状态暂未同步，请稍后核对；未重复发送设备指令")
		return
	}
	jsonResponse(w, 200, view)
}
func (s *Server) stopUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	var input struct {
		SessionID string `json:"session_id"`
		Confirm   bool   `json:"confirm"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !input.Confirm || input.SessionID == "" || len(r.URL.Query()) != 0 {
		writeError(w, 400, "confirmation_required", "需要确认终止当前使用会话")
		return
	}
	rec, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	device := r.PathValue("device")
	lock := s.usageLock(rec.Profile.ID, device)
	lock.Lock()
	defer lock.Unlock()
	e, exists, err := s.store.GetUsageSession(rec.Profile.ID, device)
	if err != nil || !exists || e.ID != input.SessionID {
		writeError(w, 409, "usage_session_changed", "当前使用会话已变化，未发送终止请求")
		return
	}
	if e.StopAttempted || e.State == "finished" {
		jsonResponse(w, 200, map[string]any{"status": e.State, "replayed": true, "message": "已提交过终止请求，正在查询结果"})
		return
	}
	view, err := s.readLiveUsage(r.Context(), rec, device, true)
	if err != nil || !view.CanStop {
		writeError(w, 409, "stop_unavailable", "暂未确认本人正在使用且可远程结束，未发送终止请求")
		return
	}
	e, err = s.store.UpdateUsageSession(rec.Profile.ID, device, input.SessionID, func(entry *accounts.UsageSession) error {
		if entry.StopAttempted || entry.State != "running" {
			return errors.New("使用会话已经变化")
		}
		entry.StopAttempted = true
		entry.State = "stopping"
		return nil
	})
	if err != nil {
		writeError(w, 409, "usage_session_changed", "会话已变化或无法保存，未发送终止请求")
		return
	}
	g := s.platform.(lifecycleGateway)
	err = g.EndUsage(r.Context(), rec.Token, e.Category, device, e.OrderNo)
	s.liveCache.Delete(rec.Profile.ID + "\x00" + device)
	if err != nil {
		writeError(w, 502, "stop_result_unknown", "终止结果待确认，正在查询；未自动重试，请核对实物与官方 App")
		return
	}
	jsonResponse(w, 200, map[string]any{"status": "stopping", "replayed": false, "message": "终止请求已发送，正在确认结束"})
}

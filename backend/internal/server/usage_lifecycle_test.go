package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeLifecycle struct {
	fakeUsageGateway
	status    int
	identity  platform.DeviceID
	support   bool
	stops     atomic.Int32
	stopError bool
}

func (g *fakeLifecycle) SyncUsage(_ context.Context, token, category, sku string) (platform.UsageSync, error) {
	if token != "fixture-private-token" || category != "04" || sku != "fixture-sku" {
		panic("incorrect saved usage context")
	}
	return platform.UsageSync{Status: &g.status, Identify: g.identity}, nil
}
func (g *fakeLifecycle) UsageCommandConfirmed(context.Context, string, string) (bool, error) {
	return true, nil
}
func (g *fakeLifecycle) StartupStatus(_ context.Context, token, order string) (bool, error) {
	if token != "fixture-private-token" || order != "fixture-order" {
		panic("wrong receipt")
	}
	return true, nil
}
func (g *fakeLifecycle) UsageControl(_ context.Context, token, device string) (platform.UsageControl, error) {
	if token != "fixture-private-token" || device != "fixture-device" {
		panic("incorrect control identity")
	}
	return platform.UsageControl{GoodsID: "fixture-device", Category: "04", CanFinish: g.support}, nil
}
func (g *fakeLifecycle) EndUsage(_ context.Context, token, category, device, order string) error {
	if token != "fixture-private-token" || category != "04" || device != "fixture-device" || order != "fixture-order" {
		panic("incorrect end context")
	}
	g.stops.Add(1)
	if g.stopError {
		return errors.New("timeout after end command")
	}
	return nil
}

func TestAmbiguousStopSurvivesRestartWithoutResending(t *testing.T) {
	g := &fakeLifecycle{status: 1, identity: "fixture-order", support: true, stopError: true}
	h, store := lifecycleHandler(t, g)
	key := startFixtureSession(t, h)
	_ = readFixtureState(t, h)
	if r := apiRequest(t, h, "POST", usagePath+"stop", map[string]any{"session_id": key, "confirm": true}); r.Code != 502 {
		t.Fatal("ambiguous end reported completed")
	}
	restarted := New(store, g, "")
	if r := apiRequest(t, restarted, "POST", usagePath+"stop", map[string]any{"session_id": key, "confirm": true}); r.Code != 200 {
		t.Fatal("saved stop receipt unavailable")
	}
	if g.stops.Load() != 1 {
		t.Fatal("unknown end replayed after restart")
	}
	e, _, _ := store.GetUsageSession("fixture-account", "fixture-device")
	if !e.StopAttempted || e.State != "stopping" {
		t.Fatal("stop reservation not persistent")
	}
}
func lifecycleHandler(t *testing.T, g *fakeLifecycle) (http.Handler, *accounts.Store) {
	h, st := testHandler(t, g)
	if err := st.Save(accounts.Record{Profile: accounts.Profile{ID: "fixture-account"}, Token: "fixture-private-token"}); err != nil {
		t.Fatal(err)
	}
	return h, st
}
func startFixtureSession(t *testing.T, h http.Handler) string {
	key := prepareKey(t, h)
	r := apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": key, "confirm": true})
	if r.Code != 200 {
		t.Fatal("start failed", r.Body.String())
	}
	return key
}
func readFixtureState(t *testing.T, h http.Handler) liveUsage {
	r := apiRequest(t, h, "GET", usagePath+"state", nil)
	if r.Code != 200 {
		t.Fatal("state failed", r.Body.String())
	}
	if strings.Contains(r.Body.String(), "fixture-private-token") || strings.Contains(r.Body.String(), "fixture-order") {
		t.Fatal("private receipt exposed")
	}
	var view liveUsage
	if err := json.Unmarshal(r.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}
func TestUsageLifecycleSurvivesRestartAndReleasesGuardOnlyAfterConfirmedEnd(t *testing.T) {
	g := &fakeLifecycle{status: 1, identity: "fixture-order", support: true}
	h, store := lifecycleHandler(t, g)
	key := startFixtureSession(t, h)
	v := readFixtureState(t, h)
	if v.State != "running" || !v.CanStop || v.CostYuan != nil || v.CostSource != "unavailable" {
		t.Fatal("own active receipt not reflected independently from stale idle list", v)
	}
	// The session, not the old timed lock, continues to prohibit another start.
	if err := store.ReleaseUsageAttempt("fixture-account", "fixture-device"); err != nil {
		t.Fatal(err)
	}
	restarted := New(store, g, "")
	if r := apiRequest(t, restarted, "POST", usagePath+"prepare", map[string]any{"sku_id": "fixture-sku", "latitude": 22.5, "longitude": 113.5}); r.Code != 409 {
		t.Fatal("timed lock expiry bypassed active session")
	}
	if v = readFixtureState(t, restarted); v.SessionID != key || !v.CanStop {
		t.Fatal("active session not restored")
	}
	for _, body := range []map[string]any{{"session_id": "other", "confirm": true}, {"session_id": key}, {"session_id": key, "confirm": true, "orderNo": "other"}} {
		if r := apiRequest(t, restarted, "POST", usagePath+"stop", body); r.Code == 200 {
			t.Fatal("invalid stop accepted")
		}
	}
	if g.stops.Load() != 0 {
		t.Fatal("invalid requests stopped equipment")
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := apiRequest(t, restarted, "POST", usagePath+"stop", map[string]any{"session_id": key, "confirm": true})
			if r.Code != 200 {
				t.Error("stop failed")
			}
		}()
	}
	wg.Wait()
	if g.stops.Load() != 1 {
		t.Fatal("duplicate end command")
	}
	if v = readFixtureState(t, restarted); v.State != "stopping" || v.CanStop {
		t.Fatal("accepted end reported physically stopped", v)
	}
	// New handler bypasses the 3s read cache without sleeping in this test.
	g.status = 3
	g.identity = "" // Official zero-usage terminal responses can omit identify.
	settled := New(store, g, "")
	if v = readFixtureState(t, settled); v.State != "finished" {
		t.Fatal("terminal state not settled", v)
	}
	if pending, err := store.UsageAttemptPending("fixture-account", "fixture-device"); err != nil || pending {
		t.Fatal("confirmed end retained 32 minute lock")
	}
	_ = prepareKey(t, settled)
	if g.starts.Load() != 1 {
		t.Fatal("a state read started equipment")
	}
}
func TestStopRejectsAnotherSessionAndUnsupportedFinish(t *testing.T) {
	for _, tc := range []struct {
		identity platform.DeviceID
		support  bool
	}{{"different-session", true}, {"fixture-order", false}} {
		g := &fakeLifecycle{status: 1, identity: "fixture-order", support: true}
		h, store := lifecycleHandler(t, g)
		key := startFixtureSession(t, h)
		_ = readFixtureState(t, h)
		g.identity = tc.identity
		g.support = tc.support
		if r := apiRequest(t, h, "POST", usagePath+"stop", map[string]any{"session_id": key, "confirm": true}); r.Code != 409 {
			t.Fatal("unsafe stop accepted")
		}
		if g.stops.Load() != 0 {
			t.Fatal("foreign/unsupported usage ended")
		}
		if e, _, _ := store.GetUsageSession("fixture-account", "fixture-device"); tc.identity != "fixture-order" && e.State != "unknown" {
			t.Fatal("changed identity not latched as unknown")
		}
	}
	g := &fakeLifecycle{status: 1, identity: "fixture-order", support: true}
	h, _ := lifecycleHandler(t, g)
	if r := apiRequest(t, h, "POST", usagePath+"stop", map[string]any{"session_id": "unowned", "confirm": true}); r.Code != 409 || g.stops.Load() != 0 {
		t.Fatal("busy machine without own session could be stopped")
	}
}
func TestLegacyGuardNeedsTerminalSyncAndIdleList(t *testing.T) {
	for _, status := range []int{3, 6} {
		g := &fakeLifecycle{status: status, support: true}
		h, store := lifecycleHandler(t, g)
		_, _ = store.ReserveUsageAttempt("fixture-account", "fixture-device")
		view := readFixtureState(t, h)
		pending, _ := store.UsageAttemptPending("fixture-account", "fixture-device")
		if status == 3 && (pending || view.State != "idle") {
			t.Fatal("confirmed old end did not unlock")
		}
		if status == 6 && (!pending || view.State != "unknown") {
			t.Fatal("unknown/fault state cleared old guard")
		}
		if g.stops.Load() != 0 || g.starts.Load() != 0 {
			t.Fatal("legacy reconciliation controlled equipment")
		}
	}
}

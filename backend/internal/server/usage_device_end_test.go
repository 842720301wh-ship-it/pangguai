package server

import (
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"testing"
	"time"
)

func TestSyncFailureRecoversUsingFreshIdleEvidence(t *testing.T) {
	for _, name := range []string{"confirmed", "expired-receipt", "expired-message", "busy", "details-failed", "list-failed", "recent-unconfirmed", "startup-propagating"} {
		t.Run(name, func(t *testing.T) {
			idle := 10
			g := &fakeLifecycle{syncError: true, category: "08", listWorkStatus: &idle,
				deviceState: &platform.RecentDevice{GoodsID: "fixture-device", Category: "08"}}
			e := accounts.UsageSession{ID: "saved-session", Device: "fixture-device", SKU: "fixture-sku", Category: "08",
				OrderNo: "fixture-order", State: "unknown", Confirmed: true, StartedAt: time.Now().Add(-2 * time.Hour)}
			wantEnd := true
			switch name {
			case "expired-receipt", "expired-message":
				e.Confirmed = false
				g.confirmationError = true
				if name == "expired-message" {
					e.MessageID = "expired-message"
				}
			case "busy":
				idle = 20
				wantEnd = false
			case "details-failed":
				g.detailError = true
				wantEnd = false
			case "list-failed":
				g.listError = true
				wantEnd = false
			case "recent-unconfirmed":
				e.Confirmed = false
				e.StartedAt = time.Now().Add(-time.Minute)
				g.confirmationError = true
				wantEnd = false
			case "startup-propagating":
				e.StartedAt = time.Now()
				wantEnd = false
			}
			h, store := lifecycleHandler(t, g)
			_, _ = store.ReserveUsageAttempt("fixture-account", "fixture-device")
			if err := store.CreateUsageSession("fixture-account", "fixture-device", e); err != nil {
				t.Fatal(err)
			}
			if v := readFixtureState(t, h); v.State != "unknown" || v.CanStop {
				t.Fatal("failed sync must retain protection until verified", v)
			}
			saved, _, err := store.GetUsageSession("fixture-account", "fixture-device")
			if err != nil || saved.IdleSince.IsZero() == wantEnd {
				t.Fatal("incorrect first idle observation", saved, err)
			}
			if v := readFixtureState(t, New(store, g, "")); v.State != "unknown" {
				t.Fatal("unspaced read released guard", v)
			}
			// Seed an earlier observation, including the negative cases: a failed
			// or busy observation must reset it rather than release the guard.
			_, err = store.UpdateUsageSession("fixture-account", "fixture-device", e.ID, func(e *accounts.UsageSession) error {
				e.IdleSince = time.Now().Add(-6 * time.Second)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			v := readFixtureState(t, New(store, g, ""))
			if (v.State == "finished") != wantEnd || v.CanStop {
				t.Fatal("incorrect recovery after restart", v)
			}
			if pending, err := store.UsageAttemptPending("fixture-account", "fixture-device"); err != nil || pending == wantEnd {
				t.Fatal("incorrect duplicate guard", err)
			}
			saved, _, _ = store.GetUsageSession("fixture-account", "fixture-device")
			if !wantEnd && !saved.IdleSince.IsZero() {
				t.Fatal("unverified observation retained idle evidence")
			}
			if g.starts.Load() != 0 || g.stops.Load() != 0 {
				t.Fatal("recovery controlled equipment")
			}
		})
	}
}

func TestHardwareStopReleasesActualStartedSession(t *testing.T) {
	g := &fakeLifecycle{status: 1, identity: "fixture-order", support: true}
	h, store := lifecycleHandler(t, g)
	key := startFixtureSession(t, h)
	if v := readFixtureState(t, h); v.State != "running" {
		t.Fatal("startup not confirmed")
	}
	// The user stops at the machine. No /stop request is made to the helper.
	g.status = 6
	g.identity = ""
	g.deviceState = &platform.RecentDevice{GoodsID: "fixture-device", Category: "04", WorkStatus: 10}
	if v := readFixtureState(t, New(store, g, "")); v.State != "unknown" {
		t.Fatal("fault was not held for idle verification")
	}
	_, err := store.UpdateUsageSession("fixture-account", "fixture-device", key, func(e *accounts.UsageSession) error {
		e.IdleSince = time.Now().Add(-6 * time.Second)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the original handler too, including its in-memory start intent.
	time.Sleep(3 * time.Second) // Expire the original handler's read cache.
	if v := readFixtureState(t, h); v.State != "finished" {
		t.Fatal("physical stop not completed")
	}
	_ = prepareKey(t, h)
	if g.starts.Load() != 1 || g.stops.Load() != 0 {
		t.Fatal("reconciliation sent a physical command")
	}
}

func TestRemoteStopThenFaultAlsoReconcilesWithoutResending(t *testing.T) {
	g := &fakeLifecycle{status: 1, identity: "fixture-order", support: true}
	h, store := lifecycleHandler(t, g)
	key := startFixtureSession(t, h)
	_ = readFixtureState(t, h)
	if r := apiRequest(t, h, "POST", usagePath+"stop", map[string]any{"session_id": key, "confirm": true}); r.Code != 200 {
		t.Fatal("remote stop failed", r.Body.String())
	}
	g.status = 6
	g.identity = ""
	g.deviceState = &platform.RecentDevice{GoodsID: "fixture-device", Category: "04", WorkStatus: 10}
	if v := readFixtureState(t, New(store, g, "")); v.State != "unknown" {
		t.Fatal("remote stop fault not held for verification")
	}
	_, err := store.UpdateUsageSession("fixture-account", "fixture-device", key, func(e *accounts.UsageSession) error {
		e.IdleSince = time.Now().Add(-6 * time.Second)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := readFixtureState(t, New(store, g, "")); v.State != "finished" {
		t.Fatal("remote stop did not reconcile", v)
	}
	if g.starts.Load() != 1 || g.stops.Load() != 1 {
		t.Fatal("status recovery resent a command")
	}
}

func TestDeviceButtonStopReconcilesAfterRestart(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		observed bool
	}{{"hardware-stop-fault", 6, true}, {"background-missed-running-fault", 6, false}, {"background-missed-terminal", 3, false}, {"background-missed-terminal-5", 5, false}} {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeLifecycle{status: tc.status, category: "08", deviceState: &platform.RecentDevice{GoodsID: "fixture-device", Category: "08", WorkStatus: 10}}
			h, store := lifecycleHandler(t, g)
			_, _ = store.ReserveUsageAttempt("fixture-account", "fixture-device")
			e := accounts.UsageSession{ID: "local-session", Device: "fixture-device", SKU: "fixture-sku", Category: "08", OrderNo: "fixture-order", State: "unknown", Confirmed: true, ObservedRunning: tc.observed, StartedAt: time.Now().Add(-25 * time.Minute)}
			if tc.observed {
				e.SyncID = "original-usage"
			}
			if err := store.CreateUsageSession("fixture-account", "fixture-device", e); err != nil {
				t.Fatal(err)
			}
			first := readFixtureState(t, h)
			if first.State == "finished" || first.CanStop {
				t.Fatal("single idle observation ended session", first)
			}
			// Repeated clicks/cache hits must not turn one observation into two.
			if v := readFixtureState(t, New(store, g, "")); v.State == "finished" {
				t.Fatal("unspaced idle observations ended session")
			}
			_, err := store.UpdateUsageSession("fixture-account", "fixture-device", e.ID, func(e *accounts.UsageSession) error {
				if e.IdleSince.IsZero() {
					t.Fatal("idle confirmation not persisted")
				}
				e.IdleSince = time.Now().Add(-6 * time.Second)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			restarted := New(store, g, "")
			if v := readFixtureState(t, restarted); v.State != "finished" || v.FinishedAt.IsZero() || v.CanStop {
				t.Fatal("hardware stop not reconciled", v)
			}
			if pending, err := store.UsageAttemptPending("fixture-account", "fixture-device"); err != nil || pending {
				t.Fatal("finished session retained duplicate guard", err)
			}
			if v := readFixtureState(t, New(store, g, "")); v.State != "finished" {
				t.Fatal("finished state not persistent")
			}
			if g.starts.Load() != 0 || g.stops.Load() != 0 {
				t.Fatal("read-only reconciliation controlled equipment")
			}
		})
	}
}

func TestDeviceEndRequiresConfirmedMatchingIdleEvidence(t *testing.T) {
	for _, name := range []string{"busy", "unknown-device-status", "wrong-device", "wrong-category", "details-failed", "unconfirmed", "identity-changed", "unbound-identity", "still-running", "startup-propagating"} {
		t.Run(name, func(t *testing.T) {
			g := &fakeLifecycle{status: 6, category: "08", deviceState: &platform.RecentDevice{GoodsID: "fixture-device", Category: "08", WorkStatus: 10}}
			e := accounts.UsageSession{ID: "local-session", Device: "fixture-device", SKU: "fixture-sku", Category: "08", OrderNo: "fixture-order", State: "unknown", Confirmed: true, ObservedRunning: true, SyncID: "original-usage", StartedAt: time.Now().Add(-25 * time.Minute), IdleSince: time.Now().Add(-6 * time.Second)}
			switch name {
			case "busy":
				g.deviceState.WorkStatus = 20
			case "unknown-device-status":
				g.deviceState.WorkStatus = 0
			case "wrong-device":
				g.deviceState.GoodsID = "other-device"
			case "wrong-category":
				g.deviceState.Category = "04"
			case "details-failed":
				g.detailError = true
			case "unconfirmed":
				e.Confirmed = false
				e.StartedAt = time.Now().Add(-90 * time.Second)
				g.unconfirmed = true
			case "identity-changed":
				g.identity = "other-usage"
			case "unbound-identity":
				e.SyncID = ""
				e.ObservedRunning = false
				g.identity = "other-usage"
			case "still-running":
				g.status = 1
			case "startup-propagating":
				e.ObservedRunning = false
				e.SyncID = ""
				e.StartedAt = time.Now()
			}
			h, store := lifecycleHandler(t, g)
			_, _ = store.ReserveUsageAttempt("fixture-account", "fixture-device")
			if err := store.CreateUsageSession("fixture-account", "fixture-device", e); err != nil {
				t.Fatal(err)
			}
			if v := readFixtureState(t, h); v.State == "finished" || v.CanStop {
				t.Fatal("insufficient evidence released session", v)
			}
			saved, _, err := store.GetUsageSession("fixture-account", "fixture-device")
			if err != nil || !saved.IdleSince.IsZero() {
				t.Fatal("contradictory observation did not reset idle confirmation", err)
			}
			if pending, _ := store.UsageAttemptPending("fixture-account", "fixture-device"); !pending {
				t.Fatal("unconfirmed end released guard")
			}
			if g.starts.Load() != 0 || g.stops.Load() != 0 {
				t.Fatal("verification issued a command")
			}
		})
	}
}

func TestOldSessionRecoveryWithMissingDetailStatus(t *testing.T) {
	for _, name := range []string{"expired-unconfirmed", "startup-receipt-timeout", "receipt-expired", "receipt-missing", "confirmed", "recent-unconfirmed", "list-busy", "list-unknown", "list-failed", "list-missing", "detail-busy", "changed-session"} {
		t.Run(name, func(t *testing.T) {
			idle := 10
			g := &fakeLifecycle{status: 6, category: "08", unconfirmed: true, listWorkStatus: &idle,
				deviceState: &platform.RecentDevice{GoodsID: "fixture-device", Category: "08"}}
			e := accounts.UsageSession{ID: "old-session", Device: "fixture-device", SKU: "fixture-sku", Category: "08", OrderNo: "fixture-order", State: "unknown", StartedAt: time.Now().Add(-2 * time.Hour)}
			wantEnd := true
			switch name {
			case "startup-receipt-timeout":
				e.StartedAt = time.Now().Add(-3 * time.Minute)
			case "receipt-expired":
				g.confirmationError = true
			case "receipt-missing":
				e.OrderNo = ""
			case "confirmed":
				e.Confirmed = true
			case "recent-unconfirmed":
				e.StartedAt = time.Now().Add(-90 * time.Second)
				wantEnd = false
			case "list-busy":
				idle = 20
				wantEnd = false
			case "list-unknown":
				idle = 0
				wantEnd = false
			case "list-failed":
				g.listError = true
				wantEnd = false
			case "list-missing":
				g.listMissing = true
				wantEnd = false
			case "detail-busy":
				g.deviceState.WorkStatus = 20
				wantEnd = false
			case "changed-session":
				e.SyncID = "original"
				g.identity = "other"
				wantEnd = false
			}
			h, store := lifecycleHandler(t, g)
			_, _ = store.ReserveUsageAttempt("fixture-account", "fixture-device")
			if err := store.CreateUsageSession("fixture-account", "fixture-device", e); err != nil {
				t.Fatal(err)
			}
			if v := readFixtureState(t, h); v.State != "unknown" {
				t.Fatal("one observation released old record", v)
			}
			saved, _, err := store.GetUsageSession("fixture-account", "fixture-device")
			if err != nil {
				t.Fatal(err)
			}
			if !wantEnd {
				if !saved.IdleSince.IsZero() {
					t.Fatal("contradictory evidence counted as idle")
				}
				if pending, _ := store.UsageAttemptPending("fixture-account", "fixture-device"); !pending {
					t.Fatal("old age alone released guard")
				}
				return
			}
			if saved.IdleSince.IsZero() {
				t.Fatal("fresh latest-used idle was ignored when details omitted workStatus")
			}
			_, err = store.UpdateUsageSession("fixture-account", "fixture-device", e.ID, func(e *accounts.UsageSession) error {
				e.IdleSince = time.Now().Add(-6 * time.Second)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if v := readFixtureState(t, New(store, g, "")); v.State != "finished" {
				t.Fatal("old local record remained locked", v)
			}
			if pending, _ := store.UsageAttemptPending("fixture-account", "fixture-device"); pending {
				t.Fatal("finished old record retained guard")
			}
			if g.starts.Load() != 0 || g.stops.Load() != 0 {
				t.Fatal("old record recovery controlled equipment")
			}
		})
	}
}

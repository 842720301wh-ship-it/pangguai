package server

import (
	"context"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"testing"
)

type fakeScanGateway struct {
	fakeDeviceGateway
	scans int
}

func (g *fakeScanGateway) ScanDevice(_ context.Context, token, text string) (platform.RecentDevice, error) {
	if token != "fixture-token" {
		panic("wrong account token")
	}
	g.scans++
	return platform.RecentDevice{GoodsID: "fixture-scan", Name: "scanned device", Category: "04"}, nil
}
func (g *fakeScanGateway) ScannedDeviceState(context.Context, string, string) (platform.RecentDevice, error) {
	return platform.RecentDevice{GoodsID: "fixture-scan", Name: "scanned device", Category: "04"}, nil
}
func TestScanIsAccountBoundAndPersistedWithoutRawQR(t *testing.T) {
	g := &fakeScanGateway{}
	h, store := testHandler(t, g)
	if err := store.Save(accounts.Record{Profile: accounts.Profile{ID: "fixture"}, Token: "fixture-token"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		body map[string]any
	}{{"/api/accounts/missing/devices/scan", map[string]any{"text": "SN=fixture"}}, {"/api/accounts/fixture/devices/scan?token=other", map[string]any{"text": "SN=fixture"}}, {"/api/accounts/fixture/devices/scan", map[string]any{"text": "SN=a&IMEI=b"}}, {"/api/accounts/fixture/devices/scan", map[string]any{"text": "SN=fixture", "token": "other"}}} {
		if r := apiRequest(t, h, "POST", tc.path, tc.body); r.Code == 200 {
			t.Fatal("override accepted")
		}
	}
	if g.scans != 0 {
		t.Fatal("invalid request reached platform")
	}
	for i := 0; i < 2; i++ {
		if r := apiRequest(t, h, "POST", "/api/accounts/fixture/devices/scan", map[string]any{"text": "SN=fixture"}); r.Code != 200 {
			t.Fatal("scan failed", r.Body.String())
		}
	}
	rec, err := store.Get("fixture")
	if err != nil || len(rec.Devices) != 1 || rec.Devices[0].ID != "fixture-scan" {
		t.Fatal("device not saved idempotently", err)
	}
	// A new successful login must preserve this account's saved devices.
	if err = store.Save(accounts.Record{Profile: rec.Profile, Token: "new-fixture-token"}); err != nil {
		t.Fatal(err)
	}
	rec, _ = store.Get("fixture")
	if len(rec.Devices) != 1 || rec.Token != "new-fixture-token" {
		t.Fatal("login replaced saved devices")
	}
	if g.sent != 0 || g.logins != 0 {
		t.Fatal("scan performed login or SMS")
	}
}

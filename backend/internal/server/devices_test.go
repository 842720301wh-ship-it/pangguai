package server

import (
	"context"
	"strings"
	"testing"

	"panguai/internal/accounts"
	"panguai/internal/platform"
)

type fakeDeviceGateway struct {
	fakeGateway
	deviceReads int
}

func (g *fakeDeviceGateway) DeviceDetails(_ context.Context, token, id string) (platform.Observation, error) {
	if id != "fixture-device" {
		return nil, &platform.Error{Code: "unexpected_device", Message: "unlisted device reached platform"}
	}
	return platform.Observation{}, nil
}

func (g *fakeDeviceGateway) RecentOrders(context.Context, string) ([]platform.Observation, error) {
	return []platform.Observation{}, nil
}

func (g *fakeDeviceGateway) RecentDevices(_ context.Context, token, category string) ([]platform.RecentDevice, error) {
	g.deviceReads++
	if token != "fixture-private-token" {
		return nil, &platform.Error{Code: "test_wrong_identity", Message: "wrong stored identity"}
	}
	return []platform.RecentDevice{{GoodsID: "fixture-device", Name: "测试设备", Category: category}}, nil
}

func TestRecentDeviceRouteUsesSavedAccountOnly(t *testing.T) {
	gateway := &fakeDeviceGateway{}
	handler, store := testHandler(t, gateway)
	if err := store.Save(accounts.Record{Profile: accounts.Profile{ID: "fixture-account"}, Token: "fixture-private-token"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/accounts/missing/devices",
		"/api/accounts/fixture-account/devices?token=another-account",
		"/api/accounts/fixture-account/devices?category=a&category=b",
	} {
		if result := apiRequest(t, handler, "GET", path, nil); result.Code == 200 {
			t.Fatal("unbound account or caller-supplied credential was accepted")
		}
	}
	if gateway.deviceReads != 0 {
		t.Fatal("invalid requests reached the platform")
	}
	response := apiRequest(t, handler, "GET", "/api/accounts/fixture-account/devices", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "fixture-device") || strings.Contains(response.Body.String(), "fixture-private-token") {
		t.Fatal("device response invalid or credential exposed")
	}
	if gateway.deviceReads != 1 || gateway.sent != 0 || gateway.logins != 0 {
		t.Fatal("device query had login or SMS side effects")
	}
	if response := apiRequest(t, handler, "GET", "/api/accounts/fixture-account/devices/unlisted-device", nil); response.Code != 404 {
		t.Fatal("device outside own recent list was accepted")
	}
	if response := apiRequest(t, handler, "GET", "/api/accounts/fixture-account/devices/fixture-device", nil); response.Code != 200 {
		t.Fatal("own listed device read rejected")
	}
	if response := apiRequest(t, handler, "POST", "/api/accounts/fixture-account/devices", map[string]string{}); response.Code == 200 {
		t.Fatal("unexpected mutation method accepted")
	}
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"panguai/internal/accounts"
	"panguai/internal/platform"
)

type fakeUsageGateway struct {
	fakeDeviceGateway
	starts     atomic.Int32
	reads      atomic.Int32
	changed    bool
	startError bool
	enter      chan struct{}
	release    chan struct{}
}

func (g *fakeUsageGateway) RecentDevices(_ context.Context, token, category string) ([]platform.RecentDevice, error) {
	if token != "fixture-private-token" {
		return nil, errors.New("wrong stored identity")
	}
	return []platform.RecentDevice{{GoodsID: "fixture-device", Name: "fixture", Category: "04", WorkStatus: 10}}, nil
}

func (g *fakeUsageGateway) RecentOrders(context.Context, string) ([]platform.Observation, error) {
	return []platform.Observation{{"orderNo": json.RawMessage(`"own-order"`)}}, nil
}
func (g *fakeUsageGateway) StartupStatus(_ context.Context, token, order string) (bool, error) {
	if order != "own-order" || token != "fixture-private-token" {
		return false, errors.New("wrong order identity")
	}
	return true, nil
}
func TestStartupStatusRequiresOwnRecentOrder(t *testing.T) {
	g := &fakeUsageGateway{}
	h := usageHandler(t, g)
	for _, tc := range []struct {
		order string
		code  int
	}{{"own-order", 200}, {"unlisted-order", 404}} {
		r := apiRequest(t, h, "GET", "/api/accounts/fixture-account/orders/"+tc.order+"/start-status", nil)
		if r.Code != tc.code {
			t.Fatal("order membership check failed", r.Code)
		}
	}
	if g.starts.Load() != 0 {
		t.Fatal("state query started equipment")
	}
}

func (g *fakeUsageGateway) UsageOptions(_ context.Context, token, id string) (platform.UsageDevice, error) {
	g.reads.Add(1)
	if id != "fixture-device" || token != "fixture-private-token" {
		return platform.UsageDevice{}, errors.New("wrong account")
	}
	price := `"0.04"`
	if g.changed {
		price = `"0.05"`
	}
	return platform.UsageDevice{GoodsID: "fixture-device", Name: "fixture", Category: "04", IMEI: "fixture-imei", Services: []platform.Observation{{"skuId": json.RawMessage(`"fixture-sku"`), "name": json.RawMessage(`"hot water"`), "price": json.RawMessage(price), "amount": json.RawMessage(`1000`), "unit": json.RawMessage(`"ml"`), "scanAvailable": json.RawMessage(`true`), "billing_unit": json.RawMessage(`"元/L"`), "billing_levels": json.RawMessage(`[{"price":"0.28"}]`)}}}, nil
}
func (g *fakeUsageGateway) StartUsage(_ context.Context, token string, d platform.UsageDevice, sku string, p platform.UsageLocation) (platform.UsageReceipt, error) {
	g.starts.Add(1)
	if token != "fixture-private-token" || string(d.GoodsID) != "fixture-device" || sku != "fixture-sku" || !p.Valid() {
		return platform.UsageReceipt{}, errors.New("wrong context")
	}
	if g.enter != nil {
		close(g.enter)
		<-g.release
	}
	if g.startError {
		return platform.UsageReceipt{}, errors.New("timeout after send")
	}
	return platform.UsageReceipt{OrderNo: "fixture-order"}, nil
}
func usageHandler(t *testing.T, g *fakeUsageGateway) http.Handler {
	t.Helper()
	handler, store := testHandler(t, g)
	if err := store.Save(accounts.Record{Profile: accounts.Profile{ID: "fixture-account"}, Token: "fixture-private-token"}); err != nil {
		t.Fatal(err)
	}
	return handler
}

const usagePath = "/api/accounts/fixture-account/devices/fixture-device/usage/"

func prepareKey(t *testing.T, h http.Handler) string {
	t.Helper()
	r := apiRequest(t, h, "POST", usagePath+"prepare", map[string]any{"sku_id": "fixture-sku", "latitude": 22.5, "longitude": 113.5})
	if r.Code != 200 {
		t.Fatalf("prepare failed %d %s", r.Code, r.Body)
	}
	if strings.Contains(r.Body.String(), "fixture-imei") || strings.Contains(r.Body.String(), "fixture-private-token") {
		t.Fatal("private context exposed")
	}
	var out struct {
		Key string `json:"confirmation_key"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	if out.Key == "" {
		t.Fatal("confirmation missing")
	}
	return out.Key
}

func TestUsageRequiresOwnDeviceAndExplicitConfirmation(t *testing.T) {
	g := &fakeUsageGateway{}
	h := usageHandler(t, g)
	if r := apiRequest(t, h, "GET", usagePath+"options", nil); r.Code != 200 {
		t.Fatal("options unavailable")
	}
	if g.starts.Load() != 0 {
		t.Fatal("read started equipment")
	}
	key := prepareKey(t, h)
	if g.starts.Load() != 0 {
		t.Fatal("preparation started equipment")
	}
	if r := apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": key}); r.Code != 400 {
		t.Fatal("implicit confirmation accepted")
	}
	if r := apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": key, "confirm": true, "price": "0"}); r.Code != 400 {
		t.Fatal("price override accepted")
	}
	if r := apiRequest(t, h, "POST", strings.Replace(usagePath, "fixture-device", "unlisted", 1)+"start", map[string]any{"confirmation_key": key, "confirm": true}); r.Code != 400 {
		t.Fatal("confirmation reused for another device")
	}
	for i := 0; i < 2; i++ {
		if r := apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": key, "confirm": true}); r.Code != 200 || !strings.Contains(r.Body.String(), "accepted") {
			t.Fatal("confirmed start failed")
		}
	}
	if g.starts.Load() != 1 {
		t.Fatal("duplicate start request")
	}
	if r := apiRequest(t, h, "POST", usagePath+"prepare", map[string]any{"sku_id": "fixture-sku", "latitude": 22.5, "longitude": 113.5}); r.Code != 409 {
		t.Fatal("new confirmation bypassed single attempt")
	}
}

func TestChangedFeeAndUnknownResultNeverResend(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed_fee", true: "unknown"}[uncertain], func(t *testing.T) {
			g := &fakeUsageGateway{startError: uncertain}
			h := usageHandler(t, g)
			key := prepareKey(t, h)
			if !uncertain {
				g.changed = true
			}
			r := apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": key, "confirm": true})
			if r.Code != 502 {
				t.Fatal("changed/uncertain result reported success")
			}
			_ = apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": key, "confirm": true})
			want := int32(0)
			if uncertain {
				want = 1
			}
			if g.starts.Load() != want {
				t.Fatal("changed fee started device or uncertain command retried")
			}
		})
	}
}

func TestConcurrentDifferentConfirmationsCannotDoubleStart(t *testing.T) {
	g := &fakeUsageGateway{enter: make(chan struct{}), release: make(chan struct{})}
	h := usageHandler(t, g)
	a, b := prepareKey(t, h), prepareKey(t, h)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		body := strings.NewReader(`{"confirmation_key":"` + a + `","confirm":true}`)
		req := httptest.NewRequest("POST", "http://127.0.0.1:18080"+usagePath+"start", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Panguai-Client", "web")
		h.ServeHTTP(httptest.NewRecorder(), req)
	}()
	<-g.enter
	r := apiRequest(t, h, "POST", usagePath+"start", map[string]any{"confirmation_key": b, "confirm": true})
	close(g.release)
	wg.Wait()
	if r.Code != 409 || g.starts.Load() != 1 {
		t.Fatal("concurrent confirmations sent two commands")
	}
}

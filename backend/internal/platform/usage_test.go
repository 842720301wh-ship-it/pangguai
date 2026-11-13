package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestUsageOptionsPreservesFeeAndIDButDropsPrivateFields(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("goodsId") != "own-device" || r.Form.Get("token") != "fixture-token" {
			t.Fatal("unbound read")
		}
		switch r.URL.Path {
		case "/goods/normal/details":
			_, _ = w.Write([]byte(`{"code":0,"data":{"goodsId":"own-device","name":"fixture","imei":"private-hardware","categoryCode":"04","isBluetooth":0,"payModel":0,"extAttr":{"flowmeter":1,"secret":"private-mode"}}}`))
		case "/goods/normal/skus":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"skuId":9007199254740993,"name":"热水","price":"0.04","amount":1000,"unit":"ml","scanAvailable":true,"attach":0,"token":"private-token","extAttr":"{\"level\":[{\"price\":\"0.28\",\"secret\":\"private-options\"}]}","function":{"secret":"private-function"}}]}`))
		default:
			t.Fatal("unexpected mutation")
		}
	})
	d, err := c.UsageOptions(context.Background(), "fixture-token", "own-device")
	if err != nil || !UsageSupported(d) || len(d.Services) != 1 || ObservationID(d.Services[0], "skuId") != "9007199254740993" {
		t.Fatal("read failed", err)
	}
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw), "private-") || string(d.Services[0]["price"]) != `"0.04"` {
		t.Fatal("private data leaked or fee changed")
	}
	if string(d.Services[0]["billing_unit"]) != `"元/L"` || !strings.Contains(string(d.Services[0]["billing_levels"]), `"0.28"`) {
		t.Fatal("actual metered rate was replaced by raw SKU price")
	}
}

func TestUsageRejectsMissingDeviceMode(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"goodsId":"own-device","categoryCode":"04","imei":"fixture"}}`))
	})
	if _, err := c.UsageOptions(context.Background(), "token", "own-device"); err == nil {
		t.Fatal("missing mode treated as default supported mode")
	}
}

func TestWaterAndShowerUseHeaderMapAndOnlyFixedFormFields(t *testing.T) {
	for _, category := range []string{"04", "08"} {
		t.Run(category, func(t *testing.T) {
			requests := 0
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				want := "/goods/water/unlock"
				if category == "08" {
					want = "/goods/shower/unlock"
				}
				if r.URL.Path != want || r.Header.Get("imei") != "fixture-hardware" || r.Header.Get("categoryCode") != category || r.Header.Get("lat") != "22.5" || r.Header.Get("lng") != "113.5" {
					t.Fatal("wrong startup branch or header map")
				}
				_ = r.ParseForm()
				if len(r.Form) != 3 || r.Form.Get("skuId") != "fixture-sku" || r.Form.Get("promotions") != "[]" || r.Form.Get("token") != "fixture-token" {
					t.Fatal("unexpected billing/device override")
				}
				if r.Header.Get("sign") != Signature(cSecret(), "1.145.0", r.Header.Get("timestamp"), "fixture-token", want) {
					t.Fatal("startup signature changed")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"orderNo":"fixture-number","msgId":9007199254740993}}`))
			})
			d := UsageDevice{Category: category, IMEI: "fixture-hardware", Services: []Observation{{"skuId": json.RawMessage(`"fixture-sku"`), "scanAvailable": json.RawMessage(`true`)}}}
			receipt, err := c.StartUsage(context.Background(), "fixture-token", d, "fixture-sku", UsageLocation{22.5, 113.5})
			if err != nil || receipt.OrderNo != "fixture-number" || receipt.MessageID != "9007199254740993" || requests != 1 {
				t.Fatal("startup receipt failed", err)
			}
		})
	}
}

func cSecret() string { return "fixture-secret" }

func TestStartupStatusIsAnOrderQuery(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path != "/machine/device_is_start" || r.Form.Get("orderNo") != "own-order" || r.Form.Get("token") != "token" || len(r.Form) != 2 {
			t.Fatal("wrong query or device command")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":false}`))
	})
	if started, err := c.StartupStatus(context.Background(), "token", "own-order"); err != nil || started {
		t.Fatal("pending state was treated as started", err)
	}
}

func TestStartDoesNotRetryUncertainOutcomeAndRejectsUnknownBranch(t *testing.T) {
	requests := 0
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	d := UsageDevice{Category: "04", IMEI: "fixture", Services: []Observation{{"skuId": json.RawMessage(`"sku"`), "scanAvailable": json.RawMessage(`true`)}}}
	_, err := c.StartUsage(context.Background(), "token", d, "sku", UsageLocation{22, 113})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != "usage_unconfirmed" || requests != 1 {
		t.Fatal("uncertain result accepted or retried")
	}
	for _, category := range []string{"00", "01", "99"} {
		d.Category = category
		if _, err := c.StartUsage(context.Background(), "token", d, "sku", UsageLocation{22, 113}); err == nil {
			t.Fatal("unknown branch accepted")
		}
	}
	if requests != 1 {
		t.Fatal("unsupported branch reached upstream")
	}
}

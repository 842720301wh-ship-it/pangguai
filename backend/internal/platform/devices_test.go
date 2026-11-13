package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRecentDevicesUsesOwnTokenAndOmitsEmptyCategory(t *testing.T) {
	for _, category := range []string{"", "washer"} {
		t.Run(category, func(t *testing.T) {
			requests := 0
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "POST" || r.URL.Path != "/goods/latestUsed" {
					t.Fatal("unexpected endpoint or device control request")
				}
				_ = r.ParseForm()
				if r.Header.Get("Authorization") != "fixture-token" || r.Form.Get("token") != "fixture-token" {
					t.Fatal("saved account credential was not used")
				}
				if category == "" {
					if r.Form.Has("categoryCode") || len(r.Form) != 1 {
						t.Fatal("empty category should not be sent")
					}
				} else if r.Form.Get("categoryCode") != category || len(r.Form) != 2 {
					t.Fatal("category filter changed")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":[{"goodsId":9007199254740993,"goodsName":"测试设备","orgId":"fixture-org","workStatus":1,"categoryCode":"washer"}]}`))
			})
			devices, err := client.RecentDevices(context.Background(), "fixture-token", category)
			if err != nil || len(devices) != 1 || devices[0].GoodsID != "9007199254740993" || requests != 1 {
				t.Fatal("device identity lost precision or read-only request failed", err)
			}
		})
	}
}

func TestDeviceIDRejectsNonIdentifierJSON(t *testing.T) {
	for _, raw := range []string{`true`, `{}`, `1.5`, `-1`, `1e6`} {
		var id DeviceID
		if json.Unmarshal([]byte(raw), &id) == nil {
			t.Fatalf("invalid identifier accepted: %s", raw)
		}
	}
}

func TestObservationReadsDropCredentialsAndNestedPaymentData(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("token") != "fixture-token" {
			t.Fatal("wrong account credential")
		}
		switch r.URL.Path {
		case "/goods/normal/details":
			if r.Form.Get("goodsId") != "fixture-device" {
				t.Fatal("device filter missing")
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"goodsId":"fixture-device","name":"fixture","isBluetooth":true,"token":"hidden","payment":{"sign":"hidden"},"remainTime":{"sign":"hidden"}}}`))
		case "/order/list":
			if r.Form.Get("page") != "1" || r.Form.Get("pageSize") != "10" || !r.Form.Has("orderStatus") || r.Form.Has("displayTag") {
				t.Fatal("order query differs from observed ViewModel")
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"fixture-order","orderStatus":10,"orderNo":"fixture-number","userId":"hidden","payInfo":{"sign":"hidden"},"token":"hidden"}],"total":1}}`))
		default:
			t.Fatal("unexpected mutation or non-observation endpoint")
		}
	})
	device, err := client.DeviceDetails(context.Background(), "fixture-token", "fixture-device")
	if err != nil {
		t.Fatal(err)
	}
	orders, err := client.RecentOrders(context.Background(), "fixture-token")
	if err != nil || len(orders) != 1 {
		t.Fatal("order read failed", err)
	}
	raw, _ := json.Marshal([]any{device, orders})
	if strings.Contains(string(raw), "hidden") || strings.Contains(string(raw), "payment") || strings.Contains(string(raw), "userId") {
		t.Fatal("non-observation information exposed")
	}
}

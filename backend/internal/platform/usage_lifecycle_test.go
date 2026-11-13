package platform

import (
	"context"
	"net/http"
	"testing"
)

func TestUsageReadAndEndWireContracts(t *testing.T) {
	for _, category := range []string{"04", "08"} {
		paths := []string{}
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			_ = r.ParseForm()
			if r.Form.Get("token") != "fixture-token" || r.Header.Get("Authorization") != "fixture-token" {
				t.Fatal("wrong account")
			}
			switch r.URL.Path {
			case "//goods/water/sync", "/goods/shower/sync":
				if r.Form.Get("skuId") != "9007199254740993" || len(r.Form) != 2 {
					t.Fatal("sync form altered")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"status":1,"workStatus":2,"identify":9007199254740993,"amount":12.34}}`))
			case "/machine/msg/sync":
				if r.Form.Get("msgId") != "fixture-message" {
					t.Fatal("message override")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"state":"ok","code":"00"}}`))
			case "/goods/endWater":
				if category != "04" || r.Form.Get("goodsId") != "fixture-device" || len(r.Form) != 2 {
					t.Fatal("water end form changed")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			case "/shower/goods/remoteEnd":
				if category != "08" || r.Form.Get("goodsId") != "fixture-device" || r.Form.Get("orderNo") != "fixture-order" || len(r.Form) != 3 {
					t.Fatal("shower end form changed")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			default:
				t.Fatal("unexpected endpoint or payment")
			}
		})
		ctx := context.Background()
		live, err := client.SyncUsage(ctx, "fixture-token", category, "9007199254740993")
		if err != nil || live.Identify != "9007199254740993" || live.Terminal() {
			t.Fatal("identifier precision or state lost", err)
		}
		if ok, err := client.UsageCommandConfirmed(ctx, "fixture-token", "fixture-message"); err != nil || !ok {
			t.Fatal("message confirmation failed", err)
		}
		if err := client.EndUsage(ctx, "fixture-token", category, "fixture-device", "fixture-order"); err != nil {
			t.Fatal("end acknowledgement failed", err)
		}
		if len(paths) != 3 {
			t.Fatal("automatic retry or extra request")
		}
	}
}

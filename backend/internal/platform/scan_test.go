package platform

import (
	"context"
	"net/http"
	"testing"
)

func TestQRParsingRejectsAmbiguityAndNeverFetchesQRHost(t *testing.T) {
	for _, text := range []string{"", "https://example.test/?IMEI=a&IMEI=b", "SN=a&IMEI=b", "https://example.test/?SN=", "javascript:alert(1)", "https://example.test/?IMEI=%0A", "https://example.test/?SN=%ZZ"} {
		if _, _, err := ParseDeviceQR(text); err == nil {
			t.Errorf("invalid QR accepted: %q", text)
		}
	}
	for _, key := range []string{"NQT", "IMEI", "SN"} {
		calls := 0
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Host != "userapi.qiekj.com" || r.Method != "POST" {
				t.Fatal("QR host or external flow requested")
			}
			_ = r.ParseForm()
			if r.Form.Get("token") != "fixture-token" {
				t.Fatal("stored token missing")
			}
			switch r.URL.Path {
			case "/goods/scan/v2":
				if r.Form.Get(key) != "fixture-code" || len(r.Form) != 2 {
					t.Fatal("QR form mismatch")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"id":9007199254740993}}`))
			case "/goods/normal/details":
				if r.Form.Get("goodsId") != "9007199254740993" {
					t.Fatal("device precision lost")
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"goodsId":"9007199254740993","name":"fixture","categoryCode":"04","token":"private","imei":"private"}}`))
			default:
				t.Fatal("unexpected call, possibly a device command")
			}
		})
		d, err := client.ScanDevice(context.Background(), "fixture-token", "http://untrusted.test/?"+key+"=fixture-code")
		if err != nil || d.GoodsID != "9007199254740993" || d.WorkStatus != 0 || calls != 2 {
			t.Fatal("scan resolution or unknown state failed", err)
		}
	}
}

func TestScanExternalFlowAndMismatchedDetailsAreRejected(t *testing.T) {
	for _, external := range []bool{true, false} {
		calls := 0
		client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path == "/goods/scan/v2" {
				if external {
					_, _ = w.Write([]byte(`{"code":0,"data":{"id":"fixture","jumpToDevicePageFlag":true}}`))
				} else {
					_, _ = w.Write([]byte(`{"code":0,"data":{"id":"fixture"}}`))
				}
			} else {
				_, _ = w.Write([]byte(`{"code":0,"data":{"goodsId":"other","name":"other","categoryCode":"04"}}`))
			}
		})
		if _, err := client.ScanDevice(context.Background(), "fixture-token", "SN=fixture"); err == nil {
			t.Fatal("unverified device accepted")
		}
		if external && calls != 1 {
			t.Fatal("redirect followed")
		}
	}
}

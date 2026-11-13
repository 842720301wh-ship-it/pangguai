package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTicketBalancesKeepOrganizationsAndDecimalAmountsSeparate(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path != "/tokenCoin/userTokenCoins" || r.Form.Get("page") != "1" || r.Form.Get("pageSize") != "20" || r.Form.Get("token") != "fixture-token" || len(r.Form) != 3 {
			t.Fatal("unexpected wallet read or mutation")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"shopId":9007199254740993,"shopName":"测试校区甲","totalAmount":"43.49","principalAmount":"43.49","presentAmount":"0","userId":"private-user","phone":"private-phone"},{"shopId":"fixture-b","shopName":"测试校区乙","totalAmount":29.7,"principalAmount":"29.7","presentAmount":"0.00"}],"total":2}}`))
	})
	page, err := c.TicketBalances(context.Background(), "fixture-token")
	if err != nil || len(page.Items) != 2 || page.Items[0].Total != "43.49" || page.Items[1].Total != "29.7" || page.Items[0].ShopID != "9007199254740993" {
		t.Fatal("wallet identity or precision changed", err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "private-") {
		t.Fatal("private account data exposed")
	}
}
func TestMissingWalletAmountIsNotZero(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"shopId":"fixture","shopName":"测试"}]}}`))
	})
	if _, err := c.TicketBalances(context.Background(), "token"); err == nil {
		t.Fatal("absent amount displayed as zero")
	}
}

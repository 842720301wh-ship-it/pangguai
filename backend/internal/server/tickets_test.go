package server

import (
	"context"
	"panguai/internal/accounts"
	"panguai/internal/platform"
	"strings"
	"testing"
)

type fakeTicketGateway struct {
	fakeGateway
	reads int
}

func (g *fakeTicketGateway) TicketBalances(_ context.Context, token string) (platform.TicketPage, error) {
	g.reads++
	if token != "fixture-token" {
		return platform.TicketPage{}, &platform.Error{Code: "wrong_identity", Message: "wrong identity"}
	}
	return platform.TicketPage{Items: []platform.TicketBalance{{ShopID: "fixture-shop", ShopName: "测试", Total: "4349"}}, Total: 1}, nil
}
func TestTicketRouteBindsSavedAccountAndRejectsOverrides(t *testing.T) {
	g := &fakeTicketGateway{}
	h, st := testHandler(t, g)
	if err := st.Save(accounts.Record{Profile: accounts.Profile{ID: "fixture"}, Token: "fixture-token"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/accounts/missing/tickets", "/api/accounts/fixture/tickets?token=other", "/api/accounts/fixture/tickets?page=2"} {
		if r := apiRequest(t, h, "GET", p, nil); r.Code == 200 {
			t.Fatal("wallet override accepted")
		}
	}
	if g.reads != 0 {
		t.Fatal("invalid query reached platform")
	}
	if r := apiRequest(t, h, "GET", "/api/accounts/fixture/tickets", nil); r.Code != 200 {
		t.Fatal("own wallet read failed")
	} else if !strings.Contains(r.Body.String(), `"amount_unit":"fen"`) || !strings.Contains(r.Body.String(), `"currency":"CNY"`) || !strings.Contains(r.Body.String(), `"totalAmount":"4349"`) {
		t.Fatal("wallet amount unit contract missing")
	}
	if g.reads != 1 || g.sent != 0 || g.logins != 0 {
		t.Fatal("wallet query had login or SMS side effects")
	}
}

package server

import (
	"context"
	"net/http"
	"panguai/internal/platform"
)

type ticketGateway interface {
	TicketBalances(context.Context, string) (platform.TicketPage, error)
}

func (s *Server) tickets(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatform(w) {
		return
	}
	g, ok := s.platform.(ticketGateway)
	if !ok {
		writeError(w, 503, "tickets_pending", "小票查询尚未配置")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, 400, "invalid_input", "小票只查询当前本机账号")
		return
	}
	record, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "account_missing", "本机未保存该账号")
		return
	}
	result, err := g.TicketBalances(r.Context(), record.Token)
	if err != nil {
		upstreamError(w, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"wallets": result.Items, "amount_unit": "fen", "currency": "CNY", "has_more": result.Total > len(result.Items), "mode": "readonly", "source": "/tokenCoin/userTokenCoins"})
}

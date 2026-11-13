package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"regexp"
)

// Amount retains the platform's decimal text. Wallets are not summed or
// converted through float64, and absent amounts must not appear as zero.
type Amount string

var decimalAmount = regexp.MustCompile(`^-?[0-9]{1,18}(\.[0-9]{1,8})?$`)

func (a *Amount) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		text = string(bytes.TrimSpace(raw))
	}
	if !decimalAmount.MatchString(text) {
		return &Error{Code: "ticket_data_invalid", Message: "小票金额格式无法确认", UpstreamPath: "/tokenCoin/userTokenCoins"}
	}
	*a = Amount(text)
	return nil
}

type TicketBalance struct {
	ShopID    DeviceID `json:"shopId"`
	ShopName  string   `json:"shopName"`
	Total     Amount   `json:"totalAmount"`
	Principal Amount   `json:"principalAmount"`
	Present   Amount   `json:"presentAmount"`
}
type TicketPage struct {
	Items []TicketBalance `json:"items"`
	Total int             `json:"total"`
}

func (c *Client) TicketBalances(ctx context.Context, token string) (TicketPage, error) {
	if token == "" {
		return TicketPage{}, &Error{Code: "credential_missing", Message: "账号凭据为空"}
	}
	var page TicketPage
	// TicketViewModel.getTicket passes page and pageSize to ticketList.
	if err := c.post(ctx, "/tokenCoin/userTokenCoins", token, url.Values{"page": {"1"}, "pageSize": {"20"}}, &page); err != nil {
		return TicketPage{}, err
	}
	for _, item := range page.Items {
		if item.ShopID == "" || item.ShopName == "" || item.Total == "" {
			return TicketPage{}, &Error{Code: "ticket_data_invalid", Message: "平台小票归属或余额不完整", UpstreamPath: "/tokenCoin/userTokenCoins"}
		}
	}
	if page.Items == nil {
		page.Items = []TicketBalance{}
	}
	return page, nil
}

package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

type UsageSync struct {
	Status     *int     `json:"status"`
	WorkStatus *int     `json:"workStatus"`
	Identify   DeviceID `json:"identify"`
	// The official page tests amount > 0 only after terminal status. Its unit
	// and whether it is live money are NOT inferred from the field name.
	Amount json.RawMessage `json:"amount"`
}

func (s UsageSync) Terminal() bool { return s.Status != nil && (*s.Status == 3 || *s.Status == 5) }
func (s UsageSync) Fault() bool    { return s.Status != nil && *s.Status == 6 }
func (c *Client) SyncUsage(ctx context.Context, token, category, sku string) (UsageSync, error) {
	path := "//goods/water/sync" // Exact Retrofit path; the double slash participates in signing.
	if category == "08" {
		path = "/goods/shower/sync"
	} else if category != "04" {
		return UsageSync{}, &Error{Code: "usage_unsupported", Message: "暂不支持该设备"}
	}
	var state UsageSync
	if err := c.post(ctx, path, token, url.Values{"skuId": {sku}}, &state); err != nil {
		return state, err
	}
	if state.Status == nil {
		return state, &Error{Code: "usage_state_invalid", Message: "平台使用状态不完整"}
	}
	return state, nil
}
func (c *Client) UsageCommandConfirmed(ctx context.Context, token, id string) (bool, error) {
	var state struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := c.post(ctx, "/machine/msg/sync", token, url.Values{"msgId": {id}}, &state); err != nil {
		return false, err
	}
	return strings.EqualFold(state.State, "OK") || state.Code == "05", nil
}

type UsageControl struct {
	GoodsID   DeviceID `json:"goodsId"`
	Category  string   `json:"categoryCode"`
	Bluetooth int      `json:"isBluetooth"`
	CanFinish bool     `json:"supportAppUserFinish"`
}

func (c *Client) UsageControl(ctx context.Context, token, id string) (UsageControl, error) {
	var info UsageControl
	err := c.post(ctx, "/goods/normal/details", token, url.Values{"goodsId": {id}}, &info)
	if err == nil && string(info.GoodsID) != id {
		err = &Error{Code: "device_changed", Message: "设备详情不匹配"}
	}
	return info, err
}
func (c *Client) EndUsage(ctx context.Context, token, category, device, order string) error {
	path := "/goods/endWater"
	form := url.Values{"goodsId": {device}}
	if category == "08" {
		path = "/shower/goods/remoteEnd"
		if order != "" {
			form.Set("orderNo", order)
		}
	} else if category != "04" {
		return &Error{Code: "usage_unsupported", Message: "暂不支持该设备"}
	}
	// An accepted request is followed by read-only sync; never reported as stopped here.
	return c.post(ctx, path, token, form, nil)
}

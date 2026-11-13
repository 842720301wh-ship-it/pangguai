package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"regexp"
)

// DeviceID preserves opaque string identifiers and JSON integer identifiers
// without converting them through float64. Some Gson String DTOs accept both.
type DeviceID string

var integerID = regexp.MustCompile(`^[0-9]{1,40}$`)

func (id *DeviceID) UnmarshalJSON(raw []byte) error {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		*id = DeviceID(text)
		return nil
	}
	if integerID.Match(raw) {
		*id = DeviceID(raw)
		return nil
	}
	return &Error{Code: "device_data_invalid", Message: "设备标识格式与已知协议不一致", UpstreamPath: "/goods/latestUsed"}
}

// RecentDevice contains only fields declared in LatestUsedDeviceBean.
// No caller-supplied identity, price or command is accepted here.
type RecentDevice struct {
	GoodsID         DeviceID `json:"goodsId"`
	Name            string   `json:"goodsName"`
	Category        string   `json:"categoryCode"`
	WorkStatus      int      `json:"workStatus"`
	OrganizationID  DeviceID `json:"orgId"`
	Organization    string   `json:"orgName"`
	ParentPointID   int      `json:"parentPointId"`
	ParentPointName string   `json:"parentPointName"`
	ChildPointID    DeviceID `json:"childPointId"`
	ChildPointName  string   `json:"childPointName"`
}

func (c *Client) RecentDevices(ctx context.Context, token, category string) ([]RecentDevice, error) {
	if token == "" {
		return nil, &Error{Code: "credential_missing", Message: "账号凭据为空"}
	}
	form := url.Values{}
	// NearbyShopViewModel.getLatestUsedDevice omits an empty categoryCode.
	if category != "" {
		form.Set("categoryCode", category)
	}
	var result []RecentDevice
	if err := c.post(ctx, "/goods/latestUsed", token, form, &result); err != nil {
		return nil, err
	}
	for _, device := range result {
		if device.GoodsID == "" {
			return nil, &Error{Code: "device_data_invalid", Message: "平台未返回可确认的设备标识", UpstreamPath: "/goods/latestUsed"}
		}
	}
	return result, nil
}

// Observation exposes only named scalar state fields, never nested payment,
// Bluetooth key, contact, or credential objects from platform responses.
type Observation map[string]json.RawMessage

func observation(raw map[string]json.RawMessage, names ...string) Observation {
	result := Observation{}
	for _, name := range names {
		value := bytes.TrimSpace(raw[name])
		if len(value) == 0 || value[0] == '{' || value[0] == '[' {
			continue
		}
		result[name] = value
	}
	return result
}

func (c *Client) DeviceDetails(ctx context.Context, token, goodsID string) (Observation, error) {
	var raw map[string]json.RawMessage
	if err := c.post(ctx, "/goods/normal/details", token, url.Values{"goodsId": {goodsID}}, &raw); err != nil {
		return nil, err
	}
	return observation(raw, "goodsId", "machineId", "name", "categoryCode", "orgName", "isBluetooth", "isReserve", "payModel", "remainTime", "supportAppUserFinish", "deviceErrorCode", "deviceErrorMsg"), nil
}

func (c *Client) RecentOrders(ctx context.Context, token string) ([]Observation, error) {
	var page struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	// OrderDetailsViewModel includes page/pageSize/orderStatus, and omits the
	// all-category displayTag "0". Only the first ten own orders are observed.
	form := url.Values{"page": {"1"}, "pageSize": {"10"}, "orderStatus": {""}}
	if err := c.post(ctx, "/order/list", token, form, &page); err != nil {
		return nil, err
	}
	result := make([]Observation, 0, len(page.Items))
	for _, item := range page.Items {
		result = append(result, observation(item, "id", "orderNo", "machineId", "machineName", "machineFunctionId", "machineFunctionName", "orderStatus", "orderType", "payPrice", "createTime", "payTime", "isBluetooth"))
	}
	return result, nil
}

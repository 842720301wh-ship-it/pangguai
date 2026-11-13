package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
)

// UsageDevice keeps hardware identity inside the backend. Billing fields retain
// their upstream representation; the client never supplies an amount or price.
type UsageDevice struct {
	GoodsID       DeviceID            `json:"goods_id"`
	Name          string              `json:"name"`
	Category      string              `json:"category"`
	IMEI          string              `json:"-"`
	Bluetooth     bool                `json:"bluetooth"`
	PayModel      int                 `json:"pay_model"`
	Services      []Observation       `json:"services"`
	BillingHashes map[string][32]byte `json:"-"`
}

type UsageLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (p UsageLocation) Valid() bool {
	return !math.IsNaN(p.Latitude) && !math.IsNaN(p.Longitude) &&
		!math.IsInf(p.Latitude, 0) && !math.IsInf(p.Longitude, 0) &&
		p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180 &&
		(p.Latitude != 0 || p.Longitude != 0)
}

func ObservationID(o Observation, key string) string {
	var id DeviceID
	if json.Unmarshal(o[key], &id) != nil {
		return ""
	}
	return string(id)
}

func UsageSupported(d UsageDevice) bool {
	return (d.Category == "04" || d.Category == "08") && !d.Bluetooth && d.PayModel == 0 && d.IMEI != ""
}

// UsageOptions is read-only: ScanCodeViewModel.skus sends only goodsId. The
// release API returns code=0/data=[Sku], verified for the own water/shower devices.
func (c *Client) UsageOptions(ctx context.Context, token, goodsID string) (UsageDevice, error) {
	var detail struct {
		GoodsID   DeviceID        `json:"goodsId"`
		Name      string          `json:"name"`
		Category  string          `json:"categoryCode"`
		IMEI      string          `json:"imei"`
		Bluetooth *int            `json:"isBluetooth"`
		PayModel  *int            `json:"payModel"`
		ExtAttr   json.RawMessage `json:"extAttr"`
	}
	if err := c.post(ctx, "/goods/normal/details", token, url.Values{"goodsId": {goodsID}}, &detail); err != nil {
		return UsageDevice{}, err
	}
	if string(detail.GoodsID) != goodsID || detail.Category == "" || detail.Bluetooth == nil || detail.PayModel == nil {
		return UsageDevice{}, &Error{Code: "device_data_invalid", Message: "平台设备详情不匹配", UpstreamPath: "/goods/normal/details"}
	}
	var skus []map[string]json.RawMessage
	if err := c.post(ctx, "/goods/normal/skus", token, url.Values{"goodsId": {goodsID}}, &skus); err != nil {
		return UsageDevice{}, err
	}
	services := make([]Observation, 0, len(skus))
	billingHashes := map[string][32]byte{}
	seen := map[string]bool{}
	for _, raw := range skus {
		sku := observation(raw, "skuId", "name", "price", "amount", "unit", "feature", "discount", "discountPrice", "vipDiscount", "vipDiscountPrice", "scanAvailable", "defaultModel", "attach")
		id := ObservationID(sku, "skuId")
		if id == "" || seen[id] {
			return UsageDevice{}, &Error{Code: "service_data_invalid", Message: "平台服务标识缺失或重复", UpstreamPath: "/goods/normal/skus"}
		}
		seen[id] = true
		// JavaScript cannot retain every JSON integer identifier exactly.
		// Expose the already-validated opaque identifier as a string.
		sku["skuId"], _ = json.Marshal(id)
		billingHashes[id] = sha256.Sum256(append(append([]byte(nil), detail.ExtAttr...), raw["extAttr"]...))
		// WaterSkuAdapter reads extAttr.level[].price and a separate device
		// flowmeter flag. Sku.unit alone is NOT a physical measurement label.
		var deviceExt struct {
			Flowmeter *int `json:"flowmeter"`
		}
		_ = json.Unmarshal(jsonAttribute(detail.ExtAttr), &deviceExt)
		var skuExt struct {
			Level []map[string]json.RawMessage `json:"level"`
		}
		_ = json.Unmarshal(jsonAttribute(raw["extAttr"]), &skuExt)
		levels := []Observation{}
		for _, level := range skuExt.Level {
			var price json.Number
			if json.Unmarshal(level["price"], &price) == nil && price != "" {
				levels = append(levels, Observation{"price": level["price"]})
			}
		}
		if deviceExt.Flowmeter != nil && len(levels) > 0 {
			unit := "元/秒"
			if *deviceExt.Flowmeter == 1 {
				unit = "元/L"
			}
			sku["billing_unit"], _ = json.Marshal(unit)
			sku["billing_levels"], _ = json.Marshal(levels)
		}
		services = append(services, sku)
	}
	return UsageDevice{GoodsID: detail.GoodsID, Name: detail.Name, Category: detail.Category, IMEI: detail.IMEI, Bluetooth: *detail.Bluetooth != 0, PayModel: *detail.PayModel, Services: services, BillingHashes: billingHashes}, nil
}

func jsonAttribute(raw json.RawMessage) []byte {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []byte(text)
	}
	return raw
}

// StartupStatus only queries an existing order. It sends no device command.
func (c *Client) StartupStatus(ctx context.Context, token, orderNo string) (bool, error) {
	if token == "" || orderNo == "" {
		return false, &Error{Code: "order_missing", Message: "缺少本人订单标识"}
	}
	var started bool
	if err := c.post(ctx, "/machine/device_is_start", token, url.Values{"orderNo": {orderNo}}, &started); err != nil {
		return false, err
	}
	return started, nil
}

type UsageReceipt struct {
	OrderNo   string `json:"order_no,omitempty"`
	MessageID string `json:"-"`
}

// StartUsage is called only after a local, single-use user confirmation. An
// accepted unlock command is not proof that the physical device has started.
func (c *Client) StartUsage(ctx context.Context, token string, device UsageDevice, skuID string, location UsageLocation) (UsageReceipt, error) {
	if token == "" || !UsageSupported(device) || !location.Valid() {
		return UsageReceipt{}, &Error{Code: "usage_unsupported", Message: "设备模式或实际位置信息不符合已核对的启动流程"}
	}
	found := false
	for _, sku := range device.Services {
		if ObservationID(sku, "skuId") == skuID && string(sku["scanAvailable"]) == "true" {
			found = true
		}
	}
	if !found {
		return UsageReceipt{}, &Error{Code: "service_unavailable", Message: "选定服务不属于当前设备或暂不可用"}
	}
	path := "/goods/water/unlock"
	if device.Category == "08" {
		path = "/goods/shower/unlock"
	}
	headers := map[string]string{"imei": device.IMEI, "categoryCode": device.Category, "lat": strconv.FormatFloat(location.Latitude, 'f', -1, 64), "lng": strconv.FormatFloat(location.Longitude, 'f', -1, 64)}
	var raw struct {
		OrderNo   string   `json:"orderNo"`
		MessageID DeviceID `json:"msgId"`
	}
	// DeviceStartupVm passes these as HeaderMap + FieldMap, not one form.
	// No optional payType/iotBOrderNo/promotion override is accepted from callers.
	err := c.postWithHeaders(ctx, path, token, url.Values{"skuId": {skuID}, "promotions": {"[]"}}, headers, &raw)
	if err != nil {
		return UsageReceipt{}, err
	}
	if raw.OrderNo == "" && raw.MessageID == "" {
		return UsageReceipt{}, &Error{Code: "usage_unconfirmed", Message: "平台未返回启动回执，请在官方 App 核对设备状态", UpstreamPath: path}
	}
	return UsageReceipt{OrderNo: raw.OrderNo, MessageID: string(raw.MessageID)}, nil
}

package platform

import (
	"context"
	"net/url"
	"strings"
	"unicode"
)

// ParseDeviceQR extracts identifiers only. The QR address is never requested.
func ParseDeviceQR(text string) (string, string, error) {
	invalid := &Error{Code: "qr_invalid", Message: "请扫描胖乖设备上的二维码"}
	if len(text) == 0 || len(text) > 2048 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", "", invalid
	}
	var query string
	if strings.HasPrefix(text, "NQT=") || strings.HasPrefix(text, "IMEI=") || strings.HasPrefix(text, "SN=") {
		query = text
	} else {
		u, err := url.Parse(text)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.Fragment != "" {
			return "", "", invalid
		}
		query = u.RawQuery
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return "", "", invalid
	}
	key, value := "", ""
	for _, name := range []string{"NQT", "IMEI", "SN"} {
		if entries, ok := values[name]; ok {
			if key != "" || len(entries) != 1 || entries[0] == "" || len(entries[0]) > 256 || strings.IndexFunc(entries[0], func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
				return "", "", invalid
			}
			key, value = name, entries[0]
		}
	}
	if key == "" {
		return "", "", invalid
	}
	return key, value, nil
}

func (c *Client) ScanDevice(ctx context.Context, token, text string) (RecentDevice, error) {
	key, value, err := ParseDeviceQR(text)
	if err != nil {
		return RecentDevice{}, err
	}
	var result struct {
		ID             DeviceID `json:"id"`
		Alipay         bool     `json:"jumpToAlipayFlag"`
		Alipay2        bool     `json:"jumpToAlipayFlag2"`
		ExternalDevice bool     `json:"jumpToDevicePageFlag"`
	}
	if err = c.post(ctx, "/goods/scan/v2", token, url.Values{key: {value}}, &result); err != nil {
		return RecentDevice{}, err
	}
	if result.Alipay || result.Alipay2 || result.ExternalDevice {
		return RecentDevice{}, &Error{Code: "qr_external_flow", Message: "该二维码需要官方 App 或支付宝流程，请在官方 App 使用"}
	}
	if result.ID == "" || len(result.ID) > 128 {
		return RecentDevice{}, &Error{Code: "qr_not_device", Message: "二维码未返回可确认的设备"}
	}
	return c.ScannedDeviceState(ctx, token, string(result.ID))
}

// Missing workStatus is unknown (0), never presumed idle.
func (c *Client) ScannedDeviceState(ctx context.Context, token, id string) (RecentDevice, error) {
	var detail struct {
		ID       DeviceID `json:"goodsId"`
		Name     string   `json:"name"`
		Category string   `json:"categoryCode"`
		Status   int      `json:"workStatus"`
		OrgID    DeviceID `json:"orgId"`
		Org      string   `json:"orgName"`
	}
	if err := c.post(ctx, "/goods/normal/details", token, url.Values{"goodsId": {id}}, &detail); err != nil {
		return RecentDevice{}, err
	}
	if string(detail.ID) != id || detail.Name == "" || detail.Category == "" {
		return RecentDevice{}, &Error{Code: "qr_not_device", Message: "二维码设备详情不匹配"}
	}
	return RecentDevice{GoodsID: detail.ID, Name: detail.Name, Category: detail.Category, WorkStatus: detail.Status, OrganizationID: detail.OrgID, Organization: detail.Org}, nil
}

package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// WechatLogin accepts a real, user-authorized OAuth credential. A caller-supplied
// unionId alone is never treated as evidence of account ownership.
func (c *Client) WechatLogin(ctx context.Context, accessToken, openID string) (LoginResult, error) {
	if strings.TrimSpace(accessToken) == "" || strings.TrimSpace(openID) == "" || len(accessToken) > 2048 || len(openID) > 256 {
		return LoginResult{}, &Error{Code: "wechat_credential_invalid", Message: "需要本次微信授权返回的有效凭据"}
	}
	query := url.Values{"access_token": {accessToken}, "openid": {openID}, "lang": {"zh_CN"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.weixin.qq.com/sns/userinfo?"+query.Encode(), nil)
	if err != nil {
		return LoginResult{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return LoginResult{}, &Error{Code: "wechat_network", Message: "微信授权身份校验未完成，请稍后重试"}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 || response.StatusCode != 200 {
		return LoginResult{}, &Error{Code: "wechat_response", Message: "无法确认微信授权响应"}
	}
	var identity struct {
		ErrorCode int    `json:"errcode"`
		OpenID    string `json:"openid"`
		UnionID   string `json:"unionid"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.ErrorCode != 0 || identity.OpenID != openID || identity.UnionID == "" {
		return LoginResult{}, &Error{Code: "wechat_authorization_invalid", Message: "微信授权已失效或身份不完整，请重新在官方 App 授权"}
	}
	var result LoginResult
	if err := c.post(ctx, "/wechat/unionId/login", "", url.Values{"unionId": {identity.UnionID}}, &result); err != nil {
		return LoginResult{}, err
	}
	if result.ForcedBind || result.Token == "" {
		return LoginResult{}, &Error{Code: "wechat_binding_required", Message: "平台要求绑定手机号或补充验证，请在官方 App 完成；未自动发送短信", UpstreamPath: "/wechat/unionId/login"}
	}
	return result, nil
}

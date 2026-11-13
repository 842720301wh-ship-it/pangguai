package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const Origin = "https://userapi.qiekj.com"

type Config struct {
	AppSecret  string `json:"app_secret"`
	Version    string `json:"version"`
	DeviceID   string `json:"device_id"`
	PhoneBrand string `json:"phone_brand"`
	ProxyURL   string `json:"proxy_url,omitempty"`
}

type Error struct {
	Code         string
	Message      string
	UpstreamPath string
}

func (e *Error) Error() string { return e.Message }

type Client struct {
	config Config
	http   *http.Client
	origin string
}

func Load(path string) (*Client, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return New(cfg)
}

func New(cfg Config) (*Client, error) {
	if cfg.AppSecret == "" || cfg.Version == "" || cfg.DeviceID == "" {
		return nil, errors.New("平台配置不完整")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.ProxyURL != "" {
		proxy, err := url.Parse(cfg.ProxyURL)
		if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") {
			return nil, errors.New("本客户端的上游代理地址无效")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &Client{config: cfg, origin: Origin, http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Signature matches HeadInterceptor.intercept and byte2HexSHA in App 1.145.0.
// The encoded URL path participates in the signature; form values do not.
func Signature(secret, version, timestamp, token, path string) string {
	text := "appSecret=" + secret + "&channel=android_app&timestamp=" + timestamp + "&token=" + token + "&version=" + version + "&" + path
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

type envelope struct {
	Code    *int            `json:"code"`
	Message string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) post(ctx context.Context, path, token string, form url.Values, result any) error {
	return c.postWithHeaders(ctx, path, token, form, nil, result)
}

func (c *Client) postWithHeaders(ctx context.Context, path, token string, form url.Values, headers map[string]string, result any) (err error) {
	defer func() {
		var failure *Error
		if errors.As(err, &failure) && failure.UpstreamPath == "" {
			failure.UpstreamPath = path
		}
	}()
	values := url.Values{}
	for key, entries := range form {
		values[key] = append([]string(nil), entries...)
	}
	if token != "" {
		values.Set("token", token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Authorization", token)
	req.Header.Set("Version", c.config.Version)
	req.Header.Set("channel", "android_app")
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("sign", Signature(c.config.AppSecret, c.config.Version, timestamp, token, path))
	req.Header.Set("deviceId", c.config.DeviceID)
	req.Header.Set("phoneBrand", c.config.PhoneBrand)
	req.Header.Set("User-Agent", "okhttp/4.12.0")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Code: "upstream_network", Message: "无法连接胖乖生活服务，请检查电脑现有网络；请求未自动重试"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &Error{Code: "upstream_http", Message: fmt.Sprintf("平台返回 HTTP %d，请稍后重试", resp.StatusCode)}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return &Error{Code: "upstream_response", Message: "平台响应读取失败"}
	}
	var reply envelope
	if err := json.Unmarshal(raw, &reply); err != nil || reply.Code == nil {
		return &Error{Code: "upstream_response", Message: "平台响应格式与已知协议不一致"}
	}
	if *reply.Code != 0 {
		message := reply.Message
		if message == "" {
			message = "平台拒绝本次请求"
		}
		if len(message) > 300 {
			message = "平台拒绝本次请求，请在官方 App 中检查登录状态"
		}
		for _, sensitive := range []string{token, values.Get("phone"), values.Get("verify"), values.Get("unionId")} {
			if sensitive != "" {
				message = strings.ReplaceAll(message, sensitive, "[已隐藏]")
			}
		}
		for _, sensitive := range headers {
			if sensitive != "" {
				message = strings.ReplaceAll(message, sensitive, "[已隐藏]")
			}
		}
		message = strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
		return &Error{Code: fmt.Sprintf("platform_%d", *reply.Code), Message: message}
	}
	if result != nil {
		if len(reply.Data) == 0 || string(reply.Data) == "null" || json.Unmarshal(reply.Data, result) != nil {
			return &Error{Code: "upstream_data", Message: "平台返回的数据不完整，未确认操作成功"}
		}
	}
	return nil
}

func (c *Client) SendSMS(ctx context.Context, phone string) error {
	// PhoneLoginActivity calls sendCode(phone, "reg"). Its success observer
	// ignores the response data and opens AuthCodeActivity when code == 0.
	return c.post(ctx, "/common/sms/sendCode", "", url.Values{"phone": {phone}, "template": {"reg"}}, nil)
}

type User struct {
	ID       string `json:"id,omitempty"`
	Nickname string `json:"nickname"`
	Phone    string `json:"phone"`
}
type LoginResult struct {
	Token      string `json:"token"`
	User       User   `json:"userInfo"`
	ForcedBind bool   `json:"forcedBind"`
}

func (c *Client) Login(ctx context.Context, phone, code string) (LoginResult, error) {
	var result LoginResult
	err := c.post(ctx, "/user/reg", "", url.Values{"channel": {"android_app"}, "phone": {phone}, "verify": {code}}, &result)
	if err != nil {
		return result, err
	}
	if result.Token == "" || result.ForcedBind {
		return result, &Error{Code: "login_incomplete", Message: "平台尚未完成登录，请在官方 App 检查账号绑定或安全验证要求"}
	}
	return result, nil
}

func (c *Client) Check(ctx context.Context, token string) (LoginResult, error) {
	if token == "" {
		return LoginResult{}, &Error{Code: "credential_missing", Message: "账号凭据为空"}
	}
	// ApiService.userInfo returns UserInfoBean directly, not LoginBean.userInfo.
	var profile struct {
		ID    string `json:"id"`
		Phone string `json:"phone"`
		Name  string `json:"userName"`
	}
	err := c.post(ctx, "/user/info", token, nil, &profile)
	if err != nil {
		return LoginResult{}, err
	}
	if profile.ID == "" && profile.Phone == "" {
		return LoginResult{}, &Error{Code: "identity_incomplete", Message: "平台未返回可确认的账号身份", UpstreamPath: "/user/info"}
	}
	return LoginResult{Token: token, User: User{ID: profile.ID, Phone: profile.Phone, Nickname: profile.Name}}, nil
}

func SamePhone(expected, actual string) bool {
	if expected == actual {
		return expected != ""
	}
	if len(expected) != 11 || len(actual) != 11 {
		return false
	}
	for i := 0; i < 11; i++ {
		if expected[i] == '*' || actual[i] == '*' {
			if i < 3 || i >= 7 {
				return false
			}
			continue
		}
		if expected[i] != actual[i] {
			return false
		}
	}
	return true
}

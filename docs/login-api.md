# 登录 API v0.1

地址：`http://127.0.0.1:18080`。所有写接口要求 `Content-Type: application/json` 和 `X-Panguai-Client: web`。账号 ID 是本机生成的标识，不是平台用户 ID。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/status` | 后端配置状态 |
| POST | `/api/login/sms/send` | 请求一次短信，返回登录会话 |
| POST | `/api/login/sms/confirm` | 提交验证码并核对账号身份 |
| GET | `/api/login/sessions/{session_id}` | 查询登录过程或恢复已确认结果 |
| POST | `/api/login/wechat/confirm` | 校验本人微信授权凭据并登录 |
| GET | `/api/accounts` | 账号列表，只返回公开资料 |
| POST | `/api/accounts/{account_id}/check` | 用本机保存的凭据回查平台登录态 |
| DELETE | `/api/accounts/{account_id}` | 删除本机凭据；不等于平台退出登录 |

## 请求短信

请求 JSON 字段为 `phone`，内容为本人手机号。只有平台确认发送成功，才返回 HTTP 200：

对应 App 手机号登录页实际使用的上游接口为 `POST /common/sms/sendCode`，表单字段为 `phone` 和 `template=reg`。

```json
{"session_id":"48位随机十六进制会话ID","retry_after":60,"expires_in":300}
```

同一手机号每分钟最多请求一次。冷却期间返回 HTTP 429、`Retry-After` 响应头和 JSON `retry_after`。不自动重发不明确的请求。会话只保存在内存中，后端重启后失效。

## 提交验证码

```json
{"session_id":"上一步返回的会话ID","code":"手机收到的验证码"}
```

后端请求平台登录，再使用返回的 Token 回查身份，匹配手机号后加密保存，最后返回：

```json
{
  "status":"confirmed",
  "account":{
    "id":"local_本机账号标识",
    "name":"平台返回的昵称",
    "phone":"138****0000",
    "updated_at":"2026-11-12T10:00:00Z"
  }
}
```

同一会话重复确认会返回已有结果，不再次消费验证码或重复调用平台。确认进行中返回 HTTP 409。每个会话最多允许五次验证码提交。查询会话返回 `pending`、`confirming`、`verifying` 或 `confirmed`，不会返回验证码或 Token。

平台认证成功后，后端使用 App 的个人资料接口 `POST /user/info` 回查身份。若仅回查失败，会话进入 `verifying`，可再次提交仅含 `session_id` 的确认请求；无需重新输入验证码，也不会再次调用 `/user/reg`。待核对的 Token 只暂存在原会话内存中，未确认的账号不会出现在账号列表。会话超时或后端重启会清除这份临时状态。

## 微信授权登录

`POST /api/login/wechat/confirm` 接收 `access_token` 和 `openid`，必须来自本人在官方 App 中完成的本次微信 OAuth 授权。它不是胖乖 Token，也不是短信验证码。

后端先调用微信 `GET https://api.weixin.qq.com/sns/userinfo`，核对返回的 openid 并取 unionid；仅校验成功才调用 `POST /wechat/unionId/login`，表单只有 `unionId`。随后通过 `/user/info` 回查平台身份，确认后加密保存账号。微信凭据不持久保存，公开响应不返回任何 Token。

此接口不接受单独的 unionId。如果平台要求绑定手机号或额外验证，将返回错误并停止，不自动发送短信。当前测试 APK 尚未集成可直接唤起微信的授权组件，界面明确显示接入状态。不同微信开放平台的身份不能假定通用，也不能直接把官方 AppID 移植到不同包名和签名的测试 APK。

2026-11-13 已完成一次本人账号的真实后端验收：微信身份校验成功、胖乖登录成功、个人资料回查成功、凭据加密保存成功；重启电脑 Go 后端后，使用保存的凭据再次回查成功。此次授权由用户在官方 App 主动完成，用户另行明确同意仅这一次读取该进程新产生的 `NetworkUtil` 授权日志；临时微信凭据仅在采集进程内存中转交本机接口，未保存原始日志，没有发送短信。该结果不等于手机测试 APK 已能原生唤起微信。

只验证已保存登录态，不发起新微信授权或短信：

```powershell
.\scripts\test-wechat-login.ps1
```

`scripts/capture-wechat-login.py` 是本次用户明确授权的辅助验证脚本，默认最多等待四分钟，完成一次授权即停止；后续不得在没有本人重新明确授权的情况下运行日志凭据采集。它不修改系统代理、VPN、证书、官方 APK 或日志缓冲区。其离线解析及错误输出脱敏检查见 `scripts/test_capture_wechat_login.py`。

个人资料接口返回的是直接的 `UserInfoBean`（`id`、`phone`、`userName`），不是 `LoginBean.userInfo`。本次已修正由 `/user/userHome` 错误读取资料造成的 `identity_incomplete`。短信登录仍匹配手机号；微信登录允许平台稳定账号 ID，并将其加密保存在内部记录中用于之后回查。

## 错误

```json
{"code":"错误标识","message":"可读错误说明","upstream_path":"出错的上游接口路径（若适用）"}
```

| 状态码 | 典型 code | 含义 |
| --- | --- | --- |
| 400 | `phone_invalid`、`code_invalid`、`session_expired` | 参数或会话不正确 |
| 403 | `host_rejected`、`origin_rejected`、`client_rejected` | 非本机来源或客户端标记缺失 |
| 409 | `verification_required`、`login_pending`、`identity_mismatch` | 平台额外验证、操作进行中或身份冲突 |
| 429 | `sms_cooldown`、`session_limit` | 冷却或会话数量限制 |
| 502 | `platform_数字`、`upstream_network`、`upstream_response` | 平台业务拒绝或上游请求失败 |
| 503 | `protocol_pending` | 本机签名配置尚未准备 |

HTTP 200 的平台响应仍需满足业务码 `code=0`。本机返回 `confirmed` 前会完成身份回查和凭据保存。平台错误中的当前手机号、验证码和 Token 会被隐藏。

## 数据与边界

`data/accounts.enc` 使用 AES-256-GCM 加密保存账号与 Token，密钥位于 `data/accounts.key`。不要删除密钥或提交整个 `data/`。密钥与账号文件都在本机，加密不能替代本机文件访问权限。一个数据目录只由一个后端进程使用。

密码登录、二维码登录、微信原生一键授权、官方验证码组件、余额和支付尚未实现。后续已增加本人最近设备、设备详情、最近订单查询，以及饮水/淋浴的确认启动后端第一版，见 [设备使用 API](device-use-api.md)；真实启动仍待本人实机验收。

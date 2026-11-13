# 设备开始使用：后端第一版

本文保留早期启动接口的设计与联调记录，不是当前版本的完整接口清单。当前界面已接入设备使用流程，并增加状态同步与受限的远程结束能力；功能进度见 [项目说明](../README.md#功能与进度)，具体请求以 `frontend/src/api/` 和 `backend/internal/server/` 的当前实现为准。

日期：2026-11-13。已实现饮水机（04）和淋浴（08）的非蓝牙、payModel=0 解锁分支；真实账号的设备、服务和计量费率读取通过。启动报文按官方 APK 调用链还原，已通过本机模拟测试，**尚未由用户完成本版后端的真实启动验收**。手机现有 APK 未重新打包，界面尚未接入这些新接口。

## 本机接口

沿用本机账号凭据和回环地址保护。POST 需要 `Content-Type: application/json`、`X-Panguai-Client: web`。所有设备必须来自该账号的最近使用列表，且当前状态为 10（空闲）。不接收外部 Token、IMEI、价格或优惠参数。

路径前缀为 `/api/accounts/{account_id}/devices/{device_id}/usage`。

| 方法 | 后缀 | 用途与副作用 |
| --- | --- | --- |
| GET | `/options` | 读取服务和计量费率；不启动、不扣费 |
| POST | `/prepare` | 保存两分钟本机确认会话；上游仅查询 |
| POST | `/start` | 本人明确确认后，向平台发送一次解锁请求；可能开始计费 |

`prepare` 请求字段为 `sku_id`、`latitude`、`longitude`，经纬度必须来自本人实际位置，不自动推测。响应包含 `confirmation_key`、`expires_in=120`、设备、服务、计量费率和计费说明。位置仅在会话内存中暂存，不写入防重复记录。

`start` 仅接收上一步的 `confirmation_key` 和布尔值 `confirm=true`。确认绑定账号凭据、设备和服务；再次读取设备和规格，核对状态、IMEI、计费模式、服务及完整计费属性摘要。发生变化时返回 `usage_changed`，不发送启动请求。

计费展示使用 `billing_levels[].price` 和 `billing_unit`。根据官方 WaterSkuAdapter，设备 `extAttr.flowmeter=1` 对应元/L，其余已知分支对应元/秒，费率取服务 `extAttr.level[].price`。SKU 顶层 `price`、`amount`、`unit` 保留平台原值，**不能直接把 `unit` 当作物理单位，或把顶层 price 当作每升价格**。无法读出明确计费单位和费率时，确认会话不能创建。

成功响应的 `status=accepted` 只表示平台返回启动回执，不证明实物已经工作。只有消息回执而没有订单号时，需要在官方 App 核对后续结果。最终费用由平台按实际用量结算；本版不提供结束使用、订单支付、退款或优惠券功能。

`GET /api/accounts/{account_id}/orders/{order_no}/start-status` 仅查询本人最近十笔订单中的订单号，返回平台的 `platform_started` 布尔值；它不会启动设备。订单尚未进入该列表时返回 `order_not_listed`，不能由此推断启动失败。

## 防止重复启动

同一确认会话只执行一次；重复提交返回已保存的状态。不同确认键也不能并发启动同一账号的同一设备。发送前将账号与设备的摘要及到期时间加密写入 `data/usage-attempts.enc`，后端重启后仍保留 32 分钟防重复期。文件不含 Token、经纬度、手机号或原始计费配置；记录无法读取或落盘时停止发送。

超时、连接中断或回执缺失返回 `usage_result_unknown`，不自动重试。保留受控的 `upstream_code`、`upstream_path`、`upstream_message` 供核对。此时先检查官方 App 与设备，不通过重启、删除记录或更换确认键重复发送。防重复期不是平台使用时长限制，而是首版测试的本机保护措施。

## 测试

新版后端已在本机 `127.0.0.1:18080` 运行。先只读预览：

```powershell
.\scripts\test-device-use.ps1 -PreviewOnly
```

到设备旁，确认设备允许使用后再运行：

```powershell
.\scripts\test-device-use.ps1
```

脚本选择本人的设备与服务、读取真实费率、输入实际经纬度。只有最后手动输入区分大小写的 `START` 才发送一次启动请求；回车取消。它不会发送短信、采集 OAuth 日志、修改代理/VPN/证书、自动重试、自动付款或结束设备。

后端未运行时，在一个终端启动 `scripts/start-backend.ps1`，另一个终端运行测试脚本。同一个 `data` 目录只运行一个后端进程。

## 验证范围

- 所有 Go 测试、vet、构建和 PowerShell 语法检查通过。
- 用本人已保存账号验证两台设备的服务和计量费率查询，仅发送只读请求。
- 模拟验证 HeaderMap/FieldMap、签名、长整数标识、按设备分类选择路径、费率变化、明确确认、并发、防重复、重启持久性、未知结果不重试及响应脱敏。
- 真实启动、最终用量和结算仍待本人实机测试；没有捕获官方 App 原始 HTTPS 明文，不将静态还原标记为已抓包验收。

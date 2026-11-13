# 项目架构

应用采用独立前端、Go API 和 Android 原生壳三个边界。电脑和手机复用 Go 的服务与存储代码，手机通过 JNI 启动随机回环端口，前端在 WebView 中访问同一个 API。

## 前端

`App.vue` 仅组装页面、导航及面板。`views` 处理页面级布局；`components` 接收类型化数据并发出事件；`composables` 处理短信与设备使用流程；`stores/dashboard.ts` 管理账号选择及只读页面状态。

`api` 是唯一 HTTP 访问层，按账号、设备和小票划分。`types` 定义公开 DTO，前端不持有平台 Token 或账号加密密钥。状态仅在内存中保存；账号切换使用递增请求版本，防止旧请求覆盖新账号的数据。

`styles` 区分主题变量、基础样式、组件样式及响应式覆盖。使用 Prettier 和 EditorConfig 维护格式，不把业务逻辑写进样式或模板中的多语句事件。

## 后端

- `cmd/panguai`：电脑服务配置和启动。
- `cmd/android`：JNI 入口与前端嵌入。
- `internal/server`：路由装配、HTTP 中间件、JSON 响应、按业务划分的处理器；通过 Gateway 接口调用平台。
- `internal/platform`：请求签名、HTTP 客户端及官方平台协议，不处理界面。
- `internal/accounts`：AES-GCM 账号存储与持久防重复记录，不依赖 HTTP。
- `internal/buildinfo`：根目录构建命令注入的应用版本。

本轮按已有边界拆分，保留已经测试的登录会话、身份回查、设备归属、明确确认、费率检查及防重复语义。

## 构建与数据

根目录 npm workspace 和单一 package-lock 管理 Node 依赖。`npm run build` 通过 Node 进程工具编排检查、Vue 构建、Go 构建和 PowerShell Android 打包。Node/Go/Gradle 的缓存和临时文件留在项目内；环境设置仅作用于构建进程。

输出集中为 `dist/web`、`dist/backend` 和 `release/Android`。Android 嵌入目录每次复制前验证绝对路径并清除旧资产，避免旧 JS 和 CSS 混入新 APK。生成配置与 JNI 库由 Git 忽略。

账号与密钥留在 `data`，手机账号位于应用私有目录；MCP 令牌、既有运行时和原始逆向证据也不属于普通清理目标。`npm run clean` 使用固定生成路径清单，并核对解析后的目标没有逃出工作区。

## 检查

`npm run check` 执行前端类型检查、Go 单元测试和 vet；`npm run build` 还校验 Vue 模板编译与 Android 构建。`npm run check:ui` 对静态资源及 API 全部拦截，测试不连接真实后端，不消费短信，不控制实物。

`services/phone.ts` 以请求 ID 关联原生回调。Android 的 `PhoneFeatures` 只向本机页面开放前台相机与单次定位；WebView 拦截外部资源，服务禁止 iframe 和外部脚本。相机结果只传解码文本，系统位置仅用于本次启动，不落盘；相机和定位由用户在系统提示中授权。

扫码经平台解析与详情 ID 核对后，将设备元数据写入所属账号的 AES-GCM 存储。新登录保留该账号已添加的设备，重复扫码更新同一项。最近设备列表提供基础状态，已确认的本人使用会话通过独立状态同步覆盖滞后的空闲数据，未确认状态不当作空闲。远程结束按设备分类、实时能力与本次会话归属校验，重启后也不会重复发送；平台确认结束后释放本次锁定。详见使用会话报告。真实硬件验收单独记录。

# Fork 定制差异记录

本 fork 基于上游 [QuantumNous/new-api](https://github.com/QuantumNous/new-api)。
当前合并基线：`upstream/main` = `34003e9c2`（`v1.0.0-rc.47` + 6 个上游提交）；本次自 `32e587406` 合入 44 个提交。
本文档记录 fork 与上游的全部功能差异，供后续回并上游时对照。新增定制请同步更新本文件。

## 差异总览

| # | 定制 | 主要代码位置 | 回并上游时的注意点 |
|---|------|--------------|--------------------|
| 1 | 鲸鱼（JingYu）双协议任务插件 | `plugins/tasks/jingyu/` | fork 独有插件，上游无 |
| 2 | 失败后同供应商重提宿主机制 + 鲸鱼插件实现 | `service/task_polling.go`、`relay/channel/task/jsplugin/adaptor.go`、`model/task.go`、`relay/common/relay_info.go` | **上游无此机制**，合并时保留 |
| 3 | RH-Reverse 图片/视频任务插件 | `plugins/tasks/rh-reverse/` | fork 独有插件，上游无 |
| 4 | 渠道级文件自动中转与本地缓存（file_relay） | `service/file_relay*.go`、`docs/file-relay-usage.md` | fork 独有，上游无 |
| 5 | 可配置视频任务端点（VideoTaskEndpoints） | `dto` ChannelOtherSettings、jsplugin 上下文注入、Sora 插件 | fork 独有，上游无 |
| 6 | 任务插件 multipart 请求构造与请求头修复 | jsplugin multipart 提交路径 | 保留 fork 文件处理；渠道 Header Override 已采用上游通用实现 |
| 7 | Responses 跨供应商重试的任务校验与渲染修复 | relay 响应链 | 修复性质，留意上游是否已修 |
| 8 | Magnific 图片/视频任务插件及素材工具 | `plugins/tasks/magnific/`、`scripts/magnific.py` | 保留双协议适配、供应商模型声明及配套测试；脚本凭据从环境变量读取 |

## 1. 鲸鱼插件（`plugins/tasks/jingyu/`）

- 同时适配鲸鱼官方 API 与 OpenAI 兼容代理两种协议，支持视频与图片任务，
  通过 `upstream.kind`（`vendor` / `new_api`）与原生路由前缀区分调用形态。
- 原生路由（提交/查询/内容下载）与 `openai_video` / `openai_image` 协议双格式适配，
  约定记录在插件目录注释与 v1 契约文档。
- 失败重提实现见第 2 节；插件在 `state` 中保存 `{kind, body, attempts}`（上限 64KB），
  内容审核/参数错/额度类失败命中 `NON_RETRYABLE_FAILURE` 正则时拒绝重提；
  视频且带 uploads 的请求不保存 state（multipart 文件引用无法跨请求重建）。

## 2. 失败后同供应商重提（宿主机制）

下游用户以统一异步格式提交任务；上游任务终态失败后，插件可在**同一渠道**自动重提，
对用户透明。上游没有该机制，合并上游时这些点必须保留：

- `model.TaskPrivateData.Resubmits`（`model/task.go`）：重提计数字段。
- `relaycommon.TaskResubmitOutcome`（`relay/common/relay_info.go`）：重提结果载体
  （新上游任务 ID、TaskData、PluginState、可选的立即终态 `Immediate`）。
- jsplugin 适配器 `ResubmitTask`（`relay/channel/task/jsplugin/adaptor.go`）：
  调用插件可选钩子 `buildResubmitRequest(ctx, failure)`，空对象 = 拒绝；
  描述符走与提交相同的 `doFetchDescriptor` 校验（`baseUrl` + `allowedHosts`），
  响应由插件自己的 `parseSubmitResponse` 解析；state 未返回时保留旧值。
- 轮询集成（`service/task_polling.go`）：`updateVideoSingleTask` 在轮询解析出
  `FAILURE` 时、结算退款之前调用 `tryTaskPluginResubmit`；
  `MaxTaskPluginResubmits = 2` 为宿主兜底上限（state 是插件的，计数必须宿主兜底）。
  异步重提（无 `Immediate`）换绑上游 ID 后继续轮询；同步终态（有 `Immediate`）
  直接走普通终态流程，由 CAS 赢家恰好结算一次，不会重复退款。
- 契约文档：`docs/plugin-api/v1.md`「失败后插件重提」一节、`v1.d.ts` 钩子声明。
- 不覆盖范围：提交阶段即失败（immediate FAILURE at submit）不触发；批量轮询
  （batch fetch mode）不触发；remix 等派生动作由插件依据 `ctx.action` 自行拒绝。

## 3. RH-Reverse 插件（`plugins/tasks/rh-reverse/`）

逆向 RH 图片/视频接口的任务插件，支持通用模型扩展。目前未实现失败重提钩子，
如需可仿照鲸鱼插件接入 `buildResubmitRequest`。

## 4. 渠道级文件中转（file_relay）

- 统一文件中转与本地缓存：上传文件可按渠道策略自动中转，保留历史缓存访问。
- 全局与渠道双开关，使用说明见 `docs/file-relay-usage.md`。

## 5. 可配置视频任务端点（VideoTaskEndpoints）

渠道 `other_settings.video_task_endpoints` 配置后注入 `ctx.videoTaskEndpoints`
（`submitPath` / `queryPath` / `contentPath` / `remixPath`），Sora 插件据此覆盖默认端点。
契约见 `docs/plugin-api/v1.md` 中 fork 增补段落。

## 6. multipart 上游请求头修复

任务插件 multipart 提交曾缺失上游请求头，已修复并补 HTTP 回归测试（提交 `f8379ad8a`）。
当前基线已采用上游原生 multipart 路由及请求头覆盖实现，保留 fork 的按需解析上传文件逻辑和 HTTP 回归测试。声明 `routes[].models` 的原生路由只接收 JSON；文件表单路由应在解码器中校验模型。

## 7. Responses 跨供应商重试修复

`plugin_protocol_image` 链路在跨渠道重试后的任务校验与响应渲染修复（提交
`3d23857f7`、`9fcc75501` 部分）。属修复性质，回并上游时留意是否已被官方修复覆盖。

## 测试适配点（合并上游时易踩）

- kling-shadow 相关测试需禁用鲸鱼插件层，避免 fork 插件影响上游测试夹具。
- gpt-image-2 定价期望值与上游默认费率不同，测试断言已按 fork 费率调整。
- 本机 `bun run lint`（oxlint 1.74.0）对全仓扫描必现 SIGTRAP，为既有环境问题，
  与 fork 改动无关；plugins 目录下 rh-reverse/sora/xai 存在既有 lint/format 告警，勿顺手“修复”。

## 后续方向：跨供应商故障转移（设计要点备忘）

当前重提只在**同一渠道**内进行。跨供应商故障转移（某供应商失败后换渠道重提）
评估结论：

- 两个挂点：同步协议链路 `plugin_protocol_image` 的请求内等待超时/失败处，
  以及 `service/task_polling.go` 的 `FAILURE` 分支（本次重提挂点旁）。
- 计费约束：换渠道意味着费率可能不同，只能在**结算前**转移（沿用本次的
  「CAS 赢家恰好一次结算」），转移后按新渠道费率结算，跳过原渠道退款。
- remix / 派生动作链不可跨供应商转移（上游 ID 体系不同），必须拒绝。
- 与渠道重试（submit 阶段 `executeTaskSubmissionWith` 已有的跨渠道重试循环）
  互补：submit 阶段已能转移，运行期失败转移是增量能力。

## 2026-10-26 上游合并记录

- 保留鲸鱼、RH-Reverse、同供应商失败重提、文件中转、自定义视频端点和最终供应商响应渲染。
- 内容适配器及单任务/批量轮询同时传入 `ChannelOtherSettings` 和 `HeadersOverride`；重提调用适配上游四参数 `doFetchDescriptor`，沿用任务实际密钥。
- 七种语言分别保留 64 个 fork 定制键，吸收上游新增 23 个键；同步后每种语言 7423 键，无缺失或多余键。
- 异步任务总超时采用上游默认 `TASK_TIMEOUT_MINUTES=4320`（72 小时），未增加 24 小时覆盖。客户端协议等待超时及连续轮询失败上限仍是独立配置。
- 普通渠道和插件渠道可在共享入口首次选择时竞争，但后续重试分别留在普通渠道集合和插件候选集合内。混用时，普通表达式放模型级，任务表达式放插件覆盖项；不自动转换已保存价格。
- Magnific 插件、素材上传与积分对账脚本及测试在上游合并后单独提交，包含插件清单登记和跨供应商结果渲染回归。

### 本次验证

- Go `go1.27.1 darwin/arm64`：主模块 `GOWORK=off go build` 通过；`relaykit/` 内 `GOWORK=off go build ./...` 和 `go test ./dto ./relayconvert/... -count=1` 通过。
- 任务控制器、中间件、提交重试、插件适配器、路由、轮询及文件中转相关回归通过；另执行流式响应、缓存计费、Redis 限流、阿里/Gemini/DeepSeek 适配的相关包测试。
- `model/usedata_flow_test.go` 扩展 `TestQuotaDataCacheMigrationDatabaseMatrix`，以真实 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15 验证本次受影响的 `quota_data` 表。覆盖空库、合入前旧结构和 rc.47 结构，每种基线重复执行迁移，确认历史值、索引及主键约束保留，并验证新增缓存读写量的零默认值、入库、累加与四种汇总查询。rc.47 的该表结构与当前一致；未声称覆盖整个应用所有历史迁移。
- 数据库命令：配置隔离测试库的 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 后，执行 `GOWORK=off go test ./model -count=1 -v -timeout=120s -run '^TestQuotaDataCacheMigrationDatabaseMatrix$'`，三库及全部九个基线用例通过，无跳过。
- 计费命令：配置三库矩阵所需的 `TEST_FIXED_MYSQL_DSN`、`TEST_FIXED_POSTGRES_DSN` 及各自独立 `_LOG_DSN` 后，执行 `GOWORK=off go test ./service -count=1 -v -timeout=120s -run '^(TestFixedPriceBillingDatabaseMatrix|TestBillingSessionReserveWalletTopUpDecrementsBalance|TestPreConsumeBillingRejectsSaturatedQuotaBeforeDeduction|TestPreConsumeBillingRejectsNegativeQuotaBeforeDeduction)$'`，通过。覆盖更贵任务渠道的发送前补预扣、余额不足阻止发送及累计预扣全额退款。
- 前端使用本机 Bun 1.4.3：冻结锁文件安装、`bun run typecheck` 通过；64 个受影响 TS/TSX 文件定向 oxlint 为 0 错误、2 个既有警告。23 个相关测试文件首轮 505/506 用例通过，模型编辑选择器有一次查找失败；单独复跑 `metadata-editing.test.tsx` 的 10 个用例全部通过，未为偶发失败修改业务代码。
- 仓库声明及 CI 采用上游 Bun 1.4.4；本机下载该版本时直连和配置代理均超时，因此尚未在本机以 1.4.4 重跑前端检查。

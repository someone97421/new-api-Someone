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
| 9 | Any2API 六供应商任务插件 | `plugins/shared/any2api.js`、`plugins/tasks/{rh-reverse,lovart,libtv,xyq,jimeng-reverse,magnific-reverse}/` | 保留共享驱动拼接与独立插件身份；官方即梦、Magnific 不替换 |
| 10 | 按渠道请求体文字替换 | `common/body_text_replacement.go`、`controller/relay_attempt.go`、task jsplugin 解码、渠道编辑界面 | 每次尝试从原始请求替换，计费前重验；保留模型路由身份 |
| 11 | 私有图片 artifact 交付 | `controller/plugin_protocol_image.go` | 使用最终成功任务的插件和渠道下载图片，支持 `artifact_key` 转 `b64_json` |

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

## 9. Any2API 六供应商适配

- 插件键：`rh-reverse`、`lovart`、`libtv`、`xyq`、`jimeng-reverse`、`magnific-reverse`。后两项服务 Any2API 工作流，原有 `jimeng` / `magnific` 官方协议插件继续独立存在。
- 渠道类型选“任务插件”，绑定对应插件，Base URL 可指向供应商独立服务，也可填写 Any2API 的 `/api/hub/suppliers/<供应商键>/native` 前缀，密钥填写 Any2API API Key。
- 原生入口使用插件前缀，统一入口使用 `/v1/images/generations`、`/v1/images/edits`、`/v1/videos`。共享客户端模型 ID 可配置到多个供应商渠道，上游差异通过渠道模型映射处理；按现有优先级、权重和提交重试规则路由。
- LibTV、XYQ、逆向即梦和逆向 Magnific 复用共享驱动；`plugins.Source(key)` 返回拼接后的完整单文件源码。不要仅上传这些目录中的声明片段。
- RH 补充 Nano Banana 2.1、Kling v4 声明；Lovart 接入已核实的 generator 清单及最终 `inputArgs` / `modelParams` 参数覆盖。
- 四家共享驱动的参考素材使用 multipart 文件，不接收 URL 素材；XYQ/即梦 multipart 没有 extra 扩展字段，遇到不能表达的扩展会明确拒绝，JSON 保留扩展参数。
- 保留现有能力边界：RH 单成品、样片与转正片限制；LibTV 样片、Lovart 未验证目录、取消/加速等额外操作未接入。提交成功后的任务失败不自动跨供应商改投。
- 用量由最终请求参数和有效成品数量确定，不推算上游 credits / cost.power。管理员仍需配置渠道模型与插件价格。
- 私有图片由 `artifact_key` 标记，宿主使用成功任务对应的插件和渠道凭据下载，返回 `b64_json`；下载失败返回交付错误，不重复生成或修改已完成任务的结算。

## 10. 渠道请求体文字替换

- 配置入口：渠道编辑 → 请求与响应 → 请求处理 → 请求体文字替换。配置保存在 `setting.body_text_replacement`，结构为 `{enabled, rules:[{from,to}]}`，不新增数据库列。
- 默认关闭。按顺序执行区分大小写的字面匹配，不使用正则；`from` 不允许空字符串，不自动裁剪空格，`to` 可以为空。
- JSON 仅替换字符串值，包括嵌套对象、数组、URL 和模型名；保留字段名、数字、布尔值、null 与成员顺序。multipart 和表单替换文本字段，文件内容、文件名和字段名保持不变。
- 每次渠道尝试从原始请求重新应用当前渠道规则；普通 HTTP 路径重新解析和验证参数，刷新 token/图片/表达式计费输入，复用同一个扣费会话。任务插件重新解码当前渠道的有效请求，再构建提交请求与提取 usage。
- 原始模型名保留路由和账单身份，替换后的模型名再执行渠道模型映射。宿主文件引用保持不变；替换后的原任务关联必须仍属于本次已绑定的原任务。
- 当前范围为普通 HTTP relay 与声明式任务插件提交；不处理 WebSocket 消息帧、请求头、查询参数或插件自行生成的常量。

### 本轮检查状态

- 已完成 Go 格式化与差异检查，插件 JavaScript 语法及定向 lint、前端类型和定向 lint/格式检查、七语言同步。
- 已获授权并通过：主模块 `GOWORK=off go build -o <scratch>/new-api-provider-check .`；`relaykit/` 内 `GOWORK=off go build ./...`。
- Go 定向回归通过：`go test ./common -run 'TestBodyTextReplacement'`；`go test ./plugins ./relay/channel/task/jsplugin -run 'TestAny2API|TestLovart|TestTaskAdaptor'`；`go test ./controller -run 'TestServeTaskPluginImageProtocol|TestServeTaskPluginProtocolRendersAfterProviderSwitch'`；普通转发保活/重试与 `TestRelayChannelAttemptReplacesOriginalBodyBeforeRetry`；`service` 中分组计价、补预扣和负数/溢出拒绝用例。均使用 `GOWORK=off`、`-count=1`、`-timeout=120s`。控制器相关持久化夹具使用 SQLite 3.50.4；本轮未修改数据库结构、驱动或 SQL。
- 前端渠道配置文件首轮 100/101 通过，唯一失败是新增 Base UI Switch 禁用状态断言。改用 `aria-disabled` 后，定向复跑六个替换配置用例全部通过，其余 95 项未重复运行。
- 运行验证同时修复共享插件 `.async` 语法扫描误判、模型声明大小写冲突、multipart 提示词误解析、Lovart 本地素材下标错位。已覆盖私有图片、远程/本地素材选择及最终供应商下载。
- 未执行真实供应商请求或收费生成；实际账号权限、供应商可用性与成品质量仍需上线前使用自有账号确认。代码尚未提交或推送。

# 媒体能力插件与显式异步作业使用说明

本文说明本次上游对齐重构后，三项保留能力的实际入口、配置与排障方式：

1. Gemini 原生生图（Nano Banana）以 OpenAI Images 接口对外提供；
2. 非标准 OpenAI 兼容图片/视频任务由供应商插件承接；
3. 同步生图可显式转为后台异步作业，并把产物归档到本地存储。

组件归属：供应商差异在单文件 JS 任务插件中实现；媒体落盘、保留期限与过期清理在宿主存储模块中实现；异步作业复用官方任务行、渠道选择、预扣费、轮询与结算，仅增加最小的受理与投递记录。

## 1. 环境变量与配置

### 1.1 显式异步媒体作业

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ASYNC_MEDIA_ENABLED` | `true` | 是否接受显式异步请求。关闭时带标记的请求返回 503 `async_media_disabled`；同端点的同步调用不受影响。 |
| `ASYNC_MEDIA_DIR` | `./data/async-media` | 受理请求体与响应结果的本地目录（`requests/`、`responses/`）。 |
| `ASYNC_MEDIA_RETENTION_HOURS` | `168` | 响应结果保留期限（小时），从作业完成时刻起算；到期后删除文件，作业行保留为审计记录。 |
| `ASYNC_MEDIA_WORKERS` | `2` | 每轮调度最多领取并串行执行的作业数（上限 5）。 |
| `ASYNC_MEDIA_MAX_REQUEST_MB` | `8` | 受理请求体上限，超出返回 413。 |
| `ASYNC_MEDIA_STALE_MINUTES` | `30` | 执行中作业超过该时长仍未结束即判定为 worker 中断，转为失败并标记待对账；已关联官方任务的观察超时另行等待任务终态。 |

### 1.2 本地产物归档

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TASK_ARTIFACT_STORE_MODE` | `upstream` | `upstream`：产物一律实时从上游读取；`local`：启用本地归档读取优先；`s3` 仍为未实现并回落 `upstream`。 |
| `TASK_ARTIFACT_STORE_LOCAL_DIR` | `./data/task-artifacts` | 本地归档目录，按 `{任务ID}/{产物键 SHA-256}` 存放；独立 `.meta.json` 记录 MIME、大小、SHA-256 与归档时间。 |
| `TASK_ARTIFACT_STORE_RETENTION_HOURS` | `168` | 归档保留期限（小时），从归档时刻起算，取值 1..8760。 |

配置非法时启动日志会记录原因并回落 `upstream`（不阻断启动）。

## 2. 插件安装与渠道配置

三个插件均为内建（工厂）插件，随进程注册，无需上传：

| 插件 key | 名称 | 对外协议 | 用途 |
| --- | --- | --- | --- |
| `gemini-image` | Nano Banana (Gemini Image) | `openai_image` | Gemini 原生生图，客户端使用 `/v1/images/generations`、`/v1/images/edits` |
| `openai-task-image` | OpenAI 兼容异步图片任务 | `openai_image` | 提交返回任务 ID、需要查询的 OpenAI 兼容图片站点 |
| `openai-task-video` | OpenAI 兼容异步视频任务 | `openai_video` | 同类视频站点，客户端使用 `/v1/videos` 系列接口 |

安装/启用步骤：

1. 打开「任务插件」页面，确认目标插件处于启用状态；同一 key 可上传新版本覆盖内建实现，版本与回滚沿用官方机制。
2. 新建渠道：渠道类型选择 **Task Plugin**（类型 61），填写上游 Base URL 与密钥，并在渠道上绑定插件 key 与模型列表（插件只服务它在 `meta.models` 中声明的模型）。
   也可以把插件绑定到 New API（类型 60）渠道的扩展插件列表，用于「上游本身也是本网关」的转发场景。
3. `gemini-image` 声明了默认 Base URL `https://generativelanguage.googleapis.com`；渠道 Base URL 留空时会复制该默认值。使用第三方 Gemini 兼容站点时，把渠道 Base URL 指向该站点，并按站点需要设置 `model_mapping`。
4. 在「模型定价」或模型级任务用量定价中为该模型保存表达式（见第 4 节）。共享模型的每个插件可单独保存 `<pluginKey>::<model>` 的定价覆盖。

### 2.1 `gemini-image`

* 模型：`gemini-3.1-flash-image`、`gemini-3.1-flash-lite-image`、`gemini-3-pro-image`、`gemini-3-pro-image-preview`、`gemini-2.5-flash-image`、`gemini-2.0-flash-exp-image-generation`、`gemini-2.0-flash-exp`，以及公开别名 `nano-banana`（→ `gemini-2.5-flash-image`）、`nano-banana-2`（→ `gemini-3.1-flash-image`）、`nano-banana-2-lite`（→ `gemini-3.1-flash-lite-image`）、`nano-banana-pro`（→ `gemini-3-pro-image`）、`nano-banana-pro-preview`（原样透传）。渠道模型映射优先于插件别名。
* 请求字段：`prompt`（必填）、`n`（必须为 1）、`size`（`WIDTHxHEIGHT` 或 `16:9`）、`aspect_ratio`、`image_size`/`resolution`（`512`/`1K`/`2K`/`4K`）、`quality`（`hd`/`high`/`2k` → `2K`，其余 → `1K`）、`seed`、`response_format`、`image`/`images`/`image_url`/`image_urls`（Base64、data URL 或 HTTP URL，最多 16 张）、`metadata.generation_config`（透传额外 `generationConfig` 字段）、`image_config_mode`（`official` 默认，`response_format` 用于需要 `generationConfig.responseFormat.image` 的站点）。
* 上游调用：`POST {base}/v1beta/models/{model}:generateContent`，请求体为 `contents` + `generationConfig`（`responseModalities: ["TEXT","IMAGE"]`、`imageConfig`）。响应中的 `inlineData` 以 `b64_json` 返回，`fileData.fileUri` 以 `url` 返回；文本部分作为 `revised_prompt`。
* 失败语义：无图片的 2xx 响应（安全拦截、纯文本回复）判为生成失败并按 0 计费；错误信息带供应商的拦截原因。
* 计费事实：`image_count`、`image_size`。
* 已知限制：一个请求只生成一张图片（`n > 1` 直接拒绝）；HTTP(S) 参考图以 `fileData.fileUri` 透传，需要上游能自行抓取，Google 官方端点请改用 Base64/data URL 或 multipart 上传。

### 2.2 `openai-task-image`、`openai-task-video`

* 站点路径写死在文件顶部常量中（`SUBMIT_PATH`、`QUERY_PATH`，视频还有 `CONTENT_PATH`），部署到某个站点时按该站点的实际路径修改并上传新版本。
* 站点模型名写在顶部 `VENDOR_MODELS` 常量中。插件 API 要求插件声明它服务的模型，`meta.models` 不支持通配符，因此必须替换为该站点真实模型名。
* 图片插件在客户端请求内轮询到终态：提交返回任务 ID 时宿主轮询，直接返回图片时立即完成。视频插件对外是标准 `POST /v1/videos`、`GET /v1/videos/:id`、`GET /v1/videos/:id/content`。
* 状态映射：`queued|pending|created` → `QUEUED`；`processing|running|in_progress` → `IN_PROGRESS`；`succeeded|success|completed|done` → `SUCCESS`；`failed|error|failure|cancelled|canceled` → `FAILURE`；其余为 `UNKNOWN`（按轮询失败计数，不会误判为终态）。
* 计费事实：图片 `image_count`；视频 `seconds`、`resolution`（枚举 `480p`/`720p`/`1080p`）。

## 3. 显式异步调用

### 3.1 受理

在既有媒体接口上追加 `async=true`（或 `async=1`）即可，仅接受 JSON 请求体：

```bash
curl -sS -X POST "$BASE/v1/images/generations?async=true" \
  -H "Authorization: Bearer sk-<token>" \
  -H "Content-Type: application/json" \
  -d '{"model":"nano-banana","prompt":"a cat on a skateboard"}'
```

```json
{
  "id": "job_0123456789abcdef0123456789abcdef",
  "object": "async_media_job",
  "status": "queued",
  "model": "nano-banana",
  "created_at": 1790000000,
  "status_url": "/v1/async/tasks/job_0123456789abcdef0123456789abcdef"
}
```

不带标记的同一端点完全保持同步语义：请求在客户端连接内完成，返回标准 Images JSON。

### 3.2 查询

```bash
curl -sS "$BASE/v1/async/tasks/job_0123456789abcdef0123456789abcdef" \
  -H "Authorization: Bearer sk-<token>"
```

响应字段：`id`、`object`、`status`（`queued`/`running`/`awaiting_task`/`succeeded`/`failed`）、`billing_status`（`settled`/`not_charged`/`reconciliation_pending`）、`model`、`http_status`、`task_id`（官方任务行 ID，用于在任务日志中定位该次生成）、`error`、`created_at`、`started_at`、`completed_at`、`expires_at`、`content_type`，成功时附 `data`（重放请求返回的原始 JSON，即客户端本会收到的同步响应）。

只读令牌即可查询，且只能查询本人作业。管理员分页列表：`GET /api/task/async?page=1&page_size=20&status=failed`。

### 3.3 执行与语义

* 作业由系统任务 `async_media_job` 每 5 秒领取执行（数据库租约保证多节点不重复调度），领取使用状态条件更新（`queued` → `running`），一个作业只会被一个 worker 执行。
* 执行方式是**用受理者的令牌把原请求重放到本网关的同一端点**：渠道选择与重试、提交失败换渠、受理后的渠道亲和、预扣费、完成结算、失败退款与消费日志仍由官方链路唯一决定。作业本身不计算费用，也不写第二套账。
* 客户端断开或在 `status_url` 上等待，都不影响已受理作业：重放与结算在后台完成。
* 重启与中断规则：`running` 作业超过 `ASYNC_MEDIA_STALE_MINUTES` 仍未结束，会被判定为 worker 中断，记为 `failed` + `reconciliation_pending`，**绝不自动重放**（重放可能重复生成并重复计费，需要人工对账）。连接从未建立的失败（dial 失败）记为 `not_charged`。
* `billing_status` 是投递侧摘要，权威账单仍以消费日志为准：2xx 记为 `settled`；4xx 记为 `not_charged`（官方链路在返回错误前已退款）；5xx 与已发出但未收到响应的情况记为 `reconciliation_pending`。
* 受理沿用模型请求限流；每个用户最多保留 20 个未结束作业，超过时返回 429。显式异步请求只支持非流式 JSON，`stream: true` 返回 400。
* 官方图片桥等待超过协议时限并返回任务 ID 时，作业转为 `awaiting_task`，保留请求体供查询时从官方任务渲染结果，绝不重发生成请求。官方任务成功后查询会交付 Images JSON；失败则按官方任务失败记录。等待超过保留期仍无人查询时，作业变为失败并删除请求文件。
* 不支持取消：官方任务系统没有取消能力，已受理作业按上游终态或超时结算。
* 视频不需要该包装：官方 `openai_video` 协议本身就是「提交返回任务 ID + 查询 + 下载」，直接使用 `POST /v1/videos` 与 `GET /v1/videos/:id`。

## 4. 计费配置

任务用量定价使用官方任务用量表达式（`pkg/billingexpr/expr.md` 的 Task Usage Expressions，单位为美元）：

```
# Gemini 生图：每张图片单价
tier("base", u("image_count") * 0.04)

# 按输出尺寸分档
u("image_size") == "4K" ? tier("4k", u("image_count") * 0.24) : tier("base", u("image_count") * 0.04)

# 视频：每秒单价 × 分辨率档
u("resolution") == "1080p" ? tier("1080p", u("seconds") * 0.05) : tier("720p", u("seconds") * 0.03)
```

表达式中的 `u("<key>")` 必须是所选插件声明过的字段（`gemini-image`：`image_count`、`image_size`；图片任务插件：`image_count`；视频插件：`seconds`、`resolution`）。提交时按请求估算事实预扣，完成后用上游真实事实覆盖同名键并结算差额；拿不到真实值时保留预扣，不会出现零计费误解。

## 5. 本地媒体归档

* 把 `TASK_ARTIFACT_STORE_MODE` 设为 `local` 即启用。系统任务 `task_artifact_archive` 每 60 秒处理近期成功且可检索的任务，每轮最多 5 个；单次下载最长 5 分钟，失败后冷却 5 分钟重试。Gemini 立即完成的图片在同步响应前直接保存到同一存储。
* 产物内容接口优先读取本地副本（支持 Range/HEAD/条件请求）；未命中时，可检索任务回落上游。过期后清理本地文件，若上游链接已失效则返回代理错误；不保留任务正文的 Gemini 图片在副本过期后返回 404。
* 归档进度与失败分别记录在任务私有数据的 `artifact_archived_at` 和 `artifact_archive_error`，不改变生成任务状态或计费。旧视频任务也优先读取本地副本。
* Gemini 图片的本地副本按 `image_0`、`image_1` 等产物键，通过 `GET /v1/tasks/{task_id}/artifacts/{artifact_key}/content` 读取；同步 Images JSON 保持原样。
* 多节点 `local` 模式要求各节点共享同一目录（例如 NFS）；异机未命中本地副本时，可检索任务会回落上游。
* 显式异步作业响应体保留在 `ASYNC_MEDIA_DIR`，与产物归档独立；作业响应过期后仍可查询状态与 `task_id`，但不再返回 `data`。

## 6. 排障

| 现象 | 排查方向 |
| --- | --- |
| 带 `async=true` 返回 400 | 请求体必须是 JSON 对象且含 `model`；multipart 请走同步接口。 |
| 返回 503 `async_media_disabled` | `ASYNC_MEDIA_ENABLED` 被关闭，或存储目录不可用。 |
| 作业长期 `queued` | 主节点系统任务是否运行（`async_media_job` 每 5 秒一轮）；`ASYNC_MEDIA_DIR` 是否可写。 |
| 作业 `failed` + `reconciliation_pending` | worker 在请求发出后中断；查消费日志与任务日志确认是否已计费，必要时人工退款。 |
| 作业 `failed` + `not_charged` | 重放请求未通过官方链路（额度不足、令牌被禁用、渠道无可用上游等）；`error` 字段带原始错误体。 |
| 作业 `failed` + `reconciliation_pending` 且 error 提示结果超过存储上限 | 上游已生成但结果超过 32 MiB 未被保留；按消费日志确认计费并人工处理。 |
| 图片插件报 `no images generated: SAFETY` | 上游安全拦截或模型只返回文本；该情况按 0 计费。 |
| `GET /v1/tasks/:taskId/artifacts/:key/content` 变慢或 502 | 本地产物未命中而回落到上游；检查归档任务是否运行、上游链接是否有效。 |
| 归档目录增长异常 | 检查 `TASK_ARTIFACT_STORE_RETENTION_HOURS` 与系统任务 `task_artifact_archive` 的执行记录。 |

`billing_status` 是投递侧摘要，权威账单仍以消费日志为准：2xx 记为 `settled`；4xx 记为 `not_charged`（官方链路在返回错误前已退款）；5xx、已发出但未收到响应，以及结果超过 32 MiB 存储上限的情况记为 `reconciliation_pending`，需人工对账。

排障入口：`GET /api/task/:task_id/data`（管理员按需读取任务存储快照）、任务插件页面、系统任务页面的执行历史，以及 `DEBUG=true` 时按 `task_plugin` 过滤的结构化日志。插件与作业日志不会输出凭据、请求头或二进制正文。

## 7. 确定性样例与回归

* 插件确定性样例：`plugins/gemini_image_plugin_test.go`、`plugins/openai_task_plugins_test.go`（覆盖解码、校验、请求构建、同步与轮询两条结果分支、状态映射、用量事实与产物拉取）。
* 异步作业与产物存储：`service/async_media_job_test.go`（领取唯一性、中断恢复、等待官方任务、完成与过期清理、文件路径防护）、`service/task_artifact_local_store_test.go`（落盘、Range/HEAD、文件键隔离、过期与清理）、`controller/task_generic_test.go`（本地交付、重定向与超时）、`controller/plugin_protocol_test.go`（Gemini 立即完成归档）。
* 未验证项：真实供应商联调（需要站点 URL、模型名与密钥）、真实上游媒体拉取与跨节点共享目录部署、MySQL/PostgreSQL 上的新增表迁移（需要真实实例）。

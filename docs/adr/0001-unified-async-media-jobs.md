# ADR-0001：统一异步媒体任务

- 状态：已接受
- 日期：2026-08-08
- 决策者：项目维护者
- 使用说明：[OpenAI 异步生图与双渠道故障转移使用说明](../async-image-failover-guide.md)

## 背景

当前项目已经支持 Midjourney、Suno、视频等上游原生异步任务，并具有任务轮询、预扣费、失败退款和终态差额结算能力；普通生图渠道仍以同步 HTTP 请求为主。客户端容易受到长连接超时影响，不同渠道的任务提交、状态查询和结果格式也不统一。

目标是在不改变既有同步接口默认行为的前提下，通过 `async=true` 将生图、生视频请求统一包装为持久化异步任务，同时兼容 JSON、multipart 和 Base64 请求。同步渠道由内部 Worker 等待完整响应，原生异步渠道继续使用现有任务适配器和轮询链路。

## 决策

### 1. 对外协议

支持在既有媒体生成接口追加 `async=true`：

```text
POST /v1/images/generations?async=true
POST /v1/images/edits?async=true
POST /v1/video/generations?async=true
POST /v1/videos?async=true
```

提交成功返回 HTTP 202、公开 `job_id` 和状态查询地址。客户端通过统一任务查询接口获取状态和结果。未携带 `async=true` 的请求保持原行为。

### 2. 数据库是任务事实来源

新增独立的异步任务与媒体文件表，不复用现有 `tasks` 表承担通用队列职责。Redis 只允许作为未来的可选唤醒加速层，任务认领、租约、恢复、结果和计费状态必须能够仅依赖主数据库运行，并同时兼容 SQLite、MySQL 和 PostgreSQL。

### 3. 两类执行模式统一封装

- 同步渠道：Worker 在进程内重新进入既有 HTTP Relay 路由，移除 `async` 参数，完整复用鉴权、模型校验、选渠、重试、预扣费、结算和日志链路。响应由磁盘型 ResponseWriter 接收，避免大型 Base64 响应常驻内存。
- 原生异步渠道：进程内提交后关联现有 `Task` 记录，异步任务进入 `waiting_upstream`；现有轮询器推进上游任务，统一 Worker 在任务终态后转存结果。
- 返回独立图片任务 ID 的 OpenAI 兼容渠道：提交成功时同时保存最终选中的渠道 ID、上游任务 ID 和模型；后续查询固定使用原渠道，即使该渠道已停止接收新任务，也不重新随机选渠。

进程内调用不监听额外端口，不通过 localhost 网络回环，也不启动隐式独立后端服务。

### 4. 请求与媒体存储

- JSON、multipart 和 Base64 请求体均以原始字节流落入可配置的本地存储目录，数据库只保存相对路径和元数据。
- Worker 使用原始 Content-Type（包括 multipart boundary）重放请求，因此无需破坏性解析或重新编码上传内容。
- 同步响应中的 `b64_json`、Data URI、媒体 URL 和直接二进制响应统一转存为本站文件。
- 上游媒体 URL 下载必须使用现有 SSRF 防护客户端，并限制单文件大小。
- 对外只暴露随机 `file_id` 和 HMAC 签名下载地址，不暴露磁盘路径。

### 5. 生命周期与删除

- 输入请求文件在任务完成或失败后立即删除。
- 输出媒体从任务成功时间起保留 24 小时。
- 定时清理先标记过期，再删除物理文件；删除失败保留待清理状态并继续重试。
- 媒体过期后返回 HTTP 410，任务状态、错误和计费记录继续保留。

### 6. 任务和计费状态分离

任务状态：

```text
queued -> running -> waiting_upstream -> succeeded | failed
```

计费状态：

```text
pending -> delegated | settled | refunded | reconciliation_pending
```

同步 Relay 请求继续使用现有计费会话完成预扣和结算。原生异步任务将最终计费委托给现有任务轮询与差额结算链路。任务已经成功但最终账单查询暂时失败时，用户仍可获取媒体；计费进入独立的后台对账状态，不能阻塞结果交付。

### 7. 租约、幂等与重试

- Worker 通过数据库条件更新认领任务，并持续续租。
- 只有租约持有者可以写入该次执行结果。
- 已进入可能送达上游的执行阶段后，不进行无条件自动重新提交，避免重复生成和重复上游扣费。
- 原生异步任务通过公开异步任务 ID 关联现有 Task，终态处理保持幂等。
- 渠道故障转移只发生在任务被上游接受之前；一旦获得上游任务 ID，轮询必须保持渠道亲和。查询失败只重试同一渠道，不能拿该任务 ID 查询其他渠道。

### 8. 多节点部署

默认实现为本地文件存储。单节点可直接使用；多节点必须把存储目录挂载为共享卷。存储访问通过接口隔离，后续允许增加 S3、MinIO、OSS 等对象存储实现而不改变任务状态机。

## 配置

异步媒体选项可在“系统设置 → 模型设置 → 异步媒体”中持久化管理。对应环境变量继续作为首次启动默认值；已有数据库选项优先。存储目录和 Worker 数量在进程启动时确定，修改后需要重启全部实例。

支持以下环境变量：

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `ASYNC_MEDIA_ENABLED` | `true` | 是否接受异步媒体请求 |
| `ASYNC_MEDIA_STORAGE_PATH` | `./data/async-media` | 本地存储根目录 |
| `ASYNC_MEDIA_RETENTION_HOURS` | `24` | 输出媒体保留时间 |
| `ASYNC_MEDIA_WORKERS` | `4` | 单实例 Worker 数量 |
| `ASYNC_MEDIA_MAX_FILE_MB` | `2048` | 单个转存文件最大体积 |
| `ASYNC_MEDIA_LEASE_SECONDS` | `300` | Worker 租约时间 |

签名密钥复用部署的 `CRYPTO_SECRET`，避免增加另一份必须同步的多节点密钥。

## Ease 兼容图片协议与渠道转换

统一图片入口接受标准 OpenAI Images 字段以及 Ease-AI 扩展字段：`aspect_ratio`、`image_size`、`resolution`、`task_count`、`async`、`image/images`、`image_url/image_urls`。`task_count` 当前固定为 `1`，因为一个本站异步媒体任务只绑定一个上游任务 ID。

渠道转换规则：

- OpenAI/高级自定义鲸鱼渠道保留这些扩展字段，模型映射后提交到渠道配置的上游路径。
- Gemini Imagine 渠道把比例、分辨率和参考图转换为 `generateContent`；官方模式使用 `generationConfig.responseFormat.image`，旧版兼容模式使用 `generationConfig.imageConfig`。
- 鲸鱼查询响应中的 `result_urls`、`result_asset_urls` 和常见单数 URL 字段统一进入本站文件转存流程。
- 查询响应明确进入失败终态时，本站任务同步失败，不再永久保持 `waiting_upstream`。

渠道编辑器提供 Gemini 主渠道和鲸鱼备用渠道预设，用于自动填写公开模型、模型映射、优先级、提交路由和查询路径。密钥、Base URL 和分组仍由管理员确认。

## 后果

### 正面

- 所有同步和原生异步媒体渠道获得统一的客户端协议。
- 大型 Base64 和 multipart 数据不进入数据库，也不要求常驻内存。
- 完整复用既有 Relay 和计费路径，减少两套业务逻辑产生偏差。
- Redis 故障或未部署时仍可工作。

### 代价

- 同步上游仍会长期占用一个 Worker 的上游连接；异步包装只解除客户端连接，不会缩短上游执行时间。
- 本地文件存储要求运维监控容量，多节点需要共享卷。
- 进程在上游已接收请求后崩溃时无法普遍判断是否可以安全重试，因此部分任务可能进入需要人工确认的失败状态。

## 不采用的方案

- 直接将请求交给 goroutine：进程退出即丢失，无法多节点接管。
- 只使用 Redis 队列：Redis 不应成为任务和账务事实来源，且项目允许无 Redis 部署。
- Worker 请求 localhost 服务：会增加端口依赖和网络回环；采用进程内 Handler 调用即可复用同一套路由。
- 把通用异步任务塞入现有 `Task`：现有表围绕上游原生任务设计，无法清晰表达同步执行、文件生命周期和独立计费状态。

# OpenAI 异步生图与双渠道故障转移使用说明

本文说明如何把一个同步 Gemini 官方生图渠道和一个 OpenAI 兼容异步生图渠道，统一包装成下游可调用的 OpenAI 异步接口，并在任务提交阶段进行故障转移。

## 功能目的和工作原理

这项功能解决的是“一个客户端只能维护一种调用方式，但不同图片供应商协议完全不同”的问题。下游统一使用 Ease-AI 风格的 OpenAI 兼容请求，本站先保存一个自己的异步任务，再根据渠道类型转换请求：

- 鲸鱼渠道收到 `aspect_ratio`、`image_size`、`task_count`、`async` 和参考图 URL 等字段，并返回上游 `task_id`。
- Gemini 官方渠道收到转换后的 `generateContent` 请求；参考图 URL 会由本站安全下载并转成 `inlineData`。
- 无论最终使用哪个渠道，下游都只保存本站 `job_id`，不需要了解上游协议。

故障转移只发生在上游任务尚未被明确接受时。鲸鱼一旦返回 `task_id`，后续轮询就固定使用该渠道，避免同一请求在两个供应商重复生成和重复扣费。

适用场景：

- 上游 A：Gemini 官方渠道，同步返回图片。
- 上游 B：OpenAI 兼容中转渠道，提交后返回任务 ID，需要继续查询任务。
- 下游：始终使用 `POST /v1/images/generations?async=true` 提交，通过本站任务接口查询结果。

## 一、最终调用效果

下游只需要认识本站任务 ID，不需要区分最终命中了哪个上游：

```text
POST /v1/images/generations?async=true
                    │
                    ▼
              返回本站 job_id
                    │
                    ▼
       后台选渠、提交、重试和故障转移
          │                      │
          ▼                      ▼
  Gemini 官方同步生成      中转站异步生成
  等待图片并转存           保存 task_id 并固定原渠道轮询
          │                      │
          └──────────┬───────────┘
                     ▼
        GET /v1/async/tasks/{job_id}
                     │
                     ▼
              返回本站图片地址
```

故障转移边界：

- 上游尚未接受任务：可以切换到另一个渠道重新提交。
- 异步中转已经返回任务 ID：后续查询固定使用提交该任务的渠道，不再随机选渠。
- 已返回任务 ID 后发生查询超时、429 或 5xx：继续重试原渠道。
- 上游明确返回终态失败：本站任务失败，不会自动到另一渠道重新生成，避免重复图片和重复扣费。

## 二、开始前检查

开始配置前确认：

1. 当前部署包含统一异步媒体任务功能。
2. 数据库迁移已经执行，`async_media_jobs` 表具有 `upstream_channel_id` 字段。
3. Gemini 官方账号能够访问准备使用的图片模型。
4. 中转站提供以下两个接口：
   - 提交图片任务接口。
   - 根据任务 ID 查询结果的接口。
5. 两个渠道在本站使用同一个下游模型名和同一个分组。
6. 系统重试次数至少为 `1`。

升级后正常重启一次主程序即可由 GORM 自动补充新增字段。正式环境操作前仍建议备份数据库。

## 三、启用异步媒体任务

异步媒体任务默认启用。现在可以直接进入：

```text
系统设置 → 模型设置 → 异步媒体
```

设置页提供以下控制项：

| 设置项 | 默认值 | 生效方式 |
| --- | ---: | --- |
| 接收新的异步媒体任务 | 开启 | 保存后立即生效；关闭后已有任务继续执行。 |
| 存储目录 | `./data/async-media` | 保存后需重启所有实例。 |
| Worker 数量 | `4` | 保存后需重启所有实例。 |
| Worker 租约 | `300` 秒 | 保存后应用于后续任务处理。 |
| 结果保留时间 | `24` 小时 | 保存后应用于新生成的结果文件。 |
| 最大文件大小 | `2048` MB | 保存后应用于后续下载和解码。 |
| 任务超时 | `1440` 分钟 | 保存后应用于后续超时检查；`0` 表示禁用。 |
| 故障转移重试次数 | `0` | 主备两个渠道时设置为 `1`；`0` 表示不切换备用渠道。 |

### 新手推荐配置

1. 进入“渠道管理”，创建一个 `Gemini` 渠道。
2. 在渠道的“模型与分组”区域点击“应用 Gemini 香蕉预设”。系统会添加对外模型 `nano-banana-2`，映射到 `gemini-3.1-flash-image`，并把优先级设置为 `100`。
3. 再创建一个“高级自定义”渠道，填写鲸鱼 Base URL 和密钥。
4. 在该渠道点击“应用鲸鱼香蕉预设”。系统会自动填写提交路径 `/v1/image/generations`、查询路径 `/v1/image/generations/{task_id}`、对外模型和备用优先级 `0`。
5. 确认两个渠道属于同一个分组，例如 `default`。
6. 回到“系统设置 → 模型设置 → 异步媒体”，开启异步任务并把“故障转移重试次数”设置为 `1`。
7. 在“系统设置 → 模型设置 → Gemini”中保持“官方 responseFormat（推荐）”。只有旧兼容网关明确提示不认识 `responseFormat` 时，才切换到“旧版 imageConfig”。

预设不会填写 Base URL、API Key 或分组，因为这些信息与实际账号和部署环境有关，仍需人工确认。预设会覆盖该渠道的香蕉相关模型映射、优先级和高级路由；不要在已经承载其他复杂高级路由的渠道上随意应用鲸鱼预设。

数据库中的设置值优先于对应环境变量。环境变量仍可用作首次启动默认值或纯配置文件部署方式：

```env
ASYNC_MEDIA_ENABLED=true
ASYNC_MEDIA_STORAGE_PATH=./data/async-media
ASYNC_MEDIA_RETENTION_HOURS=24
ASYNC_MEDIA_WORKERS=4
ASYNC_MEDIA_MAX_FILE_MB=2048
ASYNC_MEDIA_LEASE_SECONDS=300
TASK_TIMEOUT_MINUTES=1440
CRYPTO_SECRET=请替换成长期固定的高强度随机字符串
```

配置说明：

| 环境变量 | 建议值 | 说明 |
| --- | ---: | --- |
| `ASYNC_MEDIA_ENABLED` | `true` | 接受带 `async=true` 的媒体生成请求。 |
| `ASYNC_MEDIA_STORAGE_PATH` | `./data/async-media` | 保存原始请求、临时响应和最终媒体文件。 |
| `ASYNC_MEDIA_RETENTION_HOURS` | `24` | 任务成功后图片保留时间，过期下载返回 HTTP 410。 |
| `ASYNC_MEDIA_WORKERS` | `4` | 每个实例同时工作的异步 Worker 数量。同步 Gemini 请求会持续占用一个 Worker。 |
| `ASYNC_MEDIA_MAX_FILE_MB` | `2048` | 单个转存文件大小上限。 |
| `ASYNC_MEDIA_LEASE_SECONDS` | `300` | Worker 的数据库租约时间。 |
| `TASK_TIMEOUT_MINUTES` | `1440` | 整个异步任务允许执行的最长时间；`0` 表示不限制。 |
| `CRYPTO_SECRET` | 固定随机值 | 用于内部 Worker 签名和图片下载签名。多节点必须一致，修改后旧链接会失效。 |

`CRYPTO_SECRET` 属于部署级敏感配置，不会显示在设置界面中，仍需通过安全的环境变量或密钥管理系统配置。

部署注意事项：

- 单节点可以直接使用本地目录。
- Docker 部署必须把 `ASYNC_MEDIA_STORAGE_PATH` 挂载到持久化卷，否则重建容器后图片会丢失。
- 多节点部署必须让所有节点访问同一个共享存储目录，并使用相同的 `CRYPTO_SECRET`。
- 存储目录必须允许运行 new-api 的用户读写。

## 四、设置系统重试

进入：

```text
系统设置 → 模型设置 → 路由可靠性 → 请求重试
```

推荐配置：

```text
重试次数：1
自动重试状态码：401,403,408,429,500-599
```

`重试次数` 默认是 `0`。如果保持为 `0`，第一个渠道失败后不会切换到备用渠道。

建议两个渠道使用不同优先级：

| 渠道 | 优先级示例 | 作用 |
| --- | ---: | --- |
| Gemini 官方同步渠道 | `100` | 正常情况下优先使用。 |
| OpenAI 异步中转渠道 | `0` | Gemini 提交失败后作为备用渠道。 |

系统按照优先级从高到低进行重试。不要把两个渠道都配置成相同优先级，否则它们更接近按权重随机分流，而不是明确的主备故障转移。

如果希望中转站作为主渠道，交换两个优先级即可。

## 五、配置统一的下游模型名

以下示例统一向下游暴露：

```text
nano-banana
```

两个渠道都必须声明支持这个模型，并位于同一分组，例如 `default`。

推荐关系：

| 项目 | Gemini 官方渠道 | 异步中转渠道 |
| --- | --- | --- |
| 下游模型名 | `nano-banana` | `nano-banana` |
| 实际上游模型 | 例如 `gemini-3.1-flash-image` | 中转站实际模型名 |
| 分组 | `default` | `default` |
| 优先级 | `100` | `0` |

实际上游模型名称仅为示例，请替换成对应账号当前真正可调用的模型。

## 六、配置 Gemini 官方同步渠道

在渠道管理中新建或编辑渠道：

| 配置项 | 示例 |
| --- | --- |
| 类型 | `Gemini` |
| 名称 | `Gemini 官方香蕉` |
| Base URL | 使用正常的 Gemini 官方 API Base URL |
| 密钥 | Gemini API Key |
| 分组 | `default` |
| 模型 | `nano-banana` |
| 优先级 | `100` |

如果下游模型名和实际上游模型名不同，在渠道的模型映射中配置：

```json
{
  "nano-banana": "gemini-3.1-flash-image"
}
```

然后进入：

```text
系统设置 → 模型设置 → Gemini → 支持的 Imagine 模型
```

确认映射后的实际上游模型在 JSON 数组中，例如：

```json
[
  "gemini-3.1-flash-image",
  "gemini-3.1-flash-image-preview",
  "gemini-3-pro-image",
  "nano-banana-pro-preview"
]
```

注意：

- 系统判断是否使用 Gemini 原生图片协议时，检查的是映射后的上游模型名。
- Gemini 原生图片渠道当前每次请求生成一张图片。
- 当下游传入 `n > 1` 时，Gemini 渠道会主动放弃本次请求，让正常重试逻辑切换到支持多图的备用渠道。
- Gemini 返回的 `inlineData` 会转换为 OpenAI Images 的 `b64_json`，随后由异步 Worker 转存成本地签名文件。
- Ease-AI 风格的 `aspect_ratio` 会转换为 Gemini `aspectRatio`，`image_size`/`resolution` 会转换为 `imageSize`。
- `image`、`images`、`image_url` 和 `image_urls` 中的 HTTP/HTTPS 参考图会经过本站 SSRF 防护、文件类型和大小限制，再转换为 Gemini `inlineData`。
- 单次 Gemini 请求最多转换 16 张参考图，超过上限会让本次渠道尝试失败并进入正常故障转移。
- Gemini 官方推荐使用 `generationConfig.responseFormat.image`；旧版 `imageConfig` 仅用于兼容旧网关。

## 七、配置 OpenAI 异步中转渠道

根据中转站的提交路径选择以下一种方式。

### 方式 A：中转站使用标准 OpenAI 提交路径

如果中转站接收：

```text
POST /v1/images/generations
```

可以直接创建 `OpenAI` 类型渠道：

| 配置项 | 示例 |
| --- | --- |
| 类型 | `OpenAI` |
| 名称 | `香蕉异步中转` |
| Base URL | `https://relay.example.com` |
| 密钥 | 中转站 API Key |
| 分组 | `default` |
| 模型 | `nano-banana` |
| 优先级 | `0` |
| 图片任务查询路径 | `/v1/images/generations/{task_id}` |

如果中转站使用不同的实际上游模型名，配置模型映射：

```json
{
  "nano-banana": "vendor-banana-model"
}
```

### 方式 B：中转站提交路径不是标准 OpenAI 路径

例如中转站实际接口为：

```text
POST /v1/image/generations
GET  /v1/image/generations/{task_id}
```

创建 `高级自定义` 渠道，填写：

| 配置项 | 示例 |
| --- | --- |
| 类型 | `高级自定义` |
| 名称 | `香蕉异步中转` |
| Base URL | `https://relay.example.com` |
| 密钥 | 中转站 API Key |
| 分组 | `default` |
| 模型 | `nano-banana` |
| 优先级 | `0` |
| 图片任务查询路径 | `/v1/image/generations/{task_id}` |

高级自定义路由配置：

```json
{
  "advanced_routes": [
    {
      "incoming_path": "/v1/images/generations",
      "upstream_path": "/v1/image/generations",
      "converter": "none",
      "models": [
        "nano-banana"
      ]
    }
  ]
}
```

说明：

- `incoming_path` 是下游访问本站的路径。
- `upstream_path` 是中转站的真实提交路径，可以填写相对路径或完整 URL。
- `converter: "none"` 表示请求体保持 OpenAI Images 格式。
- `models` 限定该路由可以处理的下游模型。
- 图片任务查询路径必须是以 `/` 开头的相对路径，并且必须恰好包含一次 `{task_id}`。

如果中转站不使用标准 `Authorization: Bearer ...` 鉴权，可以在高级自定义路由中另外配置 Header 或 Query 鉴权。

## 八、中转站响应格式要求

### 1. 提交任务响应

中转站提交成功后必须返回 HTTP 2xx，并在以下任一位置提供字符串任务 ID。

支持的顶层字段：

```json
{
  "task_id": "upstream-task-123",
  "status": "queued"
}
```

也支持：

```json
{
  "taskId": "upstream-task-123"
}
```

```json
{
  "id": "upstream-task-123"
}
```

或者放在 `data` 对象中：

```json
{
  "data": {
    "task_id": "upstream-task-123"
  }
}
```

任务 ID 必须是 JSON 字符串。数字任务 ID 应由中转站转换成字符串。

### 2. 查询任务响应

本站调用配置的查询路径后：

- HTTP 2xx 且响应中尚无图片：认为任务仍在处理，稍后继续查询。
- HTTP 429 或 5xx：认为查询暂时失败，稍后继续查询原渠道。
- 其他非 2xx：认为任务查询发生不可恢复错误，结束本站任务。
- HTTP 2xx 且发现图片：下载或解码图片，保存到本站存储并完成任务。

查询响应可以使用以下任一图片格式。

鲸鱼统一接口常用的 `data.result_urls[]` 和 `data.result_asset_urls[]` 也会被识别、下载和去重。

OpenAI `b64_json`：

```json
{
  "status": "succeeded",
  "data": [
    {
      "b64_json": "iVBORw0KGgoAAA..."
    }
  ]
}
```

图片 URL：

```json
{
  "status": "succeeded",
  "data": [
    {
      "url": "https://cdn.example.com/result.png"
    }
  ]
}
```

还会识别以下字段：

```text
base64
binary_data_base64
image_url
download_url
result_url
video_url
```

也支持 `data:image/...;base64,...` Data URI，以及直接返回 `image/*`、`video/*` 或 `application/octet-stream` 二进制响应。

URL 必须能被 new-api 服务端访问。最终会由本站下载并重新生成签名地址，不会把中转站原始 URL 直接交给下游。

## 九、下游调用方法

下面假设：

```text
本站地址：https://api.example.com
本站令牌：sk-your-new-api-token
下游模型：nano-banana
```

### 1. 提交异步生图任务

```bash
curl -X POST "https://api.example.com/v1/images/generations?async=true" \
  -H "Authorization: Bearer sk-your-new-api-token" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nano-banana",
    "prompt": "一只可爱的像素风毛绒小恐龙，坐在电脑前写代码",
    "n": 1,
    "size": "1024x1024",
    "quality": "high"
  }'
```

成功后返回 HTTP 202：

```json
{
  "id": "job_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "async_media_job",
  "status": "queued",
  "created_at": 1780000000,
  "status_url": "/v1/async/tasks/job_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
}
```

`id` 是本站任务 ID，不是中转站任务 ID。下游只保存本站任务 ID。

当前统一异步路由要求 `task_count: 1`。这是因为一个本站 `job_id` 只绑定一个上游任务 ID；Ease-AI 当前调用本身也固定使用单任务。批量生成应由客户端分别提交多个本站任务，避免部分成功、部分失败时无法准确结算和故障转移。

### 2. 查询任务状态

```bash
curl "https://api.example.com/v1/async/tasks/job_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Authorization: Bearer sk-your-new-api-token"
```

处理中示例：

```json
{
  "id": "job_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "async_media_job",
  "status": "waiting_upstream",
  "billing_status": "settled",
  "http_status": 200,
  "error": "上游图片任务仍在处理中",
  "created_at": 1780000000,
  "started_at": 1780000001,
  "completed_at": 0,
  "expires_at": 0,
  "data": []
}
```

成功示例：

```json
{
  "id": "job_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "object": "async_media_job",
  "status": "succeeded",
  "billing_status": "settled",
  "http_status": 200,
  "error": "",
  "created_at": 1780000000,
  "started_at": 1780000001,
  "completed_at": 1780000020,
  "expires_at": 1780086420,
  "data": [
    {
      "id": "file_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "mime_type": "image/png",
      "size": 1234567,
      "sha256": "...",
      "expires_at": 1780086420,
      "created_at": 1780000020,
      "url": "/v1/async/files/file_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx?expires=1780086420&signature=..."
    }
  ]
}
```

### 3. 下载图片

`data[].url` 是相对本站域名的签名地址：

```bash
curl -L "https://api.example.com/v1/async/files/file_xxx?expires=1780086420&signature=xxx" \
  --output result.png
```

文件链接已经包含签名，不需要额外传递 API Key。不要把未过期的签名链接公开给无关人员。

### 4. 推荐轮询策略

下游建议每 2 至 5 秒查询一次：

```text
queued / running / waiting_upstream → 继续轮询
succeeded                          → 读取 data[].url
failed                             → 停止轮询并显示 error
```

不要每秒进行大量并发轮询，也不要在任务已经进入终态后继续查询。

## 十、任务状态和计费状态

任务状态：

| 状态 | 含义 |
| --- | --- |
| `queued` | 已入队，等待 Worker。 |
| `running` | Worker 正在提交、等待同步响应或处理结果。 |
| `waiting_upstream` | 异步上游仍在处理，或结果转存正在重试。 |
| `succeeded` | 图片已经转存到本站。 |
| `failed` | 任务失败，不会继续执行。 |

计费状态：

| 状态 | 含义 |
| --- | --- |
| `pending` | 尚未完成计费处理。 |
| `delegated` | 计费交给已有的原生异步任务链路处理。 |
| `settled` | 已完成结算。 |
| `refunded` | 失败并已退款。 |
| `reconciliation_pending` | 上游可能已经接受或完成任务，但账务或任务状态不够明确，需要管理员核对。 |

任务状态和计费状态彼此独立。判断图片是否可以使用应以 `status` 和 `data` 为准，不要只检查 `billing_status`。

## 十一、如何验证故障转移

建议按以下顺序验收。

### 测试 1：只验证 Gemini 官方同步渠道

1. 暂时禁用异步中转渠道。
2. 使用 `n: 1` 提交异步请求。
3. 确认提交立即返回本站 `job_id`。
4. 轮询直到 `succeeded`。
5. 下载 `data[0].url`，确认图片正常。

### 测试 2：只验证异步中转渠道

1. 启用中转渠道并暂时禁用 Gemini 渠道。
2. 提交异步请求。
3. 查看任务是否进入 `waiting_upstream`。
4. 确认中转站任务完成后，本站任务进入 `succeeded`。
5. 确认结果 URL 已转成本站 `/v1/async/files/...` 地址。

### 测试 3：验证 Gemini 到中转站的故障转移

1. 两个渠道都启用。
2. Gemini 优先级设为 `100`，中转渠道设为 `0`。
3. 系统重试次数设为 `1`。
4. 临时把 Gemini Key 改成无效值，或使用明确会产生可重试状态码的测试配置。
5. 提交任务。
6. 检查请求日志中的渠道链路，应该出现从 Gemini 到中转站的重试。
7. 确认中转站任务完成后本站返回图片。

不要使用真实生产请求反复制造模糊超时。上游可能已经接受请求但本地没有收到响应，此时自动重提存在重复生成和重复扣费风险。

### 测试 4：验证异步任务不会串渠道

1. 让任务成功提交到异步中转渠道并取得任务 ID。
2. 在任务处理中禁用该渠道的新请求。
3. 保持渠道记录和密钥不删除。
4. 任务查询仍应固定使用该中转渠道，直至成功或明确失败。

## 十二、常见问题排查

### 1. 提交后没有返回 HTTP 202

检查：

- 请求 URL 是否包含 `async=true` 或 `async=1`。
- 是否请求 `POST /v1/images/generations`。
- “系统设置 → 模型设置 → 异步媒体”中的“接收新的异步媒体任务”是否开启。
- 如果使用环境变量作为首次默认值，是否已经重启应用。

### 1.1 Ease-AI 请求的比例或参考图在 Gemini 中没有生效

检查：

- 当前代码是否包含 Ease 兼容字段转换功能。
- Gemini 渠道映射后的实际上游模型是否位于“支持的 Imagine 模型”列表。
- “Gemini 图片配置格式”是否为“官方 responseFormat（推荐）”。
- 参考图是否为服务端可访问的 HTTP/HTTPS URL，且未被 SSRF 策略、端口白名单或文件大小限制拦截。
- 请求是否使用 `aspect_ratio`、`image_size`、`image/images` 等本文支持的字段。

### 2. 任务一直是 `queued`

检查：

- 日志是否出现“异步媒体 Worker 已启动”。
- `ASYNC_MEDIA_WORKERS` 是否大于 `0`。
- 数据库是否可以正常写入和更新任务租约。
- 存储目录是否可读写。

### 3. 任务一直是 `waiting_upstream`

检查：

- 中转站任务本身是否已经完成。
- 图片任务查询路径是否与中转站真实路径一致。
- 查询响应是否为 HTTP 2xx。
- 查询响应是否包含本站能够识别的图片字段。
- 图片 URL 是否能被 new-api 服务端访问。

如果查询响应只是：

```json
{
  "status": "SUCCESS",
  "result": {
    "image": "https://example.com/a.png"
  }
}
```

其中字段名是 `image`，本站不会把它当作图片 URL。应让中转站返回 `url`、`image_url`、`download_url`、`result_url`、`result_urls` 或 `result_asset_urls`，或者在中转站侧转换响应。

### 4. 提示缺少原始上游渠道信息

常见原因：

- 应用实例代码版本不一致。
- 多节点中有旧版本节点仍在处理任务。
- 数据库迁移未完成。
- 任务是在升级前创建的历史任务。

处理方式：统一升级并重启所有节点，确认数据库存在 `upstream_channel_id` 字段。历史任务如果已经被上游接受，需要人工核对，不要盲目重新提交。

### 5. 没有发生故障转移

检查：

- 系统 `重试次数` 是否至少为 `1`。
- 两个渠道是否处于同一分组。
- 两个渠道是否都声明同一个下游模型。
- 是否使用了两个不同的优先级。
- 上游返回状态码是否在自动重试范围内。
- 下游令牌是否指定了固定渠道。指定渠道请求不会切换到其他渠道。
- 渠道亲和规则是否配置为失败时禁止重试。

### 6. Gemini 返回“不支持图片生成模型”

检查：

- 模型映射后的上游模型是否在“支持的 Imagine 模型”数组中。
- 模型名是否拼写完全一致。
- 实际调用的是 Gemini 图片模型，而不是普通文本模型。

### 7. `n > 1` 总是走中转站

这是预期行为。Gemini 原生图片渠道当前只处理单图请求，多图请求会触发正常渠道重试，由支持多图的备用渠道处理。

### 8. 下载图片返回 HTTP 410

图片已经超过 `ASYNC_MEDIA_RETENTION_HOURS` 设置的保留时间，或者物理文件已被清理。任务记录仍可能存在，但需要重新生成图片。

### 9. 重启后任务变成需要对账

如果进程在请求可能已经送达上游后退出，系统不能安全判断是否应该重新提交。为避免重复生成和重复扣费，这类任务不会无条件重放，可能进入 `reconciliation_pending`。管理员需要结合本站日志和上游账单人工核对。

## 十三、推荐的最小可用配置

如果你的中转站使用标准 OpenAI 路径，最简单的组合是：

```text
系统重试次数：1

渠道 A：Gemini
  模型：nano-banana
  模型映射：nano-banana -> 实际 Gemini 图片模型
  分组：default
  优先级：100

渠道 B：OpenAI
  Base URL：https://你的中转站
  模型：nano-banana
  分组：default
  优先级：0
  图片任务查询路径：/v1/images/generations/{task_id}
```

下游调用：

```text
POST /v1/images/generations?async=true
GET  /v1/async/tasks/{job_id}
GET  /v1/async/files/{file_id}?expires=...&signature=...
```

先分别验证两个渠道能够独立完成任务，再启用主备故障转移。这样出现问题时，更容易判断是渠道协议、查询路径、图片转存还是路由配置错误。

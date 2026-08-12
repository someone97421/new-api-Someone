# Gemini2GPT / GPT2Gemini 协议桥渠道使用说明

## 功能目的

协议桥渠道用于解决“下游只会一种请求格式，上游却来自不同协议”的问题。它们是渠道管理中的正式大类，不再绑定 Ease、鲸鱼或香蕉等供应商名称。

- `Gemini2GPT`：下游使用 Gemini `generateContent`，中转层转换为 OpenAI Chat / Images / Videos 兼容格式发送给上游，再把响应转换回 Gemini 格式。
- `GPT2Gemini`：下游使用 OpenAI Chat / Images / Videos 兼容格式，中转层转换为 Gemini `generateContent`、Imagen 或 Veo 格式发送给上游，再把响应转换回 OpenAI 格式。

适合管理员把同一个公开模型配置到多个不同协议的渠道中，并使用项目原有的分组、优先级、权重和重试机制完成故障转移。

## 新手配置步骤

1. 进入“渠道管理”，新建渠道。
2. 如果上游是 Gemini 官方或 Gemini 原生中转，类型选择 `GPT2Gemini`；如果上游接受 OpenAI 兼容请求，类型选择 `Gemini2GPT`。
3. 填写上游 Base URL 和密钥。Gemini 官方可填写 `https://generativelanguage.googleapis.com`，也兼容末尾已带 `/v1beta` 的地址。
4. 在“模型”填写下游公开模型名；若上游模型名不同，在“模型映射”填写映射关系。`GPT2Gemini` 已内置常见 Nano Banana 别名，手工模型映射仍具有更高优先级。
5. 对需要故障转移的渠道使用相同公开模型名和相同分组。主渠道优先级填较大值，备用渠道填较小值。
6. 展开“高级设置 → 渠道额外设置”，按需填写图片/视频上游路径、自定义透传字段和自定义字段替换。
7. 保存后重启应用，使新增渠道类型的后端代码生效。数据库不需要手工迁移。

## 已内置的字段兼容

图片请求已覆盖参考文档中的常用字段：

- `model`、`prompt`、`n`、`task_count`、`async`
- `size`、`aspect_ratio`、`image_size`、`resolution`、`quality`
- `response_format`、`output_format`、`output_compression`
- `seed`、`negative_prompt`、`watermark`
- `image`、`images`、`image_url`、`image_urls`、`references`

OpenAI 图片请求转 Gemini 图片请求时，默认把比例和分辨率写入 Gemini 官方的 `generationConfig.imageConfig`。只有明确要求另一种结构的兼容中转站，才在系统设置中切换为 `generationConfig.responseFormat.image`。HTTP 图片和 Data URI 会转换为 Gemini `inlineData`。Gemini 图片响应会转换为 OpenAI Images 响应。

`GPT2Gemini` 还会在没有手工模型映射时自动规范以下常见公开模型名：

```text
nano-banana-2   -> gemini-3.1-flash-image
nano-banana-pro -> gemini-3-pro-image
```

例如 Canvas 仍可提交 `model: "nano-banana-2"`，协议桥会把请求发往 Gemini 官方的 `gemini-3.1-flash-image:generateContent`。如果渠道的“模型映射”显式配置了 `nano-banana-2`，则以管理员配置为准，不会被内置别名覆盖。

视频请求已覆盖：

- `model`、`prompt`、`duration`、`seconds`
- `aspect_ratio`、`resolution`、`size`、`mode`
- `task_count`、`generate_audio`、`seed`
- `image`、`images`、`input_reference`、`references`
- `metadata`、`extra`

`GPT2Gemini` 的视频任务复用 Veo 异步任务适配器，会把 `duration`、`aspect_ratio`、`resolution`、`seed`、`generate_audio` 和第一张图片 reference 转为 Gemini/Veo 字段。`Gemini2GPT` 的视频任务复用 OpenAI/Sora 兼容适配器，并可自定义提交、查询、内容和 remix 路径。

## 自定义透传字段

“自定义透传字段”每行填写一个 JSON 路径。只有下游请求中真实存在的值才会复制到转换后的上游请求；显式 `0`、`false` 和空字符串不会被误当成未填写。

示例：

```text
seed
negative_prompt
references
extra.doubao
vendor.camera_fixed
```

透传适合上下游字段名和层级相同，但内置 DTO 尚未声明的供应商扩展字段。

## 自定义字段替换

“自定义字段替换”填写 JSON 对象，左边是下游源路径，右边是转换后上游目标路径：

```json
{
  "aspect_ratio": "generationConfig.imageConfig.aspectRatio",
  "image_size": "generationConfig.imageConfig.imageSize",
  "duration": "parameters.durationSeconds",
  "generate_audio": "parameters.generateAudio",
  "extra.seedance": "parameters.vendorOptions"
}
```

执行顺序为：内置协议转换 → 自定义字段替换 → 自定义同名透传 → 渠道原有参数覆盖。后执行的规则可以覆盖前面的转换结果。鉴权、Base URL、目标 URL 等敏感根字段禁止作为映射目标。

协议桥渠道始终执行协议转换，不受全局或渠道“直接透传请求体”开关影响。该开关适合上下游协议完全相同的普通渠道；若在协议桥中直接把 OpenAI 请求原样交给 Gemini，Gemini 会因字段和结构不匹配而拒绝请求。

## 路径设置

`Gemini2GPT` 可设置：

- 图片提交路径，例如 `/v1/image/generations`
- 图片查询路径，例如 `/v1/image/generations/{task_id}`
- 视频提交路径，例如 `/v1/video/generations`
- 视频查询路径，例如 `/v1/video/generations/{task_id}`
- 视频内容路径和 remix 路径

留空时使用 OpenAI 默认路径。路径必须以 `/` 开头；查询路径必须且只能包含一个 `{task_id}`。

## 故障转移原理

协议桥本身只负责“协议转换”。故障转移继续使用系统现有渠道路由：同一公开模型、同一分组内优先选择高优先级渠道；调用失败且错误允许重试时切换到其他可用渠道。

异步图片/视频一旦得到明确的上游任务 ID，后续查询固定回到原渠道，避免两个上游重复生成和重复扣费。若希望异步媒体由本站排队和转存，在“系统设置 → 模型设置 → 异步媒体”开启异步媒体并设置故障转移重试次数。

## OpenAI 下游调用示例

```bash
curl -X POST "https://your-gateway.example/v1/images/generations" \
  -H "Authorization: Bearer sk-your-token" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "public-image-model",
    "prompt": "一只像素风毛绒小恐龙",
    "aspect_ratio": "16:9",
    "image_size": "2K",
    "seed": 0
  }'
```

下游请求不需要知道最终选中了 Gemini 还是 OpenAI 兼容渠道。

## 常见问题

- 返回“没有可用渠道”：检查两个渠道的公开模型名、分组、状态和模型映射是否一致。
- Gemini 报 `Unknown name aspectRatio` 或 `Unknown name imageSize` 且路径位于 `responseFormat.image`：系统设置中选择“官方 imageConfig”。升级后旧的 `legacy` 配置值也会自动按正确的 `imageConfig` 处理。
- 自定义字段没有生效：确认路径使用 JSON 点路径、源字段确实存在。协议桥会强制执行转换，自定义映射与透传在内置转换之后应用。
- Gemini 返回模型不支持或 URL 异常：确认渠道类型为 `GPT2Gemini`，Base URL 使用 Gemini 官方根地址或只带一次 `/v1beta`；常见 Nano Banana 名称可自动转换，其他别名请在渠道“模型映射”中配置。
- 图片参考失败：确认 URL 可由服务端访问，且未被 SSRF、文件大小或 MIME 类型限制拦截。
- 视频查询失败：检查查询路径的 `{task_id}`，以及任务是否仍绑定创建它的原渠道。
- 修改渠道类型后行为未变化：保存渠道并重启后端；前端静态资源也需要重新构建或更新部署。

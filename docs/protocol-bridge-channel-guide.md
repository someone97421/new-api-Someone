# Gemini2GPT / GPT2Gemini 协议桥渠道使用说明

## 功能目的

协议桥渠道用于解决“下游只会一种请求格式，上游却来自不同协议”的问题。它们是渠道管理中的正式大类，不再绑定 Ease、鲸鱼或香蕉等供应商名称。

- `Gemini2GPT`：下游使用 Gemini `generateContent`，中转层转换为 OpenAI Chat / Images / Videos 兼容格式发送给上游，再把响应转换回 Gemini 格式。
- `GPT2Gemini`：下游使用 OpenAI Chat / Images / Videos 兼容格式，中转层转换为 Gemini `generateContent`、Imagen 或 Veo 格式发送给上游，再把响应转换回 OpenAI 格式。

适合管理员把同一个公开模型配置到多个不同协议的渠道中，并使用项目原有的分组、优先级、权重和重试机制完成故障转移。

## 新手配置步骤

1. 进入“渠道管理”，新建渠道。
2. 如果上游是 Gemini 官方或 Gemini 原生中转，类型选择 `GPT2Gemini`；如果上游接受 OpenAI 兼容请求，类型选择 `Gemini2GPT`。
3. 填写上游 Base URL 和密钥。Base URL 不带末尾 `/`。
4. 在“模型”填写下游公开模型名；若上游模型名不同，在“模型映射”填写映射关系。
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

OpenAI 图片请求转 Gemini 图片请求时，内置转换会把比例和分辨率写入 `generationConfig.responseFormat.image`，也支持系统设置中的旧版 `generationConfig.imageConfig`。HTTP 图片和 Data URI 会转换为 Gemini `inlineData`。Gemini 图片响应会转换为 OpenAI Images 响应。

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
  "aspect_ratio": "generationConfig.responseFormat.image.aspectRatio",
  "image_size": "generationConfig.responseFormat.image.imageSize",
  "duration": "parameters.durationSeconds",
  "generate_audio": "parameters.generateAudio",
  "extra.seedance": "parameters.vendorOptions"
}
```

执行顺序为：内置协议转换 → 自定义字段替换 → 自定义同名透传 → 渠道原有参数覆盖。后执行的规则可以覆盖前面的转换结果。鉴权、Base URL、目标 URL 等敏感根字段禁止作为映射目标。

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
- Gemini 报 `Unknown name responseFormat`：在系统设置的 Gemini 图片配置中切换旧版 `imageConfig`，或升级上游网关。
- 自定义字段没有生效：确认路径使用 JSON 点路径、源字段确实存在，且没有开启“直接透传请求体”。直接透传会绕过协议转换。
- 图片参考失败：确认 URL 可由服务端访问，且未被 SSRF、文件大小或 MIME 类型限制拦截。
- 视频查询失败：检查查询路径的 `{task_id}`，以及任务是否仍绑定创建它的原渠道。
- 修改渠道类型后行为未变化：保存渠道并重启后端；前端静态资源也需要重新构建或更新部署。

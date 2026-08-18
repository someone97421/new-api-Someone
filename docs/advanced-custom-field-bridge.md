# 高级自定义路由：自定义透传字段与字段替换

## 功能目的

高级自定义渠道可以按路由做协议转换。内置转换器会先把下游请求变成上游协议，未声明的扩展字段会在这一步丢失。

“自定义透传字段”和“自定义字段替换”挂在**每条路由**上，在内置转换之后、渠道参数覆盖之前，把下游原始 JSON 里的指定字段抄回上游请求。适合管理员对接供应商私有字段，而不必改代码或再开一套 Gemini2GPT / GPT2Gemini 渠道。

适用对象：需要配置高级自定义渠道的管理员。普通用户不直接接触这项设置。

## 解决的问题

协议转换（例如 OpenAI Chat → Gemini、OpenAI Chat → Claude）会按内置 DTO 重建请求体。下游带上的 `seed`、`extra.xxx`、供应商私有对象，转换后通常不在上游请求里。

渠道“参数覆盖”只能改**已经转换完成**的请求体，看不到下游原始字段，补不回这些值。同协议原样转发时，字段还在请求体里，参数覆盖的 `copy` / `move` 就够用；跨协议时必须用本功能。

## 在哪里配置

1. 进入“渠道管理”，新建或编辑类型为“高级自定义”的渠道。
2. 打开“高级自定义路由”编辑器。
3. 选择模板并填充，或手工添加路由。
4. 每条非 `/v1/models` 路由下方会出现两项：
   - 自定义透传字段
   - 自定义字段替换
5. 保存路由配置，再保存渠道。无需数据库迁移；已运行的后端进程需要重启后才会加载新代码。

官方模板（Official OpenAI Chat / Responses / Embeddings / Images、Official Claude Messages、Official Gemini Native、Official Gemini from OpenAI Chat）本身不预填规则。填充模板后，可在对应路由上自行填写。JSON 文本模式也可直接编辑 `passthrough_fields` 和 `field_mappings`。

`/v1/models` 发现路由不支持这两项，因为它不转发推理请求体。

## 新手步骤

1. 先配好转发行：下游路径、上游路径、转换器、鉴权。
2. 确认下游真实请求里有哪些扩展字段。可用一次实际调用的请求体对照。
3. 字段名和层级与上游相同时，写入“自定义透传字段”，每行一个 JSON 点路径。
4. 字段名或层级不同时，写入“自定义字段替换”：左边是下游源路径，右边是上游目标路径。
5. 保存后用同一条下游请求复测。上游抓包或渠道日志里应能看到抄过去的字段。

## 默认值与边界

- 默认不配置任何规则，行为与升级前一致。
- 每条路由最多 128 个透传字段、128 条字段替换。
- 路径必须是 `a` 或 `a.b.c` 这种点路径，只允许字母、数字、下划线和连字符。
- 映射目标不能指向 `authorization`、`api_key`、`apikey`、`access_token`、`base_url`、`url` 及其子路径。
- 只有下游请求里真实存在的值才会复制；显式 `0`、`false` 和空字符串会保留，缺失字段不会凭空补上。
- 不同路由的规则互不影响。同一入站路径按模型拆分时，只有命中的那条路由生效。

## 配置示例

OpenAI Chat 转到 Gemini，并保留图片比例和供应商扩展字段：

```json
{
  "advanced_routes": [
    {
      "incoming_path": "/v1/chat/completions",
      "upstream_path": "/v1beta/models/{model}:generateContent",
      "converter": "openai_chat_completions_to_gemini_generate_content",
      "auth": {
        "type": "query",
        "name": "key",
        "value": "{api_key}"
      },
      "passthrough_fields": [
        "seed",
        "vendor.camera"
      ],
      "field_mappings": {
        "aspect_ratio": "generationConfig.imageConfig.aspectRatio"
      }
    }
  ]
}
```

对应下游请求片段：

```json
{
  "model": "gemini-3-flash",
  "messages": [{ "role": "user", "content": "画一只像素小恐龙" }],
  "aspect_ratio": "16:9",
  "seed": 0,
  "vendor": { "camera": "orbit" }
}
```

转换完成后，上游 Gemini 请求会带上 `generationConfig.imageConfig.aspectRatio = "16:9"`、`seed = 0` 和 `vendor.camera = "orbit"`。

## 工作原理

执行顺序：

1. 按入站路径和模型选中一条高级自定义路由。
2. 内置转换器把下游协议转成上游协议。转换器为 `none` 时只做原协议适配。
3. 按该路由的字段替换，把下游源路径的值写到上游目标路径。
4. 按该路由的同名透传，把下游字段原路径抄到上游。
5. 再执行渠道原有的参数覆盖。

后执行的规则可以覆盖前面的结果。协议桥渠道（Gemini2GPT / GPT2Gemini）仍使用渠道级 `protocol_bridge` 配置，语义相同，只是不按路由拆分。

## 失败与降级

- 路径非法、条目超限或映射到受保护字段时，保存渠道会被拒绝。
- 运行时如果路由规则非法，请求会失败，不会静默忽略。
- 源字段不存在时跳过该条规则，不影响其余字段。
- 直接透传请求体时，带转换器的路由本身不可用；本功能也不会在那条路径上执行。

## 常见问题

- 填了规则但上游没看到字段：确认源路径写的是下游原始 JSON，而不是转换后的字段名；确认请求确实命中了配置该规则的路由和模型。
- 想改 HTTP 头：用渠道“参数覆盖”的 `pass_headers` / `set_header`，不要写进这两项。
- 同协议原样转发还要不要配：字段已经在请求体里时，通常用参数覆盖即可；只有转换器会丢掉字段时才需要本功能。
- 和 Gemini2GPT / GPT2Gemini 怎么选：只做 Gemini ↔ OpenAI 且不需要按路径拆路由时，继续用协议桥渠道的渠道级设置；需要多协议、多路径或按模型拆分时，用高级自定义路由级设置。

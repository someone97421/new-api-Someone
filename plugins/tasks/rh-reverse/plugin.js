// 模型来自 RH-Reverse 的模型注册表；同协议扩展只需在对应列表添加模型 ID。
// family ID 由 RH-Reverse 按参考素材选择具体模型；渠道 model_mapping 优先于下列通用别名。
const IMAGE_MODELS = [
  "dola-seedream5-pro",
  "dola-seedream5-pro-canvas",
  "dola-seedream5-pro-canvas-edit",
  "dola-seedream5-pro-canvas-text",
  "dola-seedream5-pro-edit",
  "dola-seedream5-pro-text",
  "gpt-image-2-5-flare",
  "gpt-image-2-5-flare-edit",
  "gpt-image-2-5-flare-low-cost",
  "gpt-image-2-5-flare-low-cost-edit",
  "gpt-image-2-5-flare-low-cost-text",
  "gpt-image-2-5-flare-text",
  "gpt-image-2-5-sunburst",
  "gpt-image-2-5-sunburst-edit",
  "gpt-image-2-5-sunburst-low-cost",
  "gpt-image-2-5-sunburst-low-cost-edit",
  "gpt-image-2-5-sunburst-low-cost-text",
  "gpt-image-2-5-sunburst-text",
  "gpt-image-2-low-cost",
  "gpt-image-2-low-cost-edit",
  "gpt-image-2-low-cost-text",
  "gpt-image-2-official",
  "gpt-image-2-official-edit",
  "gpt-image-2-official-text",
  "nano-banana-pro-low-cost",
  "nano-banana-pro-low-cost-edit",
  "nano-banana-pro-low-cost-text",
  "nano-banana-pro-official",
  "nano-banana-pro-official-edit",
  "nano-banana-pro-official-text",
  "nano-banana2-1",
  "nano-banana2-1-text",
  "nano-banana2-1-edit",
  "nano-banana2-lite-low-cost",
  "nano-banana2-lite-low-cost-edit",
  "nano-banana2-lite-low-cost-text",
  "nano-banana2-low-cost",
  "nano-banana2-low-cost-edit",
  "nano-banana2-low-cost-text",
  "nano-banana2-official",
  "nano-banana2-official-edit",
  "nano-banana2-official-text",
  "qwen3-0",
  "qwen3-0-edit",
  "qwen3-0-pro",
  "qwen3-0-pro-edit",
  "qwen3-0-pro-text",
  "qwen3-0-text",
  "seedream4-0",
  "seedream4-0-edit",
  "seedream4-0-text",
  "seedream4-6",
  "seedream4-6-edit",
  "seedream4-6-text",
  "seedream5-flash",
  "seedream5-flash-edit",
  "seedream5-flash-text",
  "seedream5-lite",
  "seedream5-lite-edit",
  "seedream5-lite-text",
  "seedream5-pro",
  "seedream5-pro-edit",
  "seedream5-pro-text",
];

const VIDEO_MODELS = [
  "gemini-omni-flash",
  "gemini-omni-flash-image",
  "gemini-omni-flash-text",
  "grok-imagine-1-5-low-cost",
  "grok-imagine-1-5-official",
  "grok-imagine-1-5-official-image",
  "hailuo-h3",
  "hailuo-h3-canvas-first-frame",
  "hailuo-h3-canvas-first-last-frame",
  "hailuo-h3-canvas-omni-reference",
  "hailuo-h3-canvas-text",
  "happyhorse-1-1",
  "happyhorse-1-1-image",
  "happyhorse-1-1-reference",
  "happyhorse-1-1-text",
  "kling-v4",
  "kling-v4-text",
  "kling-v4-image",
  "kling-v4-reference",
  "kling-v4-canvas",
  "kling-v4-canvas-text",
  "kling-v4-canvas-start",
  "kling-v4-canvas-start-end",
  "kling-v4-canvas-reference",
  "kling-o1",
  "kling-o1-first-last",
  "kling-o1-image",
  "kling-o1-reference",
  "kling-o3-pro",
  "kling-o3-pro-reference",
  "kling-o3-pro-text",
  "kling-o3-std",
  "kling-o3-std-reference",
  "kling-o3-std-text",
  "kling3-0-4k",
  "kling3-0-4k-image",
  "kling3-0-4k-text",
  "klingo3-4k",
  "klingo3-4k-image",
  "klingo3-4k-reference",
  "klingo3-4k-text",
  "klingv3-turbo-pro",
  "klingv3-turbo-pro-image",
  "klingv3-turbo-pro-text",
  "klingv3-turbo-std",
  "klingv3-turbo-std-image",
  "klingv3-turbo-std-text",
  "minimax-h3-max",
  "minimax-h3-max-canvas-first-last-frame",
  "minimax-h3-max-canvas-omni-reference",
  "minimax-h3-max-canvas-text",
  "minimax-h3-max-first-last-frame",
  "minimax-h3-rh-enhanced",
  "minimax-h3-rh-enhanced-canvas-first-frame",
  "minimax-h3-rh-enhanced-canvas-first-last-frame",
  "minimax-h3-rh-enhanced-canvas-omni-reference",
  "minimax-h3-rh-enhanced-canvas-text",
  "minimax-h3-rh-enhanced-image",
  "minimax-h3-rh-enhanced-reference",
  "minimax-h3-rh-enhanced-text",
  "seedance2",
  "seedance2-5",
  "seedance2-5-global-canvas",
  "seedance2-5-global-canvas-first-frame",
  "seedance2-5-global-canvas-first-last-frame",
  "seedance2-5-global-canvas-reference",
  "seedance2-5-global-canvas-text",
  "seedance2-5-global-canvas-video-edit",
  "seedance2-5-global-canvas-video-extend",
  "seedance2-fast",
  "seedance2-global",
  "seedance2-global-canvas",
  "seedance2-global-fast",
  "seedance2-global-fast-canvas",
  "seedance2-global-mini",
  "seedance2-global-mini-canvas",
  "seedance2-mini",
  "veo3-1-fast-low-cost",
  "veo3-1-fast-low-cost-image",
  "veo3-1-fast-low-cost-text",
  "veo3-1-fast-official",
  "veo3-1-fast-official-image",
  "veo3-1-fast-official-reference",
  "veo3-1-fast-official-text",
  "veo3-1-pro-low-cost",
  "veo3-1-pro-low-cost-image",
  "veo3-1-pro-low-cost-text",
  "video-depth-capture-canvas",
  "video-hd-upscale-canvas",
  "vidu-q4-preview",
  "vidu-q4-preview-canvas-image",
  "vidu-q4-preview-canvas-reference",
  "vidu-q4-preview-image",
  "vidu-q4-preview-reference",
  "wan2-7-spicy",
  "wan3-0",
  "wan3-0-canvas",
  "wan3-0-canvas-reference",
  "wan3-0-canvas-text",
];

// 别名只统一客户端名称，不改变供应商模型的能力；管理员可用渠道映射覆盖。
const IMAGE_ALIASES = {
  "gpt-image-2": "gpt-image-2-official",
  "gpt-image-2.5-flare": "gpt-image-2-5-flare",
  "seedream-5.0-pro": "seedream5-pro",
  "gpt-image-2.5-sunburst": "gpt-image-2-5-sunburst",
  "nano-banana-pro": "nano-banana-pro-official",
  "nano-banana-2": "nano-banana2-official",
  "nano-banana-2.1": "nano-banana2-1",
};
const VIDEO_ALIASES = {
  "kling-4.0": "kling-v4",
  "doubao-seedance-2-0-260128": "seedance2",
  "doubao-seedance-2-0-fast-260128": "seedance2-fast",
  "grok-imagine-video": "grok-imagine-1-5-low-cost",
};
const IMAGE_IDS = IMAGE_MODELS.concat(Object.keys(IMAGE_ALIASES));
const VIDEO_IDS = VIDEO_MODELS.concat(Object.keys(VIDEO_ALIASES));
const MAX_VIDEO_SECONDS = 3600; // 与宿主 relaycommon.MaxTaskDurationSeconds 一致
const REFERENCE_FIELDS = ["imageUrl", "imageUrls", "fileUrl", "fileUrls", "referenceUrl", "referenceUrls", "videoUrl", "videoUrls", "audioUrl", "audioUrls"];

export const meta = {
  apiVersion: 1,
  key: "rh-reverse",
  name: "RH-Reverse",
  icon: "text:RH",
  version: "1.0.0",
  author: { name: "new-api" },
  description: { en: "Image and video generation through a self-hosted RH-Reverse backend", zh: "通过自托管 RH-Reverse 后端生成图片和视频" },
  requiredCapabilities: ["json-clone@1"],
  models: IMAGE_IDS.concat(VIDEO_IDS),
  fetchMode: "per_task",
  auth: { type: "api_key" },
  upstreams: ["vendor", "new_api"],
  protocols: [
    { name: "openai_image", models: IMAGE_IDS },
    { name: "openai_video", models: VIDEO_IDS },
  ],
  routes: [
    { method: "POST", path: "/rh-reverse/api/tasks", type: "submit", decode: "createTask", render: "taskView" },
    { method: "GET", path: "/rh-reverse/api/tasks/:task_id", type: "query", render: "taskView" },
    { method: "GET", path: "/rh-reverse/api/tasks/:task_id/content", type: "content" },
    { method: "HEAD", path: "/rh-reverse/api/tasks/:task_id/content", type: "content" },
  ],
  // RH-Reverse 未提供可核验的统一秒数/积分消耗字段；按单个成品计费，不猜测时长或上游价格。
  usageSchema: {
    image_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张" },
      description: {
        en: "Image generation unit price",
        zh: "图片生成单价",
        "zh-TW": "圖片生成單價",
        fr: "Prix unitaire de génération d’image",
        ru: "Цена генерации изображения",
        ja: "画像生成単価",
        vi: "Đơn giá tạo ảnh",
      },
    },
  },
  usageProfiles: [
    {
      models: VIDEO_IDS,
      schema: {
        video_count: {
          type: "number",
          unit: "count",
          unitLabel: { en: "video", zh: "个" },
          description: {
            en: "Video generation unit price",
            zh: "视频生成单价",
            "zh-TW": "影片生成單價",
            fr: "Prix unitaire de génération de vidéo",
            ru: "Цена генерации видео",
            ja: "動画生成単価",
            vi: "Đơn giá tạo video",
          },
        },
      },
    },
  ],
};

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function object(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isVideo(ctx) {
  if (text(ctx.action)) return ctx.action.endsWith("_video");
  return VIDEO_IDS.includes(ctx.upstreamModel || ctx.model);
}

function upstreamModel(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  return IMAGE_ALIASES[model] || VIDEO_ALIASES[model] || model;
}

function taskPath(ctx) {
  return ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/rh-reverse" : "") + "/api/tasks";
}

function authHeaders(ctx) {
  return ctx.upstream && ctx.upstream.kind === "new_api" ? { Authorization: "Bearer " + ctx.apiKey } : { "X-API-Key": ctx.apiKey };
}

function validateTask(req) {
  const params = req.modelParams === undefined ? {} : req.modelParams;
  if (!object(params)) throw new Error("modelParams 必须是 JSON 对象");
  for (const values of [req, params]) {
    for (const key of ["n", "generateNum", "task_count", "batch_size"]) {
      if (values[key] !== undefined && values[key] !== 1) throw new Error(key + " 必须为 1，RH-Reverse 插件只支持单个成品");
    }
    for (const key of ["duration", "seconds"]) {
      // -1/0 是上游的视频自适应时长标记，不参与计费乘法。
      if (values[key] !== undefined && (!Number.isInteger(values[key]) || values[key] < -1 || values[key] > MAX_VIDEO_SECONDS))
        throw new Error(key + " 必须是 -1 到 3600 之间的整数");
    }
    if (values.draft !== undefined && values.draft !== false) throw new Error("暂不支持样片生成，请使用成品模式");
    if (values.draftTaskId !== undefined) throw new Error("暂不支持样片转正片任务");
  }
  for (const key of ["width", "height"]) {
    if (req[key] !== undefined && (!Number.isSafeInteger(req[key]) || req[key] <= 0)) throw new Error(key + " 必须是正整数");
  }
  if ((req.width === undefined) !== (req.height === undefined)) throw new Error("width 和 height 必须同时提供");
  for (const key of ["realPersonMode", "generateAudio"]) {
    if (req[key] !== undefined && typeof req[key] !== "boolean") throw new Error(key + " 必须是布尔值");
  }
  let references = 0;
  for (const key of REFERENCE_FIELDS) {
    if (req[key] === undefined) continue;
    const values = Array.isArray(req[key]) ? req[key] : [req[key]];
    for (const value of values) {
      if (!/^https?:\/\/[^\s]+$/i.test(text(value))) throw new Error(key + " 必须是 HTTP(S) URL，文件请使用 multipart 上传");
      references++;
    }
  }
  return references;
}

function decodeTask(ctx, media) {
  const body = ctx.body || {};
  let req;
  const uploads = [];
  if (body.kind === "json") {
    if (!object(body.value)) throw new Error("请求体必须是 JSON 对象");
    req = utils.json.clone(body.value);
  } else if (body.kind === "multipart" || body.kind === "form") {
    req = {};
    for (const key of Object.keys(body.fields || {})) {
      const values = body.fields[key];
      if (REFERENCE_FIELDS.includes(key) || ["image", "image[]", "image_urls"].includes(key)) {
        req[key] = [];
        for (const value of values) {
          const parsed = value.trim().startsWith("[") ? JSON.parse(value) : value;
          req[key] = req[key].concat(parsed);
        }
        continue;
      }
      if (values.length !== 1) throw new Error(key + " 只能提供一次");
      const value = values[0];
      if (["n", "duration", "seconds", "width", "height", "generateNum", "task_count", "batch_size"].includes(key)) req[key] = Number(value);
      else if (["modelParams", "metadata", "stream", "async", "enhance", "audio", "draft"].includes(key)) req[key] = JSON.parse(value);
      else req[key] = value;
    }
    for (const file of body.files || []) {
      const allowed = media === "video" ? ["input_reference", "image"] : media === "image" ? ["image", "image[]"] : ["image"];
      if (!allowed.includes(file.field)) throw new Error("不支持文件字段 " + file.field + "；使用 " + allowed.join(" 或 "));
      uploads.push({ ref: file.ref, filename: file.filename });
    }
  } else throw new Error("需要 JSON 或 multipart 请求体");
  const model = text(ctx.model) || text(req.model) || text(req.modelKey) || text(req.modelFamily);
  if (!model) throw new Error("必须提供 model 或 modelKey/modelFamily");
  if (!media) {
    if (!IMAGE_IDS.includes(model) && !VIDEO_IDS.includes(model)) throw new Error("模型未在 RH-Reverse 插件声明，请先添加模型 ID");
    media = VIDEO_IDS.includes(model) ? "video" : "image";
  } else {
    const selected = ctx.upstreamModel || model;
    if ((media === "image" && VIDEO_IDS.includes(selected)) || (media === "video" && IMAGE_IDS.includes(selected))) throw new Error("模型类型与请求接口不匹配");
    // OpenAI 的 model 是唯一模型选择来源，不能让扩展字段绕过渠道映射。
    delete req.modelKey;
    delete req.modelFamily;
    for (const pair of [
      ["aspect_ratio", "aspectRatio"],
      ["output_format", "outputFormat"],
      ["generate_audio", "generateAudio"],
      ["seconds", "duration"],
    ]) {
      const source = pair[0],
        target = pair[1];
      if (req[source] === undefined) continue;
      let value = req[source];
      if (source === "seconds" && typeof value === "string" && /^-?\d+$/.test(value)) value = Number(value);
      if (req[target] !== undefined && req[target] !== value) throw new Error(source + " 与 " + target + " 必须一致");
      req[target] = value;
      delete req[source];
    }
    for (const key of media === "image" ? ["image", "image[]", "images", "image_url", "image_urls"] : ["input_reference"]) {
      if (req[key] === undefined) continue;
      req.imageUrl = (req.imageUrl === undefined ? [] : Array.isArray(req.imageUrl) ? req.imageUrl : [req.imageUrl]).concat(req[key]);
      delete req[key];
    }
    if (req.size !== undefined && req.size !== "auto") {
      const match = /^(\d+)x(\d+)$/.exec(text(req.size));
      if (!match) throw new Error("size 必须是 WIDTHxHEIGHT 或 auto");
      const width = Number(match[1]),
        height = Number(match[2]);
      if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width <= 0 || height <= 0) throw new Error("size 必须包含正整数宽高");
      let a = width,
        b = height;
      while (b) {
        const remainder = a % b;
        a = b;
        b = remainder;
      }
      const ratio = width / a + ":" + height / a;
      if (media === "image") {
        // 大多数图片模型只接受分辨率档位和比例，不支持直接像素尺寸。
        // 标准尺寸转换为对应档位；其他尺寸交由支持自定义尺寸的上游模型校验。
        const tiers = { 1024: "1k", 2048: "2k", 4096: "4k" };
        const tier = tiers[Math.max(width, height)] || tiers[Math.min(width, height)];
        const resolution = tier || req.size;
        if (req.resolution !== undefined && req.resolution !== resolution) throw new Error("size 与 resolution 必须一致");
        if (req.aspectRatio !== undefined && req.aspectRatio !== ratio) throw new Error("size 与 aspectRatio 必须一致");
        req.resolution = resolution;
        req.aspectRatio = ratio;
      } else {
        const edge = Math.min(width, height);
        const resolution = { 480: "480p", 720: "720p", 1080: "1080p", 2160: "4k" }[edge];
        if (!resolution) throw new Error("该 size 无法转换为 RH-Reverse 视频分辨率，请使用 resolution 和 aspectRatio");
        if (req.aspectRatio !== undefined && req.aspectRatio !== ratio) throw new Error("size 与 aspectRatio 必须一致");
        if (req.resolution !== undefined && req.resolution !== resolution) throw new Error("size 与 resolution 必须一致");
        req.aspectRatio = ratio;
        req.resolution = resolution;
      }
    }
    delete req.size;
    if (media === "video") {
      // 不复制每个模型的参数表；特殊规格由请求指定并交给 RH-Reverse 校验。
      if (req.resolution === undefined) req.resolution = "720p";
      if (req.aspectRatio === undefined) req.aspectRatio = "16:9";
    }
  }
  if (req.stream !== undefined && req.stream !== false) throw new Error("此接口不支持流式生成");
  if (req.response_format !== undefined && !["url", "b64_json"].includes(req.response_format)) throw new Error("response_format 必须是 url 或 b64_json");
  if (req.mask !== undefined) throw new Error("RH-Reverse 不支持 mask");
  const references = validateTask(req);
  if (references + uploads.length > 50) throw new Error("最多支持 50 个参考文件");
  if (ctx.operation === "edit" && !references && !uploads.length) throw new Error("图片编辑必须提供参考图片");
  // 宿主字段不传给 FastAPI；其余扩展字段、modelParams 对象和重复素材字段原样转发。
  for (const key of ["model", "n", "stream", "response_format"]) delete req[key];
  const action = media === "video" ? "generate_video" : "generate_image";
  return { kind: "submit", model: model, action: action, requestBody: { payload: req, uploads: uploads } };
}

export function buildSubmitRequest(ctx) {
  const req = utils.json.clone(ctx.requestBody.payload);
  validateTask(req);
  const model = upstreamModel(ctx);
  // 防止 RH-Reverse 对未知 modelKey 回退到默认图片模型。
  if (!(isVideo(ctx) ? VIDEO_MODELS : IMAGE_MODELS).includes(model)) throw new Error("上游模型未声明，请添加模型 ID 或配置渠道模型映射");
  req.modelKey = model;
  delete req.modelFamily;
  const parts = [];
  for (const key of Object.keys(req)) {
    const value = req[key];
    if (value === undefined || value === null) continue;
    const values = REFERENCE_FIELDS.includes(key) && Array.isArray(value) ? value : [value];
    for (const item of values) parts.push({ name: key, value: typeof item === "object" ? JSON.stringify(item) : String(item) });
  }
  for (const file of ctx.requestBody.uploads || []) parts.push({ name: "image", fileRef: file.ref, filename: file.filename });
  return { url: taskPath(ctx), method: "POST", headers: authHeaders(ctx), bodyType: "multipart", parts: parts };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body;
  if (!object(body) || !text(body.id)) throw new Error("RH-Reverse 响应缺少任务 id");
  const parsed = parseTaskResult({ action: ctx.action, taskId: body.id }, body);
  return { taskId: body.id, taskData: body, immediate: ["SUCCESS", "FAILURE"].includes(parsed.status) ? parsed : undefined };
}

export function buildQueryRequest(ctx) {
  return { url: taskPath(ctx) + "/" + encodeURIComponent(ctx.taskId), method: "GET", headers: authHeaders(ctx) };
}

function outputURL(value) {
  const url = text(value);
  return /^https?:\/\/[^\s]+$/i.test(url) || /^\/(?!\/)[^\s]+$/.test(url);
}

export function parseTaskResult(ctx, body) {
  if (!object(body)) throw new Error("RH-Reverse 任务响应必须是 JSON 对象");
  const states = { queued: "QUEUED", running: "IN_PROGRESS", success: "SUCCESS", failed: "FAILURE" };
  let status = states[body.status] || "UNKNOWN";
  let reason = text(body.error);
  const url = text(body.outputUrl);
  if (status === "SUCCESS" && (!outputURL(url) || (body.draftResults || []).length)) {
    status = "FAILURE";
    reason = "RH-Reverse 未返回可用的成品 URL";
  }
  return {
    taskId: ctx.taskId || body.id,
    status: status,
    progress: status === "SUCCESS" ? "100%" : "0%",
    reason: reason,
    url: status === "SUCCESS" ? url : "",
  };
}

export function extractUsage(ctx) {
  validateTask(ctx.requestBody.payload);
  return isVideo(ctx) ? { video_count: 1 } : { image_count: 1 };
}

export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS" || parseTaskResult(ctx, body).status !== "SUCCESS") return {};
  return isVideo(ctx) ? { video_count: 1 } : { image_count: 1 };
}

export const native = {
  createTask: function (ctx) {
    return decodeTask(ctx, "");
  },
  taskView: function (ctx, task) {
    const data = utils.json.clone(task.data || {});
    data.id = task.task_id;
    data.status =
      { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "running", SUCCESS: "success", FAILURE: "failed" }[task.status] || "unknown";
    if (task.fail_reason) data.error = task.fail_reason;
    // 保留 TaskOut 外形，但不向下游暴露后端本地路径、调试记录和私有任务标识。
    for (const key of ["runninghubTaskId", "sourceTaskId", "inputPath", "inputPaths", "outputPath", "rawRecord"]) data[key] = null;
    return data;
  },
};

export const protocols = {
  openai_image: {
    decodeRequest: function (ctx) {
      return decodeTask(ctx, "image");
    },
    render: function (ctx, task) {
      const item = { artifact_key: "image" };
      if (/^https?:\/\//i.test(text(task.data.outputUrl))) item.url = task.data.outputUrl;
      return { data: [item] };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      return decodeTask(ctx, "video");
    },
    render: function (ctx, task) {
      const data = task.data || {};
      const result = {};
      if (data.outputUrl) result.url = data.outputUrl;
      if (Number.isInteger(data.duration) && data.duration > 0 && data.duration <= MAX_VIDEO_SECONDS) result.seconds = String(data.duration);
      if (/^\d+x\d+$/.test(text(data.outputSize))) result.size = data.outputSize;
      return result;
    },
  },
};

export function listArtifacts(task) {
  if (task.status !== "SUCCESS" || !outputURL((task.data || {}).outputUrl)) return [];
  const type = isVideo(task) ? "video" : "image";
  return [{ key: type, type: type }];
}

export function buildContentRequest(ctx) {
  const type = isVideo(ctx) ? "video" : "image";
  const url = (ctx.data || {}).outputUrl;
  if (ctx.artifactKey !== type || !outputURL(url)) throw new Error("artifact_not_found");
  if (/^https?:\/\//i.test(url)) return { url: url, method: ctx.clientRequest.method, credentialless: true };
  if (ctx.upstream && ctx.upstream.kind === "new_api")
    return {
      url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/tasks/" + encodeURIComponent(ctx.upstreamTaskId) + "/artifacts/" + type + "/content",
      method: ctx.clientRequest.method,
      headers: authHeaders(ctx),
    };
  return { url: ctx.baseUrl.replace(/\/+$/, "") + url, method: ctx.clientRequest.method, headers: authHeaders(ctx) };
}

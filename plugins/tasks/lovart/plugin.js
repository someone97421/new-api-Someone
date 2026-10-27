// 模型及别名来自 Any2API Lovart STATIC_REGISTRY；参数由上游模型 schema 校验。
const IMAGE_ALIASES = {
  midjourney: "youchuan/midjourney",
  "nano-banana-2": "vertex/nano-banana-2",
  "nano-banana-2.1": "vertex/nano-banana-2-1",
  "nano-banana-pro": "vertex/anon-bob",
  "flux-3-image": "bfl/flux-3-image",
  "gpt-image-2.5-flare": "openai/gpt-image-2-5-flare",
  "gpt-image-2.5-sunburst": "openai/gpt-image-2-5-sunburst",
  "qwen-image-3.0": "qwenimage/qwen-image-3-0",
  "qwen-image-3.0-pro": "qwenimage/qwen-image-3-0-pro",
  "seedream-5.0-pro": "seedream/seedream-5-0-pro",
  "luma-uni-1": "luma/uni-1",
  "luma-uni-1-max": "luma/uni-1-max",
};
const VIDEO_ALIASES = {
  "seedance-2.0": "seedance/seedance-2-0",
  "seedance-2.0-fast": "seedance/seedance-2-0-fast",
  "seedance-2.0-mini": "seedance/seedance-2-0-mini",
  "seedance-2.5": "seedance/seedance-2-5",
  "kling-3.0": "kling/kling-v3",
  "kling-3.0-omni": "kling/kling-v3-omni",
  "kling-4.0": "kling/kling-v4",
  "MiniMax-H3": "minimax/minimax-h3",
  "minimax-h3-max": "minimax/minimax-h3-max",
  "gemini-omni-1.1-flash": "vertex/gemini-omni-1-1-flash",
  "wan-3.0": "wan/wan-3-0",
  "wan-3.0-prime": "wan/wan-3-0-prime",
  "doubao-seedance-2-0-260128": "seedance/seedance-2-0",
  "doubao-seedance-2-0-fast-260128": "seedance/seedance-2-0-fast",
};
const IMAGE_MODELS = Object.values(IMAGE_ALIASES);
const VIDEO_MODELS = Array.from(new Set(Object.values(VIDEO_ALIASES)));
const IMAGE_IDS = IMAGE_MODELS.concat(Object.keys(IMAGE_ALIASES));
const VIDEO_IDS = VIDEO_MODELS.concat(Object.keys(VIDEO_ALIASES));
const IMAGE_PRICE = {
  en: "Image generation unit price",
  zh: "图片生成单价",
  "zh-TW": "圖片生成單價",
  fr: "Prix unitaire de génération d’image",
  ru: "Цена генерации изображения",
  ja: "画像生成単価",
  vi: "Đơn giá tạo ảnh",
};
const VIDEO_PRICE = {
  en: "Video generation unit price",
  zh: "视频生成单价",
  "zh-TW": "影片生成單價",
  fr: "Prix unitaire de génération de vidéo",
  ru: "Цена генерации видео",
  ja: "動画生成単価",
  vi: "Đơn giá tạo video",
};
export const meta = {
  apiVersion: 1,
  key: "lovart",
  name: "Lovart",
  icon: "text:Lo",
  version: "1.0.0",
  author: { name: "new-api" },
  description: { en: "Image and video generation through Any2API Lovart", zh: "通过 Any2API Lovart 后端生成图片和视频" },
  models: IMAGE_IDS.concat(VIDEO_IDS),
  fetchMode: "per_task",
  auth: { type: "api_key" },
  upstreams: ["vendor", "new_api"],
  routes: [
    { method: "POST", path: "/lovart/api/tasks", type: "submit", decode: "createTask", render: "taskView" },
    { method: "GET", path: "/lovart/api/tasks/:task_id", type: "query", render: "taskView" },
    { method: "GET", path: "/lovart/api/tasks/:task_id/content", type: "content" },
    { method: "HEAD", path: "/lovart/api/tasks/:task_id/content", type: "content" },
  ],
  protocols: [
    { name: "openai_image", models: IMAGE_IDS },
    { name: "openai_video", models: VIDEO_IDS },
  ],
  usageSchema: { image_count: { type: "number", unit: "count", description: IMAGE_PRICE } },
  usageProfiles: [{ models: VIDEO_IDS, schema: { video_count: { type: "number", unit: "count", description: VIDEO_PRICE } } }],
};
function text(value) {
  return typeof value === "string" ? value.trim() : "";
}
function object(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
function remote(value) {
  return /^https?:\/\/[^\s]+$/i.test(text(value));
}
function video(ctx) {
  return ctx.action ? ctx.action === "generate_video" : VIDEO_IDS.includes(ctx.upstreamModel || ctx.model);
}
function generator(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  return IMAGE_ALIASES[model] || VIDEO_ALIASES[model] || model;
}
function endpoint(ctx, path) {
  return ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/lovart" : "") + path;
}
function headers(ctx) {
  return ctx.upstream && ctx.upstream.kind === "new_api" ? { Authorization: "Bearer " + ctx.apiKey } : { "X-API-Key": ctx.apiKey };
}
function validate(args) {
  for (const key of ["n", "count", "generateNum", "numberOfGenerations", "batch_size", "task_count"]) {
    if (args[key] !== undefined && (!Number.isInteger(args[key]) || args[key] < 1 || args[key] > 128)) throw new Error(key + " 必须是 1 到 128 之间的整数");
  }
  for (const key of ["duration", "duration_seconds", "seconds"]) {
    if (args[key] === undefined) continue;
    const value = typeof args[key] === "string" && /^-?\d+$/.test(args[key]) ? Number(args[key]) : args[key];
    if (!Number.isInteger(value) || value < -1 || value > 3600) throw new Error(key + " 必须是 -1 到 3600 之间的整数");
  }
  if ((args.draft !== undefined && args.draft !== false) || text(args.mode) === "draft") throw new Error("暂不支持样片生成");
  for (const key of ["draft_task_id", "draftTaskId", "source_task_id"]) if (args[key] !== undefined) throw new Error("暂不支持样片转正片任务");
}
function decode(ctx, media) {
  const body = ctx.body || {},
    uploads = [];
  let req;
  if (body.kind === "json") {
    if (!object(body.value)) throw new Error("请求体必须是 JSON 对象");
    req = utils.json.clone(body.value);
  } else if (body.kind === "multipart") {
    req = {};
    for (const key of Object.keys(body.fields || {})) {
      const values = body.fields[key];
      if (["imageUrl", "imageUrls"].includes(key)) {
        req[key] = [];
        for (const value of values) req[key] = req[key].concat(value.trim().startsWith("[") ? JSON.parse(value) : value);
        continue;
      }
      if (values.length !== 1) throw new Error(key + " 只能提供一次");
      const value = values[0];
      if (["duration", "count", "n", "width", "height", "seconds"].includes(key)) req[key] = Number(value);
      else if (["inputArgs", "modelParams", "metadata", "stream", "attachToCanvas", "preModeration"].includes(key)) req[key] = JSON.parse(value);
      else req[key] = value;
    }
    for (const file of body.files || []) {
      if (!["image", "image[]", "input_reference"].includes(file.field)) throw new Error("不支持文件字段 " + file.field);
      uploads.push({ ref: file.ref, filename: file.filename });
    }
  } else throw new Error("需要 JSON 或 multipart 请求体");
  const model = text(ctx.model) || text(req.generatorName) || text(req.modelKey) || text(req.modelFamily);
  if (!model) throw new Error("必须提供 model 或 generatorName/modelKey/modelFamily");
  if (!media) {
    if (!IMAGE_IDS.includes(model) && !VIDEO_IDS.includes(model)) throw new Error("模型未在 Lovart 插件声明");
    media = VIDEO_IDS.includes(model) ? "video" : "image";
  }
  if (req.stream !== undefined && req.stream !== false) throw new Error("此接口不支持流式生成");
  if (req.mask !== undefined) throw new Error("Lovart 任务接口不支持 mask");
  const params = req.modelParams === undefined ? {} : req.modelParams;
  const input = req.inputArgs === undefined ? {} : req.inputArgs;
  if (!object(params) || !object(input)) throw new Error("inputArgs 和 modelParams 必须是 JSON 对象");
  const args = utils.json.clone(params),
    fields = {};
  for (const key of ["projectId", "attachToCanvas", "canvasProjectId", "imageUrl", "imageUrls"]) if (req[key] !== undefined) fields[key] = req[key];
  const mapping = { aspectRatio: "aspect_ratio", renderSpeed: "render_speed", preModeration: "pre_moderation" };
  for (const key of Object.keys(req)) {
    if (
      [
        "modelParams",
        "inputArgs",
        "generatorName",
        "modelKey",
        "modelFamily",
        "projectId",
        "attachToCanvas",
        "canvasProjectId",
        "imageUrl",
        "imageUrls",
        "stream",
        "response_format",
        "prompt",
      ].includes(key)
    )
      continue;
    if (ctx.model && key === "model") continue;
    args[mapping[key] || key] = req[key];
  }
  if (req.model === undefined && req.version !== undefined) args.model = req.version;
  delete args.version;
  if (ctx.model) {
    if (args.seconds !== undefined) {
      args.duration = args.seconds;
      delete args.seconds;
    }
    if (typeof args.duration === "string" && /^-?\d+$/.test(args.duration)) args.duration = Number(args.duration);
    for (const key of ["image", "image[]", "images", "image_url", "image_urls", "input_reference"]) {
      if (args[key] === undefined) continue;
      const urls = Array.isArray(args[key]) ? args[key] : [args[key]];
      for (const url of urls) if (!remote(url)) throw new Error("参考素材必须是 HTTP(S) URL，文件请使用 multipart 上传");
      fields.imageUrl = (fields.imageUrl || []).concat(urls);
      delete args[key];
    }
    if (args.size !== undefined && args.size !== "auto") {
      const match = /^(\d+)x(\d+)$/.exec(text(args.size));
      if (!match) throw new Error("size 必须是 WIDTHxHEIGHT 或 auto");
      const width = Number(match[1]),
        height = Number(match[2]);
      if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width < 1 || height < 1) throw new Error("size 必须包含正整数宽高");
      let a = width,
        b = height;
      while (b) {
        const remainder = a % b;
        a = b;
        b = remainder;
      }
      if (args.aspect_ratio === undefined) args.aspect_ratio = width / a + ":" + height / a;
      const selected = ctx.upstreamModel || model;
      if (selected.startsWith("qwenimage/") || selected.startsWith("qwen-image-") || selected.startsWith("openai/") || selected.startsWith("gpt-image-"))
        args.size = width + "*" + height;
      else {
        const resolution =
          media === "video"
            ? { 480: "480p", 720: "720p", 1080: "1080p", 2160: "4k" }[Math.min(width, height)]
            : { 512: "512", 1024: "1K", 2048: "2K", 4096: "4K" }[Math.max(width, height)];
        if (!resolution) throw new Error("此 size 无法转换，请使用 resolution 和 aspect_ratio");
        if (args.resolution === undefined) args.resolution = resolution;
        delete args.size;
      }
    }
  }
  for (const key of Object.keys(input)) args[key] = input[key];
  if (text(req.prompt)) args.prompt = req.prompt;
  validate(args);
  for (const key of ["imageUrl", "imageUrls"]) {
    if (fields[key] === undefined) continue;
    fields[key] = Array.isArray(fields[key]) ? fields[key] : [fields[key]];
    for (const url of fields[key]) if (!remote(url)) throw new Error(key + " 必须是 HTTP(S) URL");
  }
  if (
    ctx.operation === "edit" &&
    !uploads.length &&
    !(fields.imageUrl || []).length &&
    !(fields.imageUrls || []).length &&
    !["image", "images", "image_url", "source_image", "image_list"].some(function (key) {
      return args[key] !== undefined;
    })
  )
    throw new Error("图片编辑必须提供参考素材");
  return { kind: "submit", model: model, action: "generate_" + media, requestBody: { args: args, fields: fields, uploads: uploads } };
}
function finalArgs(ctx) {
  const args = utils.json.clone(ctx.requestBody.args);
  validate(args);
  return args;
}
export function buildSubmitRequest(ctx) {
  const selected = generator(ctx);
  if (!(video(ctx) ? VIDEO_MODELS : IMAGE_MODELS).includes(selected)) throw new Error("上游模型未声明，请配置渠道模型映射");
  const args = finalArgs(ctx),
    fields = ctx.requestBody.fields;
  const parts = [
    { name: "generatorName", value: selected },
    { name: "inputArgs", value: JSON.stringify(args) },
  ];
  for (const key of Object.keys(fields)) {
    const values = Array.isArray(fields[key]) ? fields[key] : [fields[key]];
    for (const value of values) parts.push({ name: key, value: typeof value === "object" ? JSON.stringify(value) : String(value) });
  }
  for (const file of ctx.requestBody.uploads || []) parts.push({ name: "image", fileRef: file.ref, filename: file.filename });
  return { url: endpoint(ctx, "/api/tasks"), method: "POST", headers: headers(ctx), bodyType: "multipart", parts: parts };
}
function outputs(body) {
  const result = [];
  for (let position = 0; position < ((body || {}).outputArtifacts || []).length; position++) {
    const item = body.outputArtifacts[position];
    if (!object(item)) continue;
    const type = text(item.type).split("/")[0];
    if (!["image", "video", "audio", "file"].includes(type)) continue;
    const index = position;
    if (
      !remote(item.content) &&
      !(typeof item.content === "string" && item.content.startsWith("/api/tasks/")) &&
      !(Number.isFinite(item.bytes) && item.bytes > 0) &&
      !text(item.outputPath)
    )
      continue;
    result.push({ key: type + "_" + position, type: type, index: index, item: item });
  }
  return result;
}
export function parseTaskResult(ctx, body) {
  if (!object(body)) throw new Error("Lovart 任务响应必须是 JSON 对象");
  let status = { queued: "QUEUED", running: "IN_PROGRESS", completed: "SUCCESS", failed: "FAILURE", stopped: "FAILURE" }[body.status] || "UNKNOWN";
  let reason = text(body.error);
  const items = outputs(body).filter(function (item) {
    return item.type === (video(ctx) ? "video" : "image");
  });
  if (status === "SUCCESS" && !items.length) {
    status = "FAILURE";
    reason = "Lovart 未返回可读取的成品";
  }
  return {
    taskId: ctx.taskId || body.id,
    status: status,
    progress: status === "SUCCESS" ? "100%" : "0%",
    reason: reason,
    url: items.length && remote(items[0].item.content) ? items[0].item.content : "",
  };
}
export function parseSubmitResponse(ctx, response) {
  const body = response.body;
  if (!object(body) || !text(body.id)) throw new Error("Lovart 响应缺少任务 id");
  const result = parseTaskResult(ctx, body);
  return { taskId: body.id, taskData: body, immediate: ["SUCCESS", "FAILURE"].includes(result.status) ? result : undefined };
}
export function buildQueryRequest(ctx) {
  return { url: endpoint(ctx, "/api/tasks/" + encodeURIComponent(ctx.taskId)), method: "GET", headers: headers(ctx) };
}
export function extractUsage(ctx) {
  const args = finalArgs(ctx);
  // inputArgs 覆盖 modelParams 与公共表单字段；Qwen/GPT 的 n 优先于 count。
  const count = args.n !== undefined ? args.n : args.count !== undefined ? args.count : generator(ctx) === "youchuan/midjourney" ? 4 : 1;
  return video(ctx) ? { video_count: count } : { image_count: count };
}
export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS") return {};
  const count = outputs(body).filter(function (item) {
    return item.type === (video(ctx) ? "video" : "image");
  }).length;
  if (count < 1 || count > 128) return {};
  return video(ctx) ? { video_count: count } : { image_count: count };
}
export function listArtifacts(task) {
  return task.status === "SUCCESS"
    ? outputs(task.data).map(function (item) {
        return { key: item.key, type: item.type };
      })
    : [];
}
export function buildContentRequest(ctx) {
  const output = outputs(ctx.data).find(function (item) {
    return item.key === ctx.artifactKey;
  });
  if (!output) throw new Error("artifact_not_found");
  if (remote(output.item.content)) return { url: output.item.content, method: ctx.clientRequest.method, credentialless: true };
  if (ctx.upstream && ctx.upstream.kind === "new_api")
    return {
      url:
        ctx.baseUrl.replace(/\/+$/, "") +
        "/v1/tasks/" +
        encodeURIComponent(ctx.upstreamTaskId) +
        "/artifacts/" +
        encodeURIComponent(ctx.artifactKey) +
        "/content",
      method: ctx.clientRequest.method,
      headers: headers(ctx),
    };
  return {
    url: endpoint(ctx, "/api/tasks/" + encodeURIComponent(ctx.upstreamTaskId) + "/outputs/" + output.index),
    method: ctx.clientRequest.method,
    headers: headers(ctx),
  };
}
export const native = {
  createTask: function (ctx) {
    return decode(ctx, "");
  },
  taskView: function (ctx, task) {
    const data = utils.json.clone(task.data || {});
    data.id = task.task_id;
    if (task.status === "FAILURE" && !["failed", "stopped"].includes(data.status)) data.status = "failed";
    if (!data.status) data.status = { QUEUED: "queued", IN_PROGRESS: "running", SUCCESS: "completed", FAILURE: "failed" }[task.status] || "queued";
    if (task.fail_reason) data.error = task.fail_reason;
    for (const key of ["lovartTaskId", "inputPaths", "outputPaths", "rawRecord"]) data[key] = null;
    for (const artifact of data.outputArtifacts || []) delete artifact.outputPath;
    return data;
  },
};
export const protocols = {
  openai_image: {
    decodeRequest: function (ctx) {
      return decode(ctx, "image");
    },
    render: function (ctx, task) {
      return {
        data: outputs(task.data)
          .filter(function (item) {
            return item.type === "image";
          })
          .map(function (output) {
            const item = { artifact_key: output.key };
            if (remote(output.item.content)) item.url = output.item.content;
            return item;
          }),
      };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      return decode(ctx, "video");
    },
    render: function (ctx, task) {
      const output = outputs(task.data).find(function (item) {
        return item.type === "video";
      });
      return output && remote(output.item.content) ? { url: output.item.content } : {};
    },
  },
};

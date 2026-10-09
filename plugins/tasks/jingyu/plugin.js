const MAX_IMAGE_N = 128; // 与宿主 dto.MaxImageN 一致

const IMAGE_MODELS = ["nano-banana-pro", "nano-banana-2", "gpt-image-2", "gpt-image-2-1k", "gpt-image-2-2k", "gpt-image-2-4k", "gpt-image-2.5-sunburst"];
const VIDEO_MODELS = [
  "starvideos_o3",
  "doubao-seedance-2-0-260128",
  "doubao-seedance-2-0-fast-260128",
  "LongXia-G-Seedance-2.0",
  "doubao-seedance-1-0-pro-250528",
  "doubao-seedance-1-0-lite-t2v",
  "doubao-seedance-1-0-lite-i2v",
  "doubao-seedance-1-5-pro-251215",
  "sora-2",
  "sora-2-pro",
  "kling-v1",
  "kling-v1-6",
  "kling-v2-master",
  "kling-3.0",
  "kling-3.0-omni",
  "viduq2",
  "viduq1",
  "vidu2.0",
  "vidu1.5",
  "seedance-api-2.0",
  "grok-imagine-video",
];
const VIDEO_USAGE_SCHEMA = {
  video_count: { type: "number", unit: "count", unitLabel: { en: "video", zh: "个" }, description: { en: "Video generation unit price", zh: "视频生成单价" } },
  seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  resolution: { enum: ["auto", "480p", "720p", "1080p", "2k", "4k"], description: { en: "Output video resolution", zh: "输出视频分辨率" } },
  audio: { enum: ["auto", "enabled", "disabled"], description: { en: "Audio generation mode", zh: "音频生成模式" } },
};
const MAX_VIDEO_SECONDS = 3600; // 与宿主 relaycommon.MaxTaskDurationSeconds 一致

export const meta = {
  apiVersion: 1,
  key: "jingyu",
  name: "鲸鱼 AI",
  icon: "text:鲸鱼",
  version: "1.2.0",
  author: { name: "new-api" },
  website: "https://jingyuapi.art",
  baseUrl: "https://jingyuapi.art",
  description: { en: "Image and video generation via Jingyu AI", zh: "通过鲸鱼 AI 生成图片和视频" },
  requiredCapabilities: ["json-clone@1"],
  models: IMAGE_MODELS.concat(VIDEO_MODELS),
  fetchMode: "per_task",
  auth: { type: "api_key" },
  protocols: [
    { name: "openai_image", models: IMAGE_MODELS },
    { name: "openai_video", models: VIDEO_MODELS },
  ],
  upstreams: ["vendor", "new_api"],
  routes: [
    { method: "POST", path: "/jingyu/v1/image/generations", models: IMAGE_MODELS, type: "submit", decode: "createImage", render: "imageCreated" },
    { method: "GET", path: "/jingyu/v1/image/generations/:task_id", type: "query", render: "imageStatus" },
    { method: "POST", path: "/jingyu/v1/video/generations", type: "submit", decode: "createVideo", render: "videoTask" },
    { method: "GET", path: "/jingyu/v1/video/generations/:task_id", type: "query", render: "videoTask" },
    { method: "GET", path: "/jingyu/v1/video/generations/:task_id/content", type: "content" },
    { method: "HEAD", path: "/jingyu/v1/video/generations/:task_id/content", type: "content" },
  ],
  usageSchema: {
    image_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张" },
      description: { en: "Image generation unit price", zh: "图片生成单价" },
    },
  },
  usageProfiles: [{ models: VIDEO_MODELS, schema: VIDEO_USAGE_SCHEMA }],
};

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function taskPayload(body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("Jingyu response must be a JSON object");
  if (body.error) throw new Error(text(body.error.message) || text(body.message) || "Jingyu request failed");
  if (body.code !== undefined && body.code !== "success" && body.code !== 0 && body.code !== 200)
    throw new Error(text(body.message) || "Jingyu request failed");
  return body.data && typeof body.data === "object" && !Array.isArray(body.data) ? body.data : body;
}

// results / result_urls / result_asset_urls 是同一组图片的不同表示，不能累加计数。
function imageEntries(body) {
  const payload = taskPayload(body);
  let source = payload.results;
  if (!Array.isArray(source)) source = body.data;
  if (source && !Array.isArray(source)) source = [source];
  const entries = [];
  for (const item of source || []) {
    if (!item || typeof item !== "object") continue;
    const url = text(item.url);
    const encoded = text(item.b64_json);
    if (!url && !encoded) continue;
    const entry = {};
    if (url) entry.url = url;
    if (encoded) entry.b64_json = encoded;
    if (text(item.revised_prompt)) entry.revised_prompt = item.revised_prompt;
    entries.push(entry);
  }
  if (entries.length) return entries;
  const urls = Array.isArray(payload.result_urls) && payload.result_urls.some(text) ? payload.result_urls : payload.result_asset_urls;
  for (const url of urls || []) {
    if (text(url)) entries.push({ url: text(url) });
  }
  return entries;
}

function imageUsage(body) {
  const entries = imageEntries(body);
  const count = Math.max(
    entries.filter(function (item) {
      return item.url;
    }).length,
    entries.filter(function (item) {
      return item.b64_json;
    }).length
  );
  if (count < 1 || count > MAX_IMAGE_N) return {};
  const payload = taskPayload(body);
  const usage = payload.usage || body.usage || {};
  const reported = usage.image_count === undefined ? usage.output_image_count : usage.image_count;
  // 有上游计数时只接受与图片载荷相符的有界整数，否则保留预留数量。
  if (reported !== undefined && (!Number.isInteger(reported) || reported < 1 || reported > MAX_IMAGE_N || reported !== count)) return {};
  return { image_count: reported === undefined ? count : reported };
}

function validateSingleTask(req) {
  for (const key of ["n", "task_count"]) {
    if (req[key] !== undefined && req[key] !== 1) throw new Error(key + " must be 1; Jingyu multi-task batches are not supported by this plugin");
  }
}

function decodeImageRequest(ctx, openAI) {
  let req;
  const body = ctx.body || {};
  if (body.kind === "json") {
    if (!body.value || typeof body.value !== "object" || Array.isArray(body.value)) throw new Error("request body must be a JSON object");
    req = utils.json.clone(body.value);
  } else if (body.kind === "multipart") {
    if ((body.files || []).length) throw new Error("Jingyu file uploads are not supported yet; provide HTTP(S) references in image_urls or images");
    req = {};
    for (const key of Object.keys(body.fields || {})) {
      const values = body.fields[key];
      if (values.length !== 1) throw new Error(key + " must be provided once");
      const value = values[0];
      // 保留扩展对象、数组、显式零值和 false，普通字段仍是字符串。
      if (["n", "task_count", "seed"].includes(key)) req[key] = Number(value);
      else if (value === "true" || value === "false" || value.trim().startsWith("[") || value.trim().startsWith("{")) {
        try {
          req[key] = JSON.parse(value);
        } catch (e) {
          throw new Error(key + " must contain valid JSON", { cause: e });
        }
      } else req[key] = value;
    }
  } else throw new Error("JSON or multipart body required");
  const model = text(ctx.model) || text(req.model);
  if (!model) throw new Error("model is required");
  if (!text(req.prompt)) throw new Error("prompt is required");
  validateSingleTask(req);
  if (req.stream !== undefined && req.stream !== false) throw new Error("stream is not supported; images are returned when generation completes");
  if (req["async"] !== undefined && typeof req["async"] !== "boolean") throw new Error("async must be a boolean");
  if (req.response_format !== undefined && !["url", "b64_json"].includes(req.response_format)) throw new Error("response_format must be url or b64_json");
  if (req.mask !== undefined) throw new Error("mask is not supported by Jingyu");
  const references = [];
  for (const key of ["image_urls", "images", "image", "image_url"]) {
    if (req[key] === undefined) continue;
    const values = Array.isArray(req[key]) ? req[key] : [req[key]];
    for (const value of values) {
      if (!/^https?:\/\/[^\s]+$/i.test(text(value))) throw new Error(key + " must contain HTTP(S) URLs; Base64 and data URLs are not supported by Jingyu");
      if (!references.includes(text(value))) references.push(text(value));
    }
  }
  if (ctx.operation === "edit" && !references.length) throw new Error("image references are required for image editing");
  req.model = model;
  // 原生入口保留供应商字段及 async=false；OpenAI 入口负责同步等待转换。
  if (!openAI) return { kind: "submit", model: model, action: references.length ? "image_to_image" : "text_to_image", requestBody: req };
  // 文档使用 image_urls；用户实际调用也验证了 images。统一转换到文档字段。
  delete req.images;
  delete req.image;
  delete req.image_url;
  if (references.length) req.image_urls = references;
  if (req.size !== undefined) {
    if (req.aspect_ratio === undefined && req.size !== "auto") req.aspect_ratio = req.size;
    delete req.size;
  }
  if (req.image_size === undefined && req.resolution !== undefined) req.image_size = req.resolution;
  // 宿主同步等待任务结果；上游始终使用异步，兼容只支持队列的 gpt-image-2。
  req.model = model;
  req["async"] = true;
  req.task_count = 1;
  req.response_format = "url";
  delete req.n;
  delete req.stream;
  return { kind: "submit", model: model, action: references.length ? "image_to_image" : "text_to_image", requestBody: req };
}

export function buildSubmitRequest(ctx) {
  if (isVideo(ctx)) {
    const body = utils.json.clone(ctx.requestBody.payload);
    validateVideo(body, ctx.upstreamModel || ctx.model);
    body.model = ctx.upstreamModel || ctx.model;
    const descriptor = { url: videoPath(ctx), method: "POST", headers: { Authorization: "Bearer " + ctx.apiKey } };
    const uploads = ctx.requestBody.uploads || [];
    if (uploads.length) {
      descriptor.bodyType = "multipart";
      descriptor.parts = [{ name: "request", value: JSON.stringify(body) }];
      for (const upload of uploads) descriptor.parts.push({ name: upload.key, fileRef: upload.ref, filename: upload.filename });
    } else {
      descriptor.headers["Content-Type"] = "application/json";
      descriptor.body = body;
    }
    return descriptor;
  }
  const body = utils.json.clone(ctx.requestBody);
  validateSingleTask(body);
  body.model = ctx.upstreamModel || ctx.model;
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/jingyu" : "") + "/v1/image/generations",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json" },
    body: body,
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body;
  const payload = taskPayload(body);
  const taskId = text(payload.task_id) || text(payload.id);
  if (Array.isArray(payload.task_ids) && payload.task_ids.length > 1) throw new Error("Jingyu unexpectedly returned multiple tasks");
  if (taskId) {
    const parsed = isVideo(ctx) ? parseTaskResult(ctx, body) : null;
    return { taskId: taskId, taskData: body, immediate: parsed && ["SUCCESS", "FAILURE"].includes(parsed.status) ? parsed : undefined };
  }
  if (isVideo(ctx)) throw new Error("Jingyu video response has no task_id");
  // 兼容上游立即完成的图片响应，不伪造可轮询的上游任务 ID。
  const images = imageEntries(body);
  if (!images.length) throw new Error("Jingyu response has no task_id or image output");
  return { taskId: ctx.publicTaskId, taskData: body, immediate: { status: "SUCCESS", progress: "100%", url: images[0].url || "" } };
}

export function buildQueryRequest(ctx) {
  if (isVideo(ctx)) return { url: videoPath(ctx) + "/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
  return {
    url:
      ctx.baseUrl.replace(/\/+$/, "") +
      (ctx.upstream && ctx.upstream.kind === "new_api" ? "/jingyu" : "") +
      "/v1/image/generations/" +
      encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey },
  };
}

export function parseTaskResult(ctx, body) {
  const payload = taskPayload(body);
  const states = {
    not_start: "NOT_START",
    submitted: "SUBMITTED",
    queued: "QUEUED",
    pending: "QUEUED",
    in_progress: "IN_PROGRESS",
    processing: "IN_PROGRESS",
    running: "IN_PROGRESS",
    success: "SUCCESS",
    succeeded: "SUCCESS",
    completed: "SUCCESS",
    failure: "FAILURE",
    failed: "FAILURE",
    cancelled: "FAILURE",
    canceled: "FAILURE",
  };
  let status = states[text(payload.status).toLowerCase()] || "UNKNOWN";
  let reason = text(payload.fail_reason) || text(payload.error && payload.error.message);
  if (isVideo(ctx)) {
    const rawProgress = Number(String(payload.progress === undefined ? 0 : payload.progress).replace(/%$/, ""));
    const progress = status === "SUCCESS" ? 100 : Number.isFinite(rawProgress) ? Math.min(100, Math.max(0, rawProgress)) : 0;
    // 已完成视频即使未提供公开 URL，仍可从带鉴权的 /content 下载。
    return {
      taskId: ctx.taskId,
      status: status,
      progress: progress + "%",
      reason: reason,
      url: text(payload.url) || text(payload.metadata && payload.metadata.url),
    };
  }
  const images = imageEntries(body);
  if (status === "SUCCESS" && !images.length) {
    status = "FAILURE";
    reason = "Jingyu marked the task successful but returned no image output";
  }
  const rawProgress = Number(String(payload.progress === undefined ? 0 : payload.progress).replace(/%$/, ""));
  const progress = status === "SUCCESS" ? 100 : Number.isFinite(rawProgress) ? Math.min(100, Math.max(0, rawProgress)) : 0;
  return { taskId: ctx.taskId, status: status, progress: progress + "%", reason: reason, url: images.length ? images[0].url || "" : "" };
}

export function extractUsage(ctx) {
  if (isVideo(ctx)) {
    const req = ctx.requestBody.payload;
    validateVideo(req, ctx.upstreamModel || ctx.model);
    const facts = videoFacts(req, ctx.upstreamModel || ctx.model);
    if (ctx.usagePurpose === "billing_ratios") return { seconds: facts.seconds };
    return facts;
  }
  validateSingleTask(ctx.requestBody);
  return { image_count: 1 };
}

export function extractUsageOnSubmit(ctx, body) {
  if (isVideo(ctx)) return {};
  return imageUsage(body);
}

export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS") return {};
  if (isVideo(ctx)) {
    const payload = taskPayload(body);
    const facts = { video_count: 1 };
    // 不用缺失字段覆盖冻结的估算；拒绝异常时长而非猜测视频 token/扣费。
    if (Number.isFinite(payload.duration) && payload.duration > 0 && payload.duration <= MAX_VIDEO_SECONDS) facts.seconds = payload.duration;
    const resolution = text(payload.resolution).toLowerCase();
    if (VIDEO_USAGE_SCHEMA.resolution.enum.includes(resolution)) facts.resolution = resolution;
    if (typeof payload.generate_audio === "boolean") facts.audio = payload.generate_audio ? "enabled" : "disabled";
    return facts;
  }
  return imageUsage(body);
}

export const native = {
  createVideo: function (ctx) {
    return decodeVideoRequest(ctx, false);
  },
  videoTask: function (ctx, task) {
    return videoTaskView(task, false);
  },
  createImage: function (ctx) {
    return decodeImageRequest(ctx, false);
  },
  imageCreated: function (ctx, task) {
    // 同步成功保持鲸鱼 ImageResponse；异步返回宿主管理的公开任务 ID。
    if (task.status === "SUCCESS") return task.data;
    const data = utils.json.clone(task.data || {});
    data.id = task.task_id;
    data.task_id = task.task_id;
    data.status = task.status === "SUBMITTED" ? "queued" : task.status.toLowerCase();
    if (data.created === undefined) data.created = task.created_at;
    return data;
  },
  imageStatus: function (ctx, task) {
    // 宿主超时/轮询失败可能已终止任务；不能继续回显旧上游快照的进行中状态。
    const snapshot = task.data || {};
    const payload = snapshot.data && typeof snapshot.data === "object" && !Array.isArray(snapshot.data) ? snapshot.data : snapshot;
    const data = utils.json.clone(payload);
    data.task_id = task.task_id;
    data.status = task.status;
    data.progress = task.progress || "0%";
    if (task.fail_reason) data.fail_reason = task.fail_reason;
    if (task.status === "SUCCESS" && !Array.isArray(data.results)) data.results = imageEntries(snapshot);
    return { code: "success", message: "", data: data };
  },
};

export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      return decodeVideoRequest(ctx, true);
    },
    render: function (ctx, task) {
      return videoTaskView(task, true);
    },
  },
  openai_image: {
    decodeRequest: function (ctx) {
      return decodeImageRequest(ctx, true);
    },
    render: function (ctx, task) {
      const data = imageEntries(task.data);
      if (!data.length) throw new Error("Jingyu returned no image output");
      return { created: task.created_at, data: data };
    },
  },
};

function isVideo(ctx) {
  return ["text_to_video", "image_to_video", "video_to_video"].includes(ctx.action) || VIDEO_MODELS.includes(ctx.upstreamModel || ctx.model);
}

function videoPath(ctx) {
  return ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/jingyu" : "") + "/v1/video/generations";
}

function videoFacts(req, model) {
  const hasVideo = (req.references || []).some(function (ref) {
    return ref.type === "video";
  });
  let seconds = req.duration;
  // 已发布默认值。参考视频决定 StarVideos 输出时长时，按允许的最高 10 秒预留。
  if (seconds === undefined && model === "starvideos_o3") seconds = hasVideo ? 10 : 3;
  if (seconds === undefined && (model.startsWith("kling-") || model.startsWith("vidu"))) seconds = 5;
  if (seconds === undefined && ["sora-2", "sora-2-pro"].includes(model)) seconds = 15;
  if (seconds === undefined) throw new Error("duration is required for this Jingyu video model because its default duration is not documented");
  let resolution = text(req.resolution).toLowerCase();
  if (!resolution && model.startsWith("doubao-seedance-2-0")) resolution = "720p";
  if (!resolution && model.startsWith("vidu")) resolution = "1080p";
  return {
    video_count: 1,
    seconds: seconds,
    resolution: resolution || "auto",
    audio: req.generate_audio === undefined ? "auto" : req.generate_audio ? "enabled" : "disabled",
  };
}

function validateVideo(req, model) {
  validateSingleTask(req);
  for (const key of ["duration", "seconds"]) {
    if (req[key] !== undefined && (!Number.isInteger(req[key]) || req[key] < 1 || req[key] > MAX_VIDEO_SECONDS))
      throw new Error(key + " must be an integer between 1 and 3600");
  }
  if (req.seconds !== undefined && req.duration !== undefined && req.seconds !== req.duration) throw new Error("seconds and duration must agree");
  if (req.stream !== undefined && req.stream !== false) throw new Error("stream is not supported for Jingyu video tasks");
  if (req["async"] !== undefined && req["async"] !== true) throw new Error("Jingyu videos are asynchronous; async must be true");
  if (req.generate_audio !== undefined && typeof req.generate_audio !== "boolean") throw new Error("generate_audio must be a boolean");
  if (req.resolution !== undefined && !VIDEO_USAGE_SCHEMA.resolution.enum.includes(text(req.resolution).toLowerCase()))
    throw new Error("unsupported video resolution");
  if (req.references !== undefined && !Array.isArray(req.references)) throw new Error("references must be an array");
  for (const ref of req.references || []) {
    if (!ref || typeof ref !== "object" || !["image", "video", "audio"].includes(ref.type) || !text(ref.role))
      throw new Error("references must specify image/video/audio type and role");
    if (ref.asset_id !== undefined || ref.resource_id !== undefined)
      throw new Error("provider asset IDs and subject IDs are not supported; use URLs or uploaded files");
    if ((ref.url !== undefined) === (ref.file_key !== undefined)) throw new Error("each reference must provide exactly one URL or file_key");
    if (ref.url !== undefined && !/^https?:\/\/[^\s]+$/i.test(text(ref.url))) throw new Error("reference URLs must use HTTP(S)");
    if (ref.file_key !== undefined && !text(ref.file_key)) throw new Error("file_key must be non-empty");
  }
  const facts = videoFacts(req, model);
  if (model === "starvideos_o3") {
    const hasVideo = (req.references || []).some(function (ref) {
      return ref.type === "video";
    });
    if (hasVideo && (req.duration !== undefined || req.aspect_ratio !== undefined))
      throw new Error("StarVideos reference video determines duration and aspect_ratio; omit both fields");
    if (!hasVideo && (facts.seconds < 3 || facts.seconds > 15)) throw new Error("StarVideos duration must be between 3 and 15 seconds");
  }
  if (model === "grok-imagine-video" && (facts.seconds < 6 || facts.seconds > 30)) throw new Error("Grok duration must be between 6 and 30 seconds");
}

function decodeVideoRequest(ctx, openAI) {
  const body = ctx.body || {};
  const inputRole = (ctx.upstreamModel || ctx.model || (body.value || {}).model) === "starvideos_o3" ? "reference_image" : "first_frame";
  let req;
  const uploads = [];
  if (body.kind === "json") {
    if (!body.value || typeof body.value !== "object" || Array.isArray(body.value)) throw new Error("request must be a JSON object");
    req = utils.json.clone(body.value);
  } else if (body.kind === "multipart") {
    req = {};
    const fields = body.fields || {};
    if (!openAI) {
      if (!fields.request || fields.request.length !== 1) throw new Error("native multipart must provide one request JSON field");
      req = JSON.parse(fields.request[0]);
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request must be a JSON object");
      for (const file of body.files || []) uploads.push({ key: file.field, ref: file.ref, filename: file.filename });
    } else {
      for (const key of Object.keys(fields)) {
        if (fields[key].length !== 1) throw new Error(key + " must be provided once");
        const value = fields[key][0];
        if (["seconds", "duration", "seed", "n", "task_count"].includes(key)) req[key] = Number(value);
        else if (value === "true" || value === "false" || value.trim().startsWith("[") || value.trim().startsWith("{")) req[key] = JSON.parse(value);
        else req[key] = value;
      }
      for (const file of body.files || []) {
        if (file.field !== "input_reference" || uploads.length || req.input_reference !== undefined)
          throw new Error("provide only one input_reference image file or URL");
        uploads.push({ key: "input_reference", ref: file.ref, filename: file.filename });
        req.references = (req.references || []).concat([{ type: "image", role: inputRole, file_key: "input_reference" }]);
      }
    }
  } else throw new Error("JSON or multipart body required");
  const model = text(ctx.model) || text(req.model);
  if (!model) throw new Error("model is required");
  if (!openAI && !VIDEO_MODELS.includes(model)) throw new Error("model is not supported on the Jingyu video route");
  if (!text(req.prompt)) throw new Error("prompt is required");
  req.model = model;
  if (openAI) {
    if (req.seconds !== undefined) {
      const seconds = typeof req.seconds === "string" && /^\d+$/.test(req.seconds) ? Number(req.seconds) : req.seconds;
      if (req.duration !== undefined && req.duration !== seconds) throw new Error("seconds and duration must agree");
      req.duration = seconds;
      delete req.seconds;
    }
    if (req.input_reference !== undefined) {
      if (!/^https?:\/\/[^\s]+$/i.test(text(req.input_reference))) throw new Error("input_reference must be an HTTP(S) image URL or multipart file");
      req.references = (req.references || []).concat([{ type: "image", role: inputRole, url: req.input_reference }]);
      delete req.input_reference;
    }
    if (req.size !== undefined) {
      const match = /^(\d+)x(\d+)$/.exec(text(req.size));
      if (!match || !Number.isSafeInteger(Number(match[1])) || !Number.isSafeInteger(Number(match[2])) || Number(match[1]) < 1 || Number(match[2]) < 1)
        throw new Error("size must use positive integer WIDTHxHEIGHT");
      const width = Number(match[1]),
        height = Number(match[2]);
      let a = width,
        b = height;
      while (b) {
        const next = a % b;
        a = b;
        b = next;
      }
      let ratio = width / a + ":" + height / a;
      const shortEdge = Math.min(width, height);
      let resolution = shortEdge <= 480 ? "480p" : shortEdge <= 720 ? "720p" : shortEdge <= 1080 ? "1080p" : shortEdge <= 2048 ? "2k" : "4k";
      // OpenAI Sora Pro 的 1792x1024 / 1024x1792 是固定规格，供应商以 16:9 / 9:16 表达。
      if (["1792x1024", "1024x1792"].includes(req.size)) {
        ratio = width > height ? "16:9" : "9:16";
        resolution = "1080p";
      }
      if (req.aspect_ratio !== undefined && req.aspect_ratio !== ratio) throw new Error("size and aspect_ratio must agree");
      if (req.resolution !== undefined && text(req.resolution).toLowerCase() !== resolution) throw new Error("size and resolution must agree");
      req.aspect_ratio = ratio;
      req.resolution = resolution;
      delete req.size;
    }
  } else if (req.seconds !== undefined || req.input_reference !== undefined || req.size !== undefined)
    throw new Error("native video requests use duration, references and aspect_ratio/resolution");
  // 已停用的旧素材字段不能当作未知扩展静默透传并丢失参考素材。
  for (const key of ["image", "image_url", "images", "video_url", "metadata", "video_config"]) {
    if (req[key] !== undefined) throw new Error(key + " is not supported; use references and provider-specific extra");
  }
  validateVideo(req, ctx.upstreamModel || model);
  for (const ref of req.references || []) {
    if (
      ref.file_key !== undefined &&
      !uploads.some(function (file) {
        return file.key === ref.file_key;
      })
    )
      throw new Error("reference file_key does not match an uploaded file");
  }
  for (const file of uploads) {
    if (
      !(req.references || []).some(function (ref) {
        return ref.file_key === file.key;
      })
    )
      throw new Error("every uploaded file must be bound by references[].file_key");
  }
  const action = (req.references || []).some(function (ref) {
    return ref.type === "video";
  })
    ? "video_to_video"
    : (req.references || []).length
      ? "image_to_video"
      : "text_to_video";
  return { kind: "submit", model: model, action: action, requestBody: { payload: req, uploads: uploads } };
}

function videoTaskView(task, openAI) {
  const data = utils.json.clone(taskPayload(task.data || {}));
  const states = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "processing", SUCCESS: "succeeded", FAILURE: "failed" };
  data.task_id = task.task_id;
  data.object = "video.task";
  data.status = states[task.status] || "unknown";
  const progress = Number(String(task.progress || "0").replace(/%$/, ""));
  data.progress = Number.isFinite(progress) ? progress : 0;
  if (data.created_at === undefined) data.created_at = task.created_at;
  if (task.fail_reason) data.error = { message: task.fail_reason };
  if (openAI) {
    if (data.duration !== undefined) data.seconds = String(data.duration);
    if (data.aspect_ratio && data.resolution) {
      const match = /^(\d+):(\d+)$/.exec(data.aspect_ratio);
      const edge = { "480p": 480, "720p": 720, "1080p": 1080, "2k": 2048, "4k": 4096 }[text(data.resolution).toLowerCase()];
      if (match && edge) {
        const w = Number(match[1]),
          h = Number(match[2]);
        if (w > 0 && h > 0) data.size = Math.round((edge * w) / Math.min(w, h)) + "x" + Math.round((edge * h) / Math.min(w, h));
      }
    }
  }
  return data;
}

export function listArtifacts(task) {
  return isVideo(task) && task.status === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video" || !isVideo(ctx)) throw new Error("artifact_not_found");
  const headers = { Authorization: "Bearer " + ctx.apiKey };
  for (const key of ["Range", "If-Range", "If-None-Match", "If-Modified-Since"]) {
    const value = (ctx.clientRequest.headers || {})[key];
    if (value) headers[key] = value;
  }
  return { url: videoPath(ctx) + "/" + encodeURIComponent(ctx.upstreamTaskId) + "/content", method: ctx.clientRequest.method, headers: headers };
}

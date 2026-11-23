const NANO_MODELS = [
  "nano-banana", "nano-banana-fast", "nano-banana-2", "nano-banana-2-cl",
  "nano-banana-2-2k-cl", "nano-banana-2-4k-cl", "nano-banana-2.1", "nano-banana-pro",
  "nano-banana-pro-vt", "nano-banana-pro-cl", "nano-banana-pro-vip", "nano-banana-pro-4k-vip",
];
const IMAGE_MODELS = NANO_MODELS.concat([
  "gpt-image-2", "gpt-image-2-vip", "gpt-image-2.5", "gpt-image-2.5-flare",
  "gpt-image-2.5-sunburst", "gpt-image-2.6", "gpt-image-2.6-flare", "gpt-image-2.6-sunburst",
]);
const H3_MODEL = "MiniMax-H3";
const VIDEO_SPECS = {
  [H3_MODEL]: { min: 1, max: 15, resolutions: ["480p", "768p", "1080p"], images: 9, audios: 3, videos: 0 },
  "agnes-video-2.5": { min: 4, max: 12, resolutions: ["720p", "1080p", "2k"], images: 5, audios: 3 },
  "wan-3.0": { min: 2, max: 30, resolutions: ["480p", "720p", "1080p"], images: 10, audios: 5, videos: 5 },
};
const VIDEO_MODELS = Object.keys(VIDEO_SPECS);
const TASK_MODELS = IMAGE_MODELS.concat(VIDEO_MODELS);
const SUBMIT_PATH = "/v1/api/generate";
const RESULT_PATH = "/v1/api/result";
const PREFIX = "/grsai";
const MAX_IMAGE_N = 128; // 与宿主 dto.MaxImageN 一致

export const meta = {
  apiVersion: 1,
  key: "grsai",
  name: "GrsAI",
  icon: "text:Grs",
  version: "1.0.0",
  author: { name: "new-api" },
  website: "https://grsai.ai",
  baseUrl: "https://grsaiapi.com",
  description: { en: "GrsAI image, video, and language models", zh: "GrsAI 图像、视频及语言模型", "zh-TW": "GrsAI 圖像、影片及語言模型" },
  models: TASK_MODELS.concat(["gemini-3.1-pro", "gemini-3.8-flash", "gpt-6.2-sol"]),
  fetchMode: "per_task",
  auth: { type: "api_key" },
  upstreams: ["vendor", "new_api"],
  submitResponseTypes: ["json", "sse"],
  requiredCapabilities: ["submit-sse-public-id@1"],
  routes: [
    { method: "POST", path: PREFIX + SUBMIT_PATH, models: TASK_MODELS, type: "submit", decode: "decodeGenerate", render: "renderTask" },
    { method: "GET", path: PREFIX + RESULT_PATH, taskIdParam: "id", type: "query", render: "renderTask" },
  ],
  protocols: [
    { name: "openai_image", models: IMAGE_MODELS },
    { name: "openai_video", models: VIDEO_MODELS },
  ],
  relay: [
    { incoming_path: "/v1/chat/completions", upstream_path: "/v1/chat/completions", models: ["re:^(gemini-|gpt-[0-9])"] },
    { incoming_path: "/v1/responses", upstream_path: "/v1/responses", models: ["re:^gpt-[0-9]"] },
    { incoming_path: "/v1beta/models/{model}:generateContent", upstream_path: "/v1beta/models/{model}:generateContent", models: ["re:^gemini-"] },
  ],
  usageSchema: {
    image_count: {
      type: "number", unit: "count", unitLabel: { en: "image", zh: "张", "zh-TW": "張" },
      description: { en: "Image generation unit price", zh: "图片生成单价", "zh-TW": "圖片生成單價" },
    },
  },
  usageProfiles: [{
    models: VIDEO_MODELS,
    schema: {
      seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价", "zh-TW": "影片生成單價" } },
      resolution: { enum: ["480p", "720p", "768p", "1080p", "2k"], description: { en: "Output video resolution", zh: "输出视频分辨率", "zh-TW": "輸出影片解析度" } },
    },
  }],
};

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function isObject(value) {
  return value && typeof value === "object" && !Array.isArray(value);
}

function videoSpec(model) {
  const key = model === "minimax-h3" ? H3_MODEL : model;
  return VIDEO_MODELS.includes(key) ? VIDEO_SPECS[key] : undefined;
}

function limitOutputCount(request) {
  for (const key of ["n", "task_count"]) {
    if (request[key] !== undefined && request[key] !== 1) throw new Error(key + " must be 1; GrsAI batches are not supported");
  }
}

function videoFacts(body, model) {
  const spec = videoSpec(model);
  const seconds = body.duration;
  const resolution = text(body.resolution).toLowerCase();
  if (!spec) throw new Error("unsupported GrsAI video model");
  if (!spec.resolutions.includes(resolution)) throw new Error("resolution must be one of " + spec.resolutions.join(", "));
  const max = spec === VIDEO_SPECS[H3_MODEL] && resolution === "1080p" ? 10 : spec.max;
  if (!Number.isInteger(seconds) || seconds < spec.min || seconds > max) throw new Error("duration must be an integer between " + spec.min + " and " + max);
  return { seconds, resolution };
}

function validateVideo(body, model) {
  const spec = videoSpec(model);
  videoFacts(body, model);
  const ratios = spec === VIDEO_SPECS[H3_MODEL] ? ["portrait", "landscape"] : ["16:9", "9:16", "21:9", "4:3", "1:1", "3:4"];
  if (!ratios.includes(body.aspectRatio)) throw new Error("aspectRatio must be one of " + ratios.join(", "));
  for (const key of ["images", "audios", "videos"]) {
    if (body[key] === undefined) continue;
    if (!Array.isArray(body[key]) || (spec[key] !== undefined && body[key].length > spec[key])) {
      throw new Error(key + " must be an array" + (spec[key] === undefined ? "" : " with at most " + spec[key] + " items"));
    }
    if (key === "videos" && body.videos.some((entry) => !isObject(entry) || !text(entry.url))) throw new Error("videos must contain entries with a url");
  }
  const first = body.firstFrame !== undefined;
  const last = body.lastFrame !== undefined;
  if (first !== last) throw new Error("firstFrame and lastFrame must be provided together");
  if (first && (body.images?.length || body.audios?.length)) throw new Error("firstFrame and lastFrame cannot be combined with images or audios");
}

function jsonBody(ctx) {
  if (ctx.body?.kind !== "json" || !isObject(ctx.body.value)) throw new Error("JSON object required");
  return { ...ctx.body.value };
}

// multipart 扩展字段可用 JSON，上传内容由宿主在发请求前替换。
function formBody(ctx, protocol) {
  if (ctx.body?.kind !== "multipart") return jsonBody(ctx);
  const entries = [];
  for (const [key, values] of Object.entries(ctx.body.fields || {})) {
    if (key === "image" || key === "image[]" || key === "images") {
      entries.push([key, values.flatMap((value) => value.trim().startsWith("[") ? JSON.parse(value) : [value])]);
      continue;
    }
    if (values.length !== 1) throw new Error(key + " must be provided once");
    const value = values[0];
    const typed = /^(?:true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)$|^[\[{]/.test(value.trim());
    const stringField = ["model", "prompt", "size", "quality", "background", "output_format", "response_format", "input_reference", "aspectRatio", "resolution", "imageSize", "firstFrame", "lastFrame"].includes(key);
    entries.push([key, !stringField && typed ? JSON.parse(value) : value]);
  }
  const request = Object.fromEntries(entries);
  const uploads = [];
  for (const file of ctx.body.files || []) {
    const supported = protocol === "image" ? ["image", "image[]", "images"].includes(file.field) : file.field === "input_reference";
    if (!supported) throw new Error("unsupported upload field " + file.field);
    uploads.push({ __fileRef: file.ref, encoding: "dataUrl" });
  }
  if (uploads.length && protocol === "video") {
    if (uploads.length !== 1 || request.input_reference !== undefined) throw new Error("provide a single input_reference file or URL");
    request.input_reference = uploads[0];
  } else if (uploads.length) {
    request["image[]"] = (request["image[]"] || []).concat(uploads);
  }
  return request;
}

function imageReference(value) {
  if (text(value)) return value;
  if (isObject(value)) {
    if (text(value.image_url)) return value.image_url;
    if (text(value.__fileRef)) return value;
  }
  throw new Error("image reference must be a URL, Base64 image, or an uploaded file");
}

function applyImageSize(body, size, model) {
  if (size === undefined) return;
  if (!text(size)) throw new Error("size must be a string");
  if (NANO_MODELS.includes(model)) {
    const ratios = { auto: "auto", "1024x1024": "1:1", "1536x1024": "3:2", "1024x1536": "2:3" };
    if (!Object.prototype.hasOwnProperty.call(ratios, size)) throw new Error("use GrsAI aspectRatio and imageSize for this Nano Banana size");
    if (body.aspectRatio === undefined) body.aspectRatio = ratios[size];
    if (size !== "auto" && body.imageSize === undefined) body.imageSize = "1K";
  } else if (body.aspectRatio === undefined) {
    body.aspectRatio = size;
  }
}

function applyVideoSize(body, size, model) {
  if (size === undefined) return;
  const sizes = { "1280x720": ["16:9", "720p"], "720x1280": ["9:16", "720p"], "1920x1080": ["16:9", "1080p"], "1080x1920": ["9:16", "1080p"] };
  if (!Object.prototype.hasOwnProperty.call(sizes, size)) throw new Error("unsupported video size; use GrsAI aspectRatio and resolution");
  const [ratio, resolution] = sizes[size];
  if (body.resolution === undefined) body.resolution = resolution;
  if (body.aspectRatio === undefined) body.aspectRatio = videoSpec(model) === VIDEO_SPECS[H3_MODEL] ? (ratio === "16:9" ? "landscape" : "portrait") : ratio;
}

function decode(ctx, protocol) {
  const body = protocol ? formBody(ctx, protocol) : jsonBody(ctx);
  const model = text(ctx.model) || text(body.model);
  const upstreamModel = text(ctx.upstreamModel) || model;
  if (!model || !text(body.prompt)) throw new Error("model and prompt are required");
  limitOutputCount(body);
  const size = protocol ? body.size : undefined;
  if (protocol) delete body.size;
  if (protocol) {
    if (body.stream !== undefined && body.stream !== false) throw new Error("use the GrsAI native route for streaming tasks");
    if (body.mask !== undefined) throw new Error("GrsAI does not support image masks");
    delete body.stream;
    delete body.n;
    delete body.task_count;
    body.replyType = "async";
  } else if (body.replyType !== undefined && !["json", "async", "stream"].includes(body.replyType)) {
    throw new Error("replyType must be json, stream, or async");
  }
  if (protocol === "image") {
    const references = [];
    for (const key of ["images", "image", "image[]", "image_url"]) {
      const value = body[key];
      if (value !== undefined) references.push(...(Array.isArray(value) ? value : [value]).map(imageReference));
      delete body[key];
    }
    if (ctx.operation === "edit" && !references.length) throw new Error("image editing requires a reference image");
    if (references.length) body.images = references;
    if (size !== undefined && !text(size)) throw new Error("size must be a string");
    if (body.outputFormat === undefined && body.output_format !== undefined) body.outputFormat = body.output_format;
    if (body.response_format !== undefined && !["url", "b64_json"].includes(body.response_format)) throw new Error("response_format must be url or b64_json");
    delete body.output_format;
    delete body.response_format;
  }
  const video = protocol === "video" || (!protocol && Boolean(videoSpec(upstreamModel) || videoSpec(model)));
  if (video) {
    if (protocol) {
      if (body.seconds !== undefined) {
        const seconds = typeof body.seconds === "string" && /^\d+$/.test(body.seconds) ? Number(body.seconds) : body.seconds;
        if (body.duration !== undefined && body.duration !== seconds) throw new Error("duration and seconds must agree");
        body.duration = seconds;
        delete body.seconds;
      }
      if (body.input_reference !== undefined) {
        if (body.images !== undefined && !Array.isArray(body.images)) throw new Error("images must be an array");
        body.images = (body.images || []).concat([imageReference(body.input_reference)]);
        delete body.input_reference;
      }
    }
    // 候选解码时可能尚未解析渠道模型映射，最终模型在提交及预扣阶段复核。
    const knownModel = videoSpec(upstreamModel) ? upstreamModel : model;
    if (videoSpec(knownModel)) {
      const candidate = { ...body };
      applyVideoSize(candidate, size, knownModel);
      validateVideo(candidate, knownModel);
    }
  }
  body.model = model;
  const action = video ? "video" : "image";
  return { kind: "submit", model, action, requestBody: size === undefined ? { body } : { body, size } };
}

function upstreamURL(ctx, path) {
  return ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream?.kind === "new_api" ? PREFIX : "") + path;
}

export function buildSubmitRequest(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  const { body: payload, size } = ctx.requestBody;
  const body = { ...payload, model: model === H3_MODEL && ctx.upstream?.kind !== "new_api" ? "minimax-h3" : model };
  limitOutputCount(body);
  if (videoSpec(model) || ctx.action === "video") {
    applyVideoSize(body, size, model);
    validateVideo(body, model);
  } else {
    applyImageSize(body, size, model);
  }
  if (body.replyType === undefined) body.replyType = "json";
  return {
    url: upstreamURL(ctx, SUBMIT_PATH), method: "POST",
    headers: { Authorization: ctx.authHeader || "Bearer " + ctx.apiKey, "Content-Type": "application/json" },
    body,
    responseType: body.replyType === "stream" ? "sse" : "json",
  };
}

export function buildQueryRequest(ctx) {
  return {
    url: upstreamURL(ctx, RESULT_PATH) + "?id=" + encodeURIComponent(ctx.taskId), method: "GET",
    headers: { Authorization: ctx.authHeader || "Bearer " + ctx.apiKey },
  };
}

function resultURLs(body) {
  const results = Array.isArray(body?.results) ? body.results : [];
  return results.map((item) => text(item?.url)).filter((url) => /^https?:\/\/[^\s]+$/i.test(url));
}

export function parseTaskResult(ctx, body) {
  const urls = resultURLs(body);
  let status = "UNKNOWN";
  let reason = "";
  switch (body?.status) {
    case "running": status = "IN_PROGRESS"; break;
    case "succeeded":
      status = urls.length ? "SUCCESS" : "FAILURE";
      if (!urls.length) reason = "GrsAI succeeded without a result URL";
      break;
    case "violation":
    case "failed":
      status = "FAILURE";
      reason = text(body.error) || (body.status === "violation" ? "GrsAI rejected the content" : "GrsAI generation failed");
      break;
  }
  const progress = status === "SUCCESS" ? 100 : Number.isFinite(body?.progress) ? Math.min(100, Math.max(0, body.progress)) : 0;
  return { status, reason, progress: progress + "%", url: urls[0] || "" };
}

export function parseSubmitResponse(ctx, response) {
  const data = response.body;
  const result = parseTaskResult(ctx, data);
  if (result.status === "UNKNOWN") throw new Error(text(data?.error) || "GrsAI returned an unknown submission status");
  const terminal = result.status === "SUCCESS" || result.status === "FAILURE";
  const taskId = text(data.id) || (terminal ? ctx.publicTaskId : "");
  if (!taskId) throw new Error("GrsAI response is missing a task ID");
  const model = ctx.upstreamModel || ctx.model;
  const spec = videoSpec(model);
  const submission = { taskId, taskData: data, immediate: terminal ? result : undefined };
  if (spec && ctx.requestBody) {
    const request = { ...ctx.requestBody.body };
    applyVideoSize(request, ctx.requestBody.size, model);
    submission.state = { maxDuration: spec === VIDEO_SPECS[H3_MODEL] && text(request.resolution).toLowerCase() === "1080p" ? 10 : spec.max };
  }
  return submission;
}

export function parseSubmitEvent(ctx, event, previous) {
  const payload = event.data.trim();
  if (!payload) return { state: previous, done: false };
  if (payload === "[DONE]") {
    return { state: previous, done: previous?.status !== "running" && Boolean(previous) };
  }
  const update = JSON.parse(payload);
  if (!isObject(update)) throw new Error("GrsAI stream data must be a JSON object");
  const state = { ...previous, ...update };
  return { state, done: ["succeeded", "failed", "violation"].includes(state.status) };
}

function imageUsage(body) {
  const count = resultURLs(body).length;
  return body?.status === "succeeded" && count >= 1 && count <= MAX_IMAGE_N ? { image_count: count } : {};
}

export function extractUsage(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  const { body: payload, size } = ctx.requestBody;
  limitOutputCount(payload);
  if (!videoSpec(model) && ctx.action !== "video") return { image_count: 1 };
  const body = { ...payload };
  applyVideoSize(body, size, model);
  const facts = videoFacts(body, model);
  return ctx.usagePurpose === "billing_ratios" ? { seconds: facts.seconds } : facts;
}

export function extractUsageOnSubmit(ctx, body) {
  return videoSpec(ctx.upstreamModel || ctx.model) || ctx.action === "video" ? {} : imageUsage(body);
}

export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS") return {};
  const spec = videoSpec(ctx.upstreamModel || ctx.model);
  if (!spec) return ctx.action === "video" ? {} : imageUsage(body);
  // 查询响应可能没有 usage，此时沿用冻结的请求时长。
  const seconds = body?.usage?.duration;
  const max = ctx.state?.maxDuration ?? spec.max;
  return Number.isFinite(seconds) && seconds > 0 && seconds <= max ? { seconds } : {};
}

export const native = {
  decodeGenerate(ctx) {
    return decode(ctx, "");
  },
  renderTask(ctx, task) {
    const snapshot = task.data || {};
    const data = { ...snapshot, id: task.task_id };
    if (task.status === "SUCCESS") {
      data.status = "succeeded";
    } else if (task.status === "FAILURE") {
      data.status = snapshot.status === "violation" ? "violation" : "failed";
      data.error = text(snapshot.error) || task.fail_reason || "GrsAI generation failed";
      delete data.results;
    } else {
      data.status = "running";
    }
    if (snapshot.progress === undefined && ctx.method === "GET") {
      data.progress = Number(String(task.progress || "0").replace(/%$/, ""));
    }
    return data;
  },
  error(ctx, error) {
    return { status: "failed", error: error.message };
  },
};

export const protocols = {
  openai_image: {
    decodeRequest(ctx) {
      return decode(ctx, "image");
    },
    render(ctx, task) {
      const data = resultURLs(task.data).map((url) => ({ url }));
      if (!data.length) throw new Error("GrsAI returned no images");
      return { created: task.created_at, data };
    },
  },
  openai_video: {
    decodeRequest(ctx) {
      return decode(ctx, "video");
    },
    render(ctx, task) {
      const url = resultURLs(task.data)[0];
      return url ? { url } : {};
    },
  },
};

export function listArtifacts(task) {
  if (task.action !== "video" || task.status !== "SUCCESS" || !resultURLs(task.data).length) return [];
  return [{ key: "video", type: "video", mimeType: "video/mp4" }];
}

export function buildContentRequest(ctx) {
  const url = resultURLs(ctx.data)[0];
  if (ctx.artifactKey !== "video" || !url) throw new Error("artifact_not_found");
  return { url, method: ctx.clientRequest.method, credentialless: true };
}

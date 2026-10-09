const MAX_IMAGE_N = 128; // 与宿主 dto.MaxImageN 一致

export const meta = {
  apiVersion: 1,
  key: "jingyu",
  name: "鲸鱼 AI",
  icon: "text:鲸鱼",
  version: "1.0.0",
  author: { name: "new-api" },
  website: "https://jingyuapi.art",
  baseUrl: "https://jingyuapi.art",
  description: { en: "Image generation and editing via Jingyu AI", zh: "通过鲸鱼 AI 生成和编辑图片" },
  requiredCapabilities: ["json-clone@1"],
  models: ["nano-banana-pro", "nano-banana-2", "gpt-image-2", "gpt-image-2-1k", "gpt-image-2-2k", "gpt-image-2-4k", "gpt-image-2.5-sunburst"],
  fetchMode: "per_task",
  auth: { type: "api_key" },
  protocols: ["openai_image"],
  usageSchema: {
    image_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张" },
      description: { en: "Image generation unit price", zh: "图片生成单价" },
    },
  },
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

function normalizedRequest(ctx) {
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
  const model = text(ctx.model);
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
  const body = utils.json.clone(ctx.requestBody);
  validateSingleTask(body);
  body.model = ctx.upstreamModel || ctx.model;
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/image/generations",
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
  if (taskId) return { taskId: taskId, taskData: body };
  // 兼容上游立即完成的图片响应，不伪造可轮询的上游任务 ID。
  const images = imageEntries(body);
  if (!images.length) throw new Error("Jingyu response has no task_id or image output");
  return { taskId: ctx.publicTaskId, taskData: body, immediate: { status: "SUCCESS", progress: "100%", url: images[0].url || "" } };
}

export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl.replace(/\/+$/, "") + "/v1/image/generations/" + encodeURIComponent(ctx.taskId),
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
  validateSingleTask(ctx.requestBody);
  return { image_count: 1 };
}

export function extractUsageOnSubmit(ctx, body) {
  return imageUsage(body);
}

export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS") return {};
  return imageUsage(body);
}

export const protocols = {
  openai_image: {
    decodeRequest: normalizedRequest,
    render: function (ctx, task) {
      const data = imageEntries(task.data);
      if (!data.length) throw new Error("Jingyu returned no image output");
      return { created: task.created_at, data: data };
    },
  },
};

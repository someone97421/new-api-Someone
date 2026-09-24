// Gemini native image generation (Nano Banana) served as the OpenAI Images API.
//
// Upstream contract (Google Generative Language REST, or a Gemini-compatible
// endpoint that implements it):
//
//   POST {base}/{v1beta}/models/{model}:generateContent
//   {
//     "contents": [{"role": "user", "parts": [
//       {"text": "..."},
//       {"inlineData": {"mimeType": "image/png", "data": "<base64>"}}
//     ]}],
//     "generationConfig": {
//       "responseModalities": ["TEXT", "IMAGE"],
//       "imageConfig": {"aspectRatio": "16:9", "imageSize": "2K"}
//     }
//   }
//   -> {
//     "candidates": [{"finishReason": "STOP", "content": {"role": "model", "parts": [
//       {"text": "..."}, {"inlineData": {"mimeType": "image/png", "data": "<base64>"}}
//     ]}}],
//     "promptFeedback": {"blockReason": "SAFETY"},
//     "usageMetadata": {...}
//   }
//
// One request generates one image. The vendor answers synchronously, so the
// submit response is the deliverable: the plugin never leaves a pollable task,
// and the host's OpenAI Images bridge returns the rendered `data[]` inline.
// Images are answered as `b64_json` because the vendor returns inline Base64
// rather than a hosted URL; a `fileData.fileUri` answer is returned as `url`.
//
// Client contract (`POST /v1/images/generations`, `POST /v1/images/edits`):
//   model            required, declared model or public Nano Banana alias
//   prompt           required, non-empty text
//   n                optional, must be 1 (one image per request)
//   size             optional, "WIDTHxHEIGHT" or "ASPECT:RATIO"
//   aspect_ratio     optional, Gemini aspect ratio such as "16:9"
//   image_size       optional, one of 512, 1K, 2K, 4K
//   resolution       optional, alias of image_size
//   quality          optional, "hd"/"high"/"2k" -> 2K, anything else -> 1K
//   image, images, image_url, image_urls
//                    optional reference images: Base64, data URL, or HTTP(S) URL
//   seed             optional integer
//   response_format  optional, "url" or "b64_json"
//   image_config_mode optional, "official" (default) or "response_format"
//   metadata.generation_config
//                    optional extra generationConfig fields, merged before the
//                    host-owned keys below
//   multipart files  optional reference images on /v1/images/edits
//
// Limits: at most 16 reference images; size/aspect ratio and image size are
// validated here so a rejected request never reserves quota. HTTP(S) reference
// images are forwarded as `fileData.fileUri` (the vendor must be able to fetch
// them); inline a Base64 value or upload the file for endpoints that cannot.

export const meta = {
  apiVersion: 1,
  key: "gemini-image",
  name: "Nano Banana (Gemini Image)",
  icon: "Gemini.Color",
  description: {
    en: "Gemini native image generation (Nano Banana) served as the OpenAI Images API",
    zh: "Gemini 原生生图（Nano Banana）以 OpenAI Images 接口提供",
  },
  version: "1.0.0",
  author: { name: "QuantumNous" },
  baseUrl: "https://generativelanguage.googleapis.com",
  models: [
    "gemini-3.1-flash-image",
    "gemini-3.1-flash-lite-image",
    "gemini-3-pro-image",
    "gemini-3-pro-image-preview",
    "gemini-2.5-flash-image",
    "gemini-2.0-flash-exp-image-generation",
    "gemini-2.0-flash-exp",
    "nano-banana",
    "nano-banana-2",
    "nano-banana-2-lite",
    "nano-banana-pro",
    "nano-banana-pro-preview",
  ],
  fetchMode: "per_task",
  usageSchema: {
    image_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张", "zh-TW": "張", fr: "image", ja: "枚", ru: "изображение", vi: "ảnh" },
      description: { en: "Image generation unit price", zh: "图片生成单价" },
    },
    image_size: {
      enum: ["512", "1K", "2K", "4K"],
      enumLabels: {
        "512": { en: "512px output", zh: "512 输出" },
        "1K": { en: "1K output", zh: "1K 输出" },
        "2K": { en: "2K output", zh: "2K 输出" },
        "4K": { en: "4K output", zh: "4K 输出" },
      },
      description: { en: "Output image size", zh: "输出图片尺寸" },
    },
  },
  usageExamples: [
    { label: "1K · 1 image", facts: { image_count: 1, image_size: "1K" } },
    { label: "2K · 1 image", facts: { image_count: 1, image_size: "2K" } },
    { label: "4K · 1 image", facts: { image_count: 1, image_size: "4K" } },
  ],
  protocols: [{ name: "openai_image" }],
};

// Public Nano Banana names mapped onto the upstream model id. Channel model
// mapping still wins: a mapped upstream name is passed through unchanged.
const MODEL_ALIASES = {
  "nano-banana": "gemini-2.5-flash-image",
  "nano-banana-2": "gemini-3.1-flash-image",
  "nano-banana-2-lite": "gemini-3.1-flash-lite-image",
  "nano-banana-pro": "gemini-3-pro-image",
};

const MAX_REFERENCE_IMAGES = 16;
const IMAGE_SIZES = ["512", "1K", "2K", "4K"];
// Size presets the Gemini image models were configured with; other sizes keep
// the model default aspect ratio of 1:1.
const SIZE_ASPECT_RATIOS = {
  "1536x1024": "3:2",
  "1024x1536": "2:3",
  "1024x1792": "9:16",
  "1792x1024": "16:9",
};
const MAX_SEED = 2147483647;

function trimmed(value) {
  return String(value || "").trim();
}

function objectValue(value, name) {
  if (value === undefined || value === null) return {};
  if (typeof value !== "object" || Array.isArray(value)) throw new Error(name + " must be an object");
  return value;
}

function isFileRef(value) {
  return !!value && typeof value === "object" && !Array.isArray(value) && typeof value.__fileRef === "string";
}

function isDataUrl(value) {
  return /^data:/i.test(value);
}

function isHttpUrl(value) {
  return /^https?:\/\//i.test(value);
}

function isBase64(value) {
  return /^[A-Za-z0-9+/]+={0,2}$/.test(value);
}

// Browser-free plugins cannot call net/http DetectContentType, so raw Base64
// input is sniffed from the common media signatures.
function base64Mime(value) {
  if (value.startsWith("iVBORw0KGgo")) return "image/png";
  if (value.startsWith("/9j/")) return "image/jpeg";
  if (value.startsWith("R0lGOD")) return "image/gif";
  if (value.startsWith("UklGR")) return "image/webp";
  return "application/octet-stream";
}

function dataUrlPart(value) {
  const comma = value.indexOf(",");
  if (comma < 0) throw new Error("image data URL is malformed");
  const mediaType = trimmed(value.slice(5, comma).split(";")[0]);
  const data = value.slice(comma + 1);
  if (!trimmed(data)) throw new Error("image data URL carries no payload");
  return { inlineData: { mimeType: mediaType || "application/octet-stream", data: data } };
}

function uploadMime(ref, files) {
  for (const file of files || []) {
    if (file.ref === ref) return trimmed(file.mimeType) || "application/octet-stream";
  }
  return "application/octet-stream";
}

function referencePart(value, files) {
  if (isFileRef(value)) return { inlineData: { mimeType: uploadMime(value.__fileRef, files), data: value } };
  const text = trimmed(value);
  if (!text) return null;
  if (isDataUrl(text)) return dataUrlPart(text);
  if (isHttpUrl(text)) return { fileData: { fileUri: text, mimeType: "image/*" } };
  return { inlineData: { mimeType: base64Mime(text), data: text } };
}

function collectReferences(req, uploads) {
  const images = [];
  const seen = {};
  const push = function (value) {
    if (isFileRef(value)) {
      images.push(value);
      return;
    }
    const text = trimmed(value);
    if (!text || seen[text]) return;
    if (!isDataUrl(text) && !isHttpUrl(text) && !isBase64(text))
      throw new Error("image must be a Base64 value, a data URL, or an HTTP URL");
    seen[text] = true;
    images.push(text);
  };
  const listed = [].concat(
    req.image === undefined ? [] : req.image,
    req.images === undefined ? [] : req.images,
    req.image_url === undefined ? [] : req.image_url,
    req.image_urls === undefined ? [] : req.image_urls,
  );
  for (const value of listed) {
    if (Array.isArray(value)) for (const item of value) push(item);
    else push(value);
  }
  for (const upload of uploads) push(upload);
  if (images.length > MAX_REFERENCE_IMAGES)
    throw new Error("Gemini image generation accepts at most " + MAX_REFERENCE_IMAGES + " reference images");
  return images;
}

function normalizeImageSize(value, name) {
  const text = trimmed(value).toUpperCase();
  if (!IMAGE_SIZES.includes(text)) throw new Error(name + " must be one of 512, 1K, 2K, or 4K");
  return text;
}

function resolveImageSize(req) {
  if (req.image_size !== undefined && trimmed(req.image_size)) return normalizeImageSize(req.image_size, "image_size");
  if (req.resolution !== undefined && trimmed(req.resolution)) return normalizeImageSize(req.resolution, "resolution");
  if (req.quality !== undefined && trimmed(req.quality)) {
    const quality = trimmed(req.quality).toLowerCase();
    return ["hd", "high", "2k"].includes(quality) ? "2K" : "1K";
  }
  return "";
}

function resolveAspectRatio(req) {
  const explicit = trimmed(req.aspect_ratio);
  if (explicit) {
    if (!/^\d+:\d+$/.test(explicit)) throw new Error("aspect_ratio must look like 16:9");
    return explicit;
  }
  const size = trimmed(req.size);
  if (!size) return "";
  if (size.includes(":")) {
    if (!/^\d+:\d+$/.test(size)) throw new Error("size must be WIDTHxHEIGHT or an aspect ratio such as 16:9");
    return size;
  }
  const normalized = size.toLowerCase().replace(/\s/g, "").replace("×", "x");
  const preset = SIZE_ASPECT_RATIOS[normalized];
  if (preset) return preset;
  const pixels = /^(\d+)x(\d+)$/.exec(normalized);
  if (!pixels) throw new Error("size must be WIDTHxHEIGHT or an aspect ratio such as 16:9");
  return "1:1";
}

function resolveSeed(req) {
  if (req.seed === undefined || req.seed === null || req.seed === "") return undefined;
  const seed = Number(req.seed);
  if (!Number.isInteger(seed) || seed < 0 || seed > MAX_SEED) throw new Error("seed must be an integer between 0 and " + MAX_SEED);
  return seed;
}

function resolveConfigMode(req) {
  const mode = trimmed(req.image_config_mode).toLowerCase();
  if (!mode) return "official";
  if (mode === "response_format") return "response_format";
  if (mode === "official" || mode === "legacy" || mode === "imageconfig" || mode === "image_config") return "official";
  throw new Error("image_config_mode must be official or response_format");
}

function modelKey(ctx) {
  return trimmed(ctx.upstreamModel || ctx.model || (ctx.requestBody || {}).model);
}

function upstreamModel(ctx) {
  const key = modelKey(ctx);
  return MODEL_ALIASES[key] || key;
}

function versionedRoot(baseUrl) {
  const base = trimmed(baseUrl).replace(/\/+$/, "");
  if (/\/(v1|v1beta|v1alpha)$/i.test(base)) return base;
  return base + "/v1beta";
}

function geminiHeaders(ctx) {
  const headers = { "Content-Type": "application/json", Accept: "application/json" };
  const key = trimmed(ctx.apiKey);
  if (key) {
    headers["x-goog-api-key"] = key;
    headers.Authorization = "Bearer " + key;
  }
  return headers;
}

function candidateParts(body) {
  const parts = [];
  for (const candidate of Array.isArray(body.candidates) ? body.candidates : []) {
    const content = candidate && candidate.content;
    const listed = content && Array.isArray(content.parts) ? content.parts : [];
    for (const part of listed) parts.push(part);
  }
  return parts;
}

function inlineImage(part) {
  if (!part || typeof part !== "object" || Array.isArray(part)) return null;
  const value = part.inlineData || part.inline_data;
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const data = trimmed(value.data);
  const mimeType = trimmed(value.mimeType || value.mime_type);
  if (!data || (mimeType && !/^image\//i.test(mimeType))) return null;
  return { data: data, mimeType: mimeType };
}

function remoteImage(part) {
  if (!part || typeof part !== "object" || Array.isArray(part)) return null;
  const value = part.fileData || part.file_data;
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const uri = trimmed(value.fileUri || value.file_uri);
  return uri ? { url: uri } : null;
}

function responseImages(body) {
  const images = [];
  for (const part of candidateParts(body)) {
    const inline = inlineImage(part);
    if (inline) {
      images.push({ data: inline.data });
      continue;
    }
    const remote = remoteImage(part);
    if (remote) images.push(remote);
  }
  return images;
}

function responseTexts(body) {
  const texts = [];
  for (const part of candidateParts(body)) {
    const text = part && typeof part.text === "string" ? trimmed(part.text) : "";
    if (text) texts.push(text);
  }
  return texts;
}

// A 2xx response without an image is a content or safety answer, not a
// transport failure: report why instead of claiming the task succeeded.
function failureReason(body) {
  const feedback = body.promptFeedback || body.prompt_feedback || {};
  const block = trimmed(feedback.blockReason || feedback.block_reason);
  if (block) return "no images generated: " + block;
  for (const candidate of Array.isArray(body.candidates) ? body.candidates : []) {
    const finish = trimmed(candidate && (candidate.finishReason || candidate.finish_reason));
    if (finish && finish !== "STOP") return "no images generated: " + finish;
  }
  return "no images generated";
}

function normalizedRequestBody(ctx) {
  const model = trimmed(ctx.model);
  if (!model) throw new Error("model is required");
  let req = {};
  const uploads = [];
  if (ctx.body && ctx.body.kind === "json") {
    req = ctx.body.value;
    if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  } else if (ctx.body && ctx.body.kind === "multipart") {
    const fields = ctx.body.fields || {};
    for (const name of Object.keys(fields)) {
      if (fields[name].length > 1) throw new Error(name + " must be provided once");
      req[name] = fields[name][0];
    }
    if (req.n !== undefined) {
      const n = Number(req.n);
      if (!Number.isInteger(n)) throw new Error("n must be an integer");
      req.n = n;
    }
    for (const file of ctx.body.files || []) {
      if (file.field === "mask") throw new Error("mask is not supported by this model");
      if (!/^(images?)(\[\d*\])?$/.test(file.field)) throw new Error("unexpected file field: " + file.field);
      uploads.push({ __fileRef: file.ref, encoding: "base64" });
    }
  } else {
    throw new Error("JSON or multipart body required");
  }
  const prompt = typeof req.prompt === "string" ? req.prompt : "";
  if (!trimmed(prompt)) throw new Error("field prompt is required");
  const requested = req.n === undefined || req.n === null ? 1 : req.n;
  if (!Number.isInteger(requested) || requested < 1) throw new Error("n must be a positive integer");
  if (requested !== 1) throw new Error("Gemini image generation supports exactly one image per request");
  if (req.response_format !== undefined && req.response_format !== "url" && req.response_format !== "b64_json")
    throw new Error("response_format must be url or b64_json");
  const images = collectReferences(req, uploads);
  const requestBody = {
    model: model,
    prompt: trimmed(prompt),
    n: 1,
    image_size: resolveImageSize(req),
    image_config_mode: resolveConfigMode(req),
  };
  const aspectRatio = resolveAspectRatio(req);
  if (aspectRatio) requestBody.aspect_ratio = aspectRatio;
  const seed = resolveSeed(req);
  if (seed !== undefined) requestBody.seed = seed;
  if (req.response_format !== undefined) requestBody.response_format = req.response_format;
  if (images.length) requestBody.images = images;
  const metadata = objectValue(req.metadata, "metadata");
  if (metadata.generation_config !== undefined) {
    objectValue(metadata.generation_config, "metadata.generation_config");
    requestBody.generation_config = metadata.generation_config;
  }
  return { requestBody: requestBody, hasReferences: images.length > 0 };
}

export function buildSubmitRequest(ctx) {
  const req = ctx.requestBody || {};
  const parts = [{ text: String(req.prompt || "") }];
  for (const image of req.images || []) {
    const part = referencePart(image, ctx.files);
    if (part) parts.push(part);
  }
  const generationConfig = Object.assign({}, req.generation_config || {});
  generationConfig.responseModalities = ["TEXT", "IMAGE"];
  const imageConfig = {};
  if (req.aspect_ratio) imageConfig.aspectRatio = req.aspect_ratio;
  if (req.image_size) imageConfig.imageSize = req.image_size;
  if (req.image_config_mode === "response_format") generationConfig.responseFormat = { image: imageConfig };
  else generationConfig.imageConfig = imageConfig;
  if (req.seed !== undefined) generationConfig.seed = req.seed;
  return {
    url: versionedRoot(ctx.baseUrl) + "/models/" + encodeURIComponent(upstreamModel(ctx)) + ":generateContent",
    method: "POST",
    headers: geminiHeaders(ctx),
    body: { contents: [{ role: "user", parts: parts }], generationConfig: generationConfig },
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = (resp && resp.body) || {};
  // No vendor task exists for a synchronous Google call. The host already
  // allocated a public id; keep that identity instead of inventing a pollable one.
  const taskId = trimmed(ctx.publicTaskId);
  if (!taskId) throw new Error("missing public task id");
  const images = responseImages(body);
  if (!images.length) return { taskId: taskId, taskData: body, immediate: { status: "FAILURE", reason: failureReason(body) } };
  return {
    taskId: taskId,
    taskData: body,
    immediate: { status: "SUCCESS", progress: "100%", url: images[0].url || "" },
  };
}

export function extractUsage(ctx) {
  const req = ctx.requestBody || {};
  const count = Number.isInteger(req.n) && req.n > 0 ? req.n : 1;
  if (ctx.usagePurpose === "billing_ratios") return { image_count: count };
  return { image_count: count, image_size: req.image_size || "1K" };
}

// Legacy per-call pricing multiplies the model price by the actual image count.
export function extractUsageOnSubmit(ctx, body) {
  const images = responseImages(body || {});
  return images.length ? { image_count: images.length } : {};
}

export function extractUsageOnComplete(task, result, body) {
  // The completion snapshot replaces the reserved count; an image-less success
  // keeps the reservation rather than guessing a smaller charge.
  const images = responseImages(body || {});
  return images.length ? { image_count: images.length } : {};
}

// Gemini answers image generation in the submit response, so this plugin never
// leaves a pollable task. The loader requires the per_task query hooks; the host
// only reaches them for a task this plugin never creates.
export function buildQueryRequest(ctx) {
  return {
    url: versionedRoot(ctx.baseUrl) + "/models/" + encodeURIComponent(upstreamModel(ctx)),
    method: "GET",
    headers: geminiHeaders(ctx),
  };
}

export function parseTaskResult() {
  return { status: "UNKNOWN", reason: "gemini image generation completes in the submit response" };
}

export const protocols = {
  openai_image: {
    decodeRequest: function (ctx) {
      const normalized = normalizedRequestBody(ctx);
      return {
        kind: "submit",
        model: trimmed(ctx.model),
        action: normalized.hasReferences ? "image_to_image" : "text_to_image",
        requestBody: normalized.requestBody,
      };
    },
    render: function (ctx, task) {
      const body = (task && task.data) || {};
      const data = [];
      for (const image of responseImages(body)) data.push(image.url ? { url: image.url } : { b64_json: image.data });
      const revised = responseTexts(body).join("\n");
      if (revised) {
        for (const entry of data) entry.revised_prompt = revised;
      }
      const response = { data: data };
      const createdAt = Number(task && task.created_at);
      if (Number.isFinite(createdAt) && createdAt > 0) response.created = createdAt;
      return response;
    },
  },
};

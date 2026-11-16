// 官方契约：https://docs.magnific.com/llms.txt 与 authentication 页面链接的 OpenAPI
// 核对日期：2026-10-20。端点按协议登记，未知扩展参数保留，不推断未登记模型的路径。
// REST 任务没有标准实际积分字段；这里报告图片数/请求时长及规格，由管理员配置价格。
// 团队积分汇总不能归因到单个并发任务；上传和对账使用 scripts/magnific.py。
// 普通生成 URL 通常 30 分钟过期，抠图 URL 约 5 分钟；本插件不提供永久媒体存储。
const AI = "/v1/ai/";
const MAX_IMAGE_N = 128; // 与宿主 dto.MaxImageN 一致
const MAX_SECONDS = 3600; // 与宿主 relaycommon.MaxTaskDurationSeconds 一致
const MB = 1024 * 1024;
const ASPECTS = {
  "1:1": "square_1_1", "4:3": "classic_4_3", "3:4": "traditional_3_4",
  "16:9": "widescreen_16_9", "9:16": "social_story_9_16", "3:2": "standard_3_2",
  "2:3": "portrait_2_3", "2:1": "horizontal_2_1", "1:2": "vertical_1_2",
  "4:5": "social_post_4_5", "5:4": "social_5_4", "21:9": "film_horizontal_21_9", "3:1": "banner_3_1",
};
const IMAGE_SPECS = [
  { model: "mystic", path: "mystic", inputs: ["structure_reference", "style_reference"], encoding: "base64", resolution: ["2k", "1k", "4k"], vendorModels: ["realism", "fluid", "zen", "flexible", "super_real", "editorial_portraits"] },
  { model: "magnific-upscaler", path: "image-upscaler", operation: "upscale", inputs: ["image"], encoding: "base64", required: ["image"], scales: ["2x", "4x", "8x", "16x"] },
  { model: "magnific-upscaler-precision", path: "image-upscaler-precision", operation: "upscale", inputs: ["image"], encoding: "base64", required: ["image"] },
  { model: "magnific-upscaler-precision-v2", path: "image-upscaler-precision-v2", operation: "upscale", inputs: ["image"], required: ["image"], scales: ["2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16"] },
  { model: "magnific-relight", path: "image-relight", operation: "relight", inputs: ["image", "transfer_light_from_reference_image", "transfer_light_from_lightmap"], required: ["image"] },
  { model: "magnific-style-transfer", path: "image-style-transfer", operation: "style", inputs: ["image", "reference_image"], required: ["image", "reference_image"] },
  { model: "magnific-expand-flux-pro", path: "image-expand/flux-pro", operation: "expand", inputs: ["image"], encoding: "base64", required: ["image"] },
  { model: "magnific-expand-seedream-4.5", path: "image-expand/seedream-v4-5", operation: "expand", inputs: ["image"], required: ["image", "left", "right", "top", "bottom"] },
  { model: "magnific-remove-background", path: "beta/remove-background", operation: "background", inputs: ["image_url"], encoding: "url", required: ["image_url"], immediate: true },
  { model: "flux-dev", path: "text-to-image/flux-dev" },
  { model: "flux-pro-v1-1", path: "text-to-image/flux-pro-v1-1", required: ["prompt"] },
  { model: "flux-kontext-pro", path: "text-to-image/flux-kontext-pro", required: ["prompt"], inputs: ["input_image"], encoding: "url" },
  { model: "flux-2-pro", path: "text-to-image/flux-2-pro", required: ["prompt"], inputs: ["input_image", "input_image_2", "input_image_3", "input_image_4"], encoding: "base64", dimensions: [256, 1440, 1024, 768] },
  { model: "flux-2-turbo", path: "text-to-image/flux-2-turbo", required: ["prompt"], dimensions: [512, 2048, 1024, 1024], nestedDimensions: true },
  { model: "flux-2-klein", path: "text-to-image/flux-2-klein", required: ["prompt"], inputs: ["input_image", "input_image_2", "input_image_3", "input_image_4"], encoding: "base64", resolution: ["1k", "2k"] },
  { model: "seedream-4.0", path: "text-to-image/seedream-v4", editPath: "text-to-image/seedream-v4-edit", required: ["prompt"], arrayInput: "reference_images", maxInputs: 5 },
  { model: "seedream-4.5", path: "text-to-image/seedream-v4-5", editPath: "text-to-image/seedream-v4-5-edit", required: ["prompt"], arrayInput: "reference_images", maxInputs: 5 },
  { model: "nano-banana-pro", path: "text-to-image/nano-banana-pro", required: ["prompt"], arrayInput: "reference_images", maxInputs: 14, encoding: "url", objectInput: true, colonAspect: true, resolution: ["2K", "1K", "4K", "low", "medium", "high"] },
  { model: "gpt-image-2", path: "text-to-image/gpt-image-2", editPath: "text-to-image/gpt-image-2-edit", required: ["prompt"], arrayInput: "reference_images", maxInputs: 16, uploadIds: true, maxBytes: 20 * MB, maxImages: 10, resolution: ["1k", "2k", "4k"], quality: ["high", "low", "medium"] },
  { model: "gpt-image-2.5", path: "text-to-image/gpt-image-2-5", editPath: "text-to-image/gpt-image-2-5-edit", required: ["prompt"], arrayInput: "reference_images", maxInputs: 16, uploadIds: true, maxBytes: 20 * MB, maxImages: 10, resolution: ["1k", "2k", "4k"], quality: ["medium", "low", "high", "xhigh", "max"], variants: ["flare", "sunburst"] },
];
const VIDEO_SPECS = [
  { model: "kling-v2-1-pro", path: "image-to-video/kling-v2-1-pro", queryPath: "image-to-video/kling-v2-1", inputs: ["image", "image_tail", "static_mask"], durations: ["5", "10"], requireImage: true },
  { model: "kling-v2-5-pro", path: "image-to-video/kling-v2-5-pro", inputs: ["image"], durations: ["5", "10"] },
  { model: "kling-v2-6-pro", path: "image-to-video/kling-v2-6-pro", queryPath: "image-to-video/kling-v2-6", inputs: ["image"], durations: ["5", "10"], audio: true },
  { model: "minimax-hailuo-02-1080p", path: "image-to-video/minimax-hailuo-02-1080p", inputs: ["first_frame_image", "last_frame_image"], durations: [6], maxBytes: 20 * MB },
  { model: "minimax-hailuo-2.3-1080p", path: "image-to-video/minimax-hailuo-2-3-1080p", inputs: ["first_frame_image", "last_frame_image"], durations: [6], maxBytes: 20 * MB },
  { model: "seedance-pro-1080p", path: "image-to-video/seedance-pro-1080p", inputs: ["image"], required: ["prompt"], durations: ["5", "10"] },
  { model: "wan-2.6-1080p", path: "text-to-video/wan-v2-6-1080p", imagePath: "image-to-video/wan-v2-6-1080p", inputs: ["image"], encoding: "url", required: ["prompt"], durations: ["5", "10", "15"], sizes: ["1920*1080", "1080*1920", "1440*1440", "1632*1248", "1248*1632"] },
  { model: "ltx-2-pro", path: "text-to-video/ltx-2-pro", imagePath: "image-to-video/ltx-2-pro", inputs: ["image_url"], encoding: "url", required: ["prompt"], durations: [6, 8, 10], resolution: ["1080p", "1440p", "2160p"], audio: false, fps: [25, 50] },
  { model: "runway-gen4-turbo", path: "image-to-video/runway-gen4-turbo", inputs: ["image"], required: ["image"], durations: [10, 5], ratios: ["1280:720", "720:1280", "1104:832", "832:1104", "960:960", "1584:672"] },
  { model: "veo-3.1", path: "text-to-video/veo-3-1", imagePath: "image-to-video/veo-3-1", inputs: ["image", "image_end"], required: ["prompt"], durations: [8, 4, 6], resolution: ["720p", "1080p", "4k"], audio: true, colonAspect: true },
  { model: "veo-3.1-fast", path: "text-to-video/veo-3-1-fast", imagePath: "image-to-video/veo-3-1-fast", inputs: ["image", "image_end"], required: ["prompt"], durations: [8, 4, 6], resolution: ["720p", "1080p", "4k"], audio: true, colonAspect: true },
];
const RESOLUTION = { enum: ["auto", "1k", "2k", "4k", "low", "medium", "high", "720p", "1080p", "1440p", "2160p"], description: { en: "Output resolution", zh: "输出分辨率" } };
const IMAGE_USAGE = {
  image_count: { type: "number", unit: "count", unitLabel: { en: "image", zh: "张" }, description: { en: "Image generation or editing unit price", zh: "图片生成或编辑单价" } },
  resolution: RESOLUTION,
  quality: { enum: ["auto", "low", "medium", "high", "xhigh", "max"], description: { en: "Output image quality", zh: "输出图片质量" } },
  operation: { enum: ["generate", "edit", "upscale", "relight", "style", "expand", "background"], description: { en: "Generate or edit images", zh: "生成或编辑图片" } },
  scale_factor: { enum: ["auto", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16"], description: { en: "Image upscale factor", zh: "图片放大倍数" } },
  variant: { enum: ["auto", "realism", "fluid", "zen", "flexible", "super_real", "editorial_portraits", "flare", "sunburst"], description: { en: "Generation variant", zh: "生成版本" } },
  output_megapixels: { type: "number", unit: "count", unitLabel: { en: "megapixel", zh: "百万像素" }, description: { en: "Image generation unit price", zh: "图片生成单价" } },
};
const VIDEO_USAGE = {
  video_count: { type: "number", unit: "count", unitLabel: { en: "video", zh: "个" }, description: { en: "Video generation unit price", zh: "视频生成单价" } },
  seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  resolution: RESOLUTION,
  audio: { enum: ["auto", "enabled", "disabled"], description: { en: "Audio generation mode", zh: "音频生成模式" } },
  fps: { enum: ["auto", "24", "25", "50"], description: { en: "Output frame rate", zh: "输出帧率" } },
};
const SPECS = IMAGE_SPECS.concat(VIDEO_SPECS);
const ENDPOINTS = {};
const ROUTES = [];
export const native = { task: renderNative };
for (const spec of SPECS) {
  spec.kind = VIDEO_SPECS.includes(spec) ? "video" : "image";
  for (const path of [spec.path, spec.editPath, spec.imagePath].filter(Boolean)) {
    const endpoint = Object.assign({}, spec, { path: path, editing: path === spec.editPath, imageVideo: path === spec.imagePath });
    ENDPOINTS[path] = endpoint;
    const hook = "submit_" + path.replace(/[^a-zA-Z0-9]/g, "_");
    native[hook] = function (ctx) { return decode(ctx, endpoint); };
    ROUTES.push({ method: "POST", path: "/magnific" + AI + path, type: "submit", decode: hook, render: "task", retainResult: !spec.immediate });
    if (spec.immediate) continue;
    const query = "/magnific" + AI + (spec.queryPath || path) + "/:task_id";
    ROUTES.push({ method: "GET", path: query, type: "query", render: "task" });
    if (spec.kind === "video") {
      ROUTES.push({ method: "GET", path: query + "/content", type: "content" });
      ROUTES.push({ method: "HEAD", path: query + "/content", type: "content" });
    }
  }
}
export const meta = {
  apiVersion: 1, key: "magnific", name: "Magnific", icon: "text:M", version: "1.0.0",
  author: { name: "new-api" }, website: "https://www.magnific.com", baseUrl: "https://api.magnific.com",
  description: { en: "Image generation, editing, upscaling and video generation via Magnific", zh: "通过 Magnific 生成、编辑、放大图片和生成视频" },
  models: SPECS.map(function (s) { return s.model; }), fetchMode: "per_task", auth: { type: "api_key" },
  upstreams: ["vendor", "new_api"], routes: ROUTES,
  protocols: [
    { name: "openai_image", models: IMAGE_SPECS.map(function (s) { return s.model; }) },
    { name: "openai_video", models: VIDEO_SPECS.map(function (s) { return s.model; }) },
  ],
  usageSchema: IMAGE_USAGE,
  usageProfiles: [{ models: VIDEO_SPECS.map(function (s) { return s.model; }), schema: VIDEO_USAGE }],
};

function text(v) { return typeof v === "string" ? v.trim() : ""; }
function object(v) { return v !== null && typeof v === "object" && !Array.isArray(v); }
function has(v) { return v !== undefined && v !== null && v !== ""; }
function integer(v, min, max, name) {
  if (typeof v !== "number" || !Number.isInteger(v) || v < min || v > max) throw new Error(name + " must be an integer between " + min + " and " + max);
  return v;
}
function enumValue(v, values, name) {
  if (!values.includes(v)) throw new Error(name + " must be one of: " + values.join(", "));
  return v;
}

// 任何嵌套扩展中的数量也必须有界，不能借 metadata/parameters 绕过计费校验。
function validateQuantities(value, depth) {
  if (!value || typeof value !== "object") return;
  if (depth > 32) throw new Error("Request nesting exceeds 32 levels");
  for (const key of Object.keys(value)) {
    const v = value[key];
    const normalized = key.toLowerCase().replace(/[_-]/g, "");
    if (["n", "numimages", "numoutputs", "batchsize", "batchcount", "taskcount", "numvideos"].includes(normalized)) {
      integer(v, 1, MAX_IMAGE_N, key);
      // 本平台只支持顶层 GPT num_images；其他批量字段不能被未知参数透传绕过。
      if (depth > 0 || !["n", "num_images"].includes(key)) {
        if (v !== 1) throw new Error(key + " batching is not supported by this plugin");
      }
    }
    if (["duration", "seconds", "durationseconds"].includes(normalized)) {
      if ((typeof v !== "number" && typeof v !== "string") || !/^\d+(\.\d+)?$/.test(String(v))) throw new Error(key + " must be a positive duration");
      const n = Number(v);
      if (!Number.isFinite(n) || n <= 0 || n > MAX_SECONDS) throw new Error(key + " must be between 0 and 3600 seconds");
    }
    if (v && typeof v === "object") validateQuantities(v, depth + 1);
  }
}

function decode(ctx, endpoint) {
  const body = ctx.body || {};
  let payload;
  if (body.kind === "json") {
    if (!object(body.value)) throw new Error("Request body must be a JSON object");
    payload = utils.json.clone(body.value);
  } else if (body.kind === "multipart" || body.kind === "form") {
    payload = {};
    for (const key of Object.keys(body.fields || {})) {
      const values = body.fields[key];
      if (values.length !== 1) throw new Error(key + " must be provided once; use a JSON array for multiple references");
      const value = values[0];
      const literalText = ["prompt", "negative_prompt", "user", "webhook_url"].includes(key);
      if (!literalText && (["n", "num_images", "seed", "width", "height", "left", "right", "top", "bottom", "fps", "guidance_scale", "cfg_scale", "creativity", "hdr", "resemblance", "fractality", "structure_strength", "style_strength", "sharpen", "smart_grain", "ultra_detail"].includes(key) || value === "true" || value === "false" || /^[\[{]/.test(value.trim()))) {
        try { payload[key] = JSON.parse(value); } catch (_) { throw new Error(key + " must contain valid JSON"); }
      } else payload[key] = value;
    }
  } else throw new Error("JSON, form or multipart body required");
  const operation = endpoint ? "native" : (ctx.operation || "video");
  const model = endpoint ? endpoint.model : text(ctx.model) || text(payload.model);
  if (!model) throw new Error("model is required");
  if (!endpoint && payload.metadata !== undefined) {
    if (!object(payload.metadata)) throw new Error("metadata must be an object");
    for (const key of Object.keys(payload.metadata)) {
      if (key === "model") throw new Error("Use vendor_model for the Magnific Mystic variant");
      if (payload[key] !== undefined) throw new Error("Duplicate parameter in metadata: " + key);
      payload[key] = payload.metadata[key];
    }
    delete payload.metadata;
  }
  validateQuantities(payload, 0);
  if (payload.stream !== undefined && payload.stream !== false) throw new Error("Magnific image/video submission does not support streaming");
  if (payload.response_format !== undefined) enumValue(payload.response_format, ["url", "b64_json"], "response_format");
  const request = { payload: payload, operation: operation, files: utils.json.clone(body.files || []) };
  if (endpoint) request.endpoint = endpoint.path;
  const intent = { kind: "submit", model: model, action: endpoint ? endpoint.path : operation, requestBody: request };
  // 别名在选中渠道后才有 upstreamModel；届时 extractUsage/buildSubmitRequest 再校验真实规格。
  if (endpoint || SPECS.some(function (s) { return s.model === model; })) prepare({ model: model, requestBody: request });
  return intent;
}

function media(value, spec, field) {
  if (object(value) && text(value.__fileRef)) return value;
  if (!text(value)) throw new Error(field + " must contain an image reference");
  const v = value.trim();
  const url = /^https?:\/\/[^\s]+$/i.test(v);
  if (spec.encoding === "url") {
    if (!url && !(spec.objectInput && /^gs:\/\//.test(v))) throw new Error(field + " requires a public image URL; upload files with scripts/magnific.py first");
    return v;
  }
  if (url) {
    if (spec.encoding === "base64") throw new Error(field + " requires Base64 or a multipart file; this endpoint does not accept image URLs");
    return v;
  }
  if (spec.uploadIds && /^upl_[a-zA-Z0-9_-]+$/.test(v)) return v;
  const encoded = v.replace(/^data:image\/[a-zA-Z0-9.+-]+;base64,/, "");
  if (!/^[a-zA-Z0-9+/]+={0,2}$/.test(encoded)) throw new Error(field + " must be an image URL or Base64 image");
  if (encoded.length * 3 / 4 > (spec.maxBytes || 10 * MB) + 2) throw new Error(field + " exceeds the model image size limit");
  return encoded;
}

function prepare(ctx) {
  const request = ctx.requestBody;
  if (!object(request) || !object(request.payload)) throw new Error("Magnific normalized request is missing");
  const model = ctx.upstreamModel || ctx.model;
  let spec = request.endpoint ? ENDPOINTS[request.endpoint] : SPECS.find(function (s) { return s.model === model; });
  if (!spec) throw new Error("Magnific model is not registered: " + model);
  const payload = utils.json.clone(request.payload);
  const protocol = request.operation !== "native";
  const files = request.files || [];
  if (protocol) {
    delete payload.model;
    delete payload.response_format;
    delete payload.stream;
    delete payload.user;
    if (payload.vendor_model !== undefined) { payload.model = payload.vendor_model; delete payload.vendor_model; }
    if (spec.kind === "image" && request.operation === "edit" && !spec.inputs && !spec.arrayInput) throw new Error("This Magnific model does not support image editing");
    if (request.operation === "edit" && spec.editPath) spec = ENDPOINTS[spec.editPath];
  }
  const references = [];
  if (protocol) {
    for (const key of spec.kind === "video" ? ["input_reference"] : ["image", "image[]", "images"]) {
      if (!has(payload[key])) continue;
      const values = Array.isArray(payload[key]) ? payload[key] : [payload[key]];
      for (const value of values) references.push(value);
      delete payload[key];
    }
    if (payload.mask !== undefined) throw new Error("A generic mask is not supported by these Magnific endpoints");
  }
  let totalBytes = 0;
  for (const file of files) {
    if (!/^image\/(png|jpeg|webp)$/.test(file.mimeType)) throw new Error("Uploaded references must be PNG, JPEG or WebP images");
    if (file.size > (spec.maxBytes || 10 * MB)) throw new Error("Uploaded image exceeds the model size limit");
    totalBytes += file.size;
    if (spec.encoding === "url") throw new Error("This model requires image URLs; upload files with scripts/magnific.py first");
    const value = { __fileRef: file.ref, encoding: "base64", maxBytes: spec.maxBytes || 10 * MB };
    if (protocol && ["image", "image[]", "images", "input_reference"].includes(file.field)) references.push(value);
    else if (spec.arrayInput && [spec.arrayInput, spec.arrayInput + "[]"].includes(file.field)) {
      if (payload[spec.arrayInput] === undefined) payload[spec.arrayInput] = [];
      if (!Array.isArray(payload[spec.arrayInput])) throw new Error(spec.arrayInput + " must be an array");
      payload[spec.arrayInput].push(value);
    } else if ((spec.inputs || []).includes(file.field)) {
      if (has(payload[file.field])) throw new Error("Duplicate image field: " + file.field);
      payload[file.field] = value;
    } else throw new Error("Unsupported uploaded file field: " + file.field);
  }
  if (totalBytes > 64 * MB) throw new Error("Total image upload exceeds 64 MiB");
  if (references.length) {
    if (spec.arrayInput) {
      if (has(payload[spec.arrayInput])) throw new Error("Provide either image or " + spec.arrayInput + ", not both");
      payload[spec.arrayInput] = references;
    } else {
      if (references.length > (spec.inputs || []).length) throw new Error("Too many image references for this model");
      for (let i = 0; i < references.length; i++) {
        const field = spec.inputs[i];
        if (has(payload[field])) throw new Error("Duplicate image field: " + field);
        payload[field] = references[i];
      }
    }
  }
  if (spec.arrayInput && payload[spec.arrayInput] !== undefined) {
    const values = payload[spec.arrayInput];
    if (!Array.isArray(values) || values.length > spec.maxInputs) throw new Error(spec.arrayInput + " exceeds the supported reference count");
    payload[spec.arrayInput] = values.map(function (v) {
      if (!spec.objectInput) return media(v, spec, spec.arrayInput);
      let copy;
      if (object(v)) copy = utils.json.clone(v);
      else if (protocol) {
        const extension = /\.(png|jpe?g|webp)(?:[?#]|$)/i.exec(text(v));
        const mimeType = payload.reference_mime_type || (extension ? "image/" + (extension[1].toLowerCase().startsWith("jp") ? "jpeg" : extension[1].toLowerCase()) : "");
        if (!mimeType) throw new Error("Set reference_mime_type for image URLs without a PNG/JPEG/WebP extension");
        copy = { image: v, mime_type: mimeType };
      } else throw new Error("reference_images entries require image and mime_type");
      copy.image = media(copy.image, spec, "reference_images.image");
      enumValue(copy.mime_type, ["image/png", "image/jpeg", "image/webp"], "mime_type");
      return copy;
    });
    if (!spec.editing && spec.editPath && values.length) {
      if (!protocol) throw new Error("Use the native edit endpoint for reference_images");
      spec = ENDPOINTS[spec.editPath];
    }
  }
  if (protocol) delete payload.reference_mime_type;
  for (const field of spec.inputs || []) if (has(payload[field])) payload[field] = media(payload[field], spec, field);
  const hasImages = (spec.inputs || []).some(function (f) { return has(payload[f]); }) || !!(spec.arrayInput && (payload[spec.arrayInput] || []).length);
  if (protocol && request.operation === "edit" && !hasImages) throw new Error("Image editing requires an image reference");
  if (spec.editing && spec.model !== "seedream-4.0" && !hasImages) throw new Error("The edit endpoint requires reference_images");
  if (spec.imagePath && hasImages && protocol) spec = ENDPOINTS[spec.imagePath];
  if (spec.imageVideo && !has(payload[spec.inputs[0]])) throw new Error("The image-to-video endpoint requires an input image");
  if (spec.requireImage && !has(payload.image) && !has(payload.image_tail)) throw new Error("This model requires an input image");
  for (const field of spec.required || []) if (!has(payload[field])) throw new Error(field + " is required");
  if (spec.kind === "video" && !hasImages && !text(payload.prompt)) throw new Error("A prompt or input image is required");
  if (payload.prompt !== undefined && typeof payload.prompt !== "string") throw new Error("prompt must be a string");
  if (spec.vendorModels && payload.model !== undefined) enumValue(payload.model, spec.vendorModels, "model");
  if (!spec.vendorModels && payload.model !== undefined) throw new Error("This native Magnific endpoint selects its model by path");
  if (payload.n !== undefined) {
    integer(payload.n, 1, spec.maxImages || 1, "n");
    if (payload.num_images !== undefined && payload.n !== payload.num_images) throw new Error("n and num_images must agree");
    if (spec.maxImages) payload.num_images = payload.n;
    delete payload.n;
  }
  if (payload.num_images !== undefined) integer(payload.num_images, 1, spec.maxImages || 1, "num_images");
  if (spec.maxImages && payload.num_images === undefined) payload.num_images = 1;
  for (const field of ["resolution", "quality"]) {
    if (spec[field]) {
      if (payload[field] === undefined || (protocol && payload[field] === "auto")) payload[field] = spec[field][0];
      if (protocol && field === "resolution") {
        const equivalent = spec[field].find(function (v) { return v.toLowerCase() === String(payload[field]).toLowerCase(); });
        if (equivalent) payload[field] = equivalent;
      }
      enumValue(payload[field], spec[field], field);
    } else if (payload[field] !== undefined) throw new Error(field + " is not supported by this Magnific endpoint");
  }
  if (spec.variants) {
    if (payload.variant === undefined) payload.variant = spec.variants[0];
    enumValue(payload.variant, spec.variants, "variant");
  }
  if (spec.scales) {
    if (payload.scale_factor === undefined && spec.model !== "magnific-upscaler-precision-v2") payload.scale_factor = spec.scales[0];
    if (payload.scale_factor !== undefined) enumValue(String(payload.scale_factor), spec.scales, "scale_factor");
  } else if (payload.scale_factor !== undefined) throw new Error("scale_factor is not supported by this endpoint");
  if (protocol && has(payload.size) && payload.size !== "auto") {
    const match = /^(\d+)[x*](\d+)$/.exec(payload.size);
    if (!match) throw new Error("size must be width x height");
    const width = integer(Number(match[1]), 1, 8192, "width");
    const height = integer(Number(match[2]), 1, 8192, "height");
    if (spec.kind === "video" && spec.resolution) {
      const shortSide = Math.min(width, height);
      const resolution = ({ 720: "720p", 1080: "1080p", 1440: "1440p", 2160: spec.model.startsWith("veo-") ? "4k" : "2160p" })[shortSide];
      if (!resolution || !spec.resolution.includes(resolution)) throw new Error("size is not supported by this video's resolution options");
      if (has(request.payload.resolution) && request.payload.resolution !== "auto" && String(request.payload.resolution).toLowerCase() !== resolution) throw new Error("size and resolution conflict");
      payload.resolution = resolution;
    }
    if (spec.dimensions) {
      const dimensions = { width: width, height: height };
      if (spec.nestedDimensions) payload.image_size = dimensions;
      else { payload.width = width; payload.height = height; }
    } else if (spec.sizes) payload.size = width + "*" + height;
    else if (spec.ratios) payload.ratio = width + ":" + height;
    else {
      let a = width, b = height;
      while (b) { const remainder = a % b; a = b; b = remainder; }
      const ratio = width / a + ":" + height / a;
      if (spec.model.startsWith("veo-") && !["16:9", "9:16"].includes(ratio)) throw new Error("Veo size must use 16:9 or 9:16");
      if (spec.model === "ltx-2-pro" && ratio !== "16:9") throw new Error("LTX size must use 16:9");
      if (!ASPECTS[ratio]) throw new Error("Unsupported aspect ratio; provide a documented aspect_ratio instead");
      if (payload.aspect_ratio !== undefined && payload.aspect_ratio !== ratio && payload.aspect_ratio !== ASPECTS[ratio]) throw new Error("size and aspect_ratio conflict");
      if (spec.model !== "ltx-2-pro") payload.aspect_ratio = spec.colonAspect ? ratio : ASPECTS[ratio];
    }
    if (!spec.sizes) delete payload.size;
  } else if (protocol && payload.size === "auto") delete payload.size;
  if (protocol && !spec.colonAspect && ASPECTS[payload.aspect_ratio]) payload.aspect_ratio = ASPECTS[payload.aspect_ratio];
  if (spec.model === "seedream-4.5" && payload.aspect_ratio === "film_horizontal_21_9") payload.aspect_ratio = "cinematic_21_9";
  if (spec.dimensions) {
    const d = spec.nestedDimensions ? (payload.image_size || {}) : payload;
    for (const field of ["width", "height"]) if (d[field] !== undefined) integer(d[field], spec.dimensions[0], spec.dimensions[1], field);
  }
  for (const field of ["left", "right", "top", "bottom"]) if (payload[field] !== undefined) integer(payload[field], 0, 2048, field);
  if (spec.operation === "relight" && has(payload.transfer_light_from_reference_image) && has(payload.transfer_light_from_lightmap)) throw new Error("Light reference image and lightmap are mutually exclusive");
  if (spec.kind === "video") {
    if (payload.seconds !== undefined) {
      if (payload.duration !== undefined && Number(payload.duration) !== Number(payload.seconds)) throw new Error("seconds and duration must agree");
      payload.duration = payload.seconds;
      delete payload.seconds;
    }
    const duration = payload.duration === undefined ? spec.durations[0] : payload.duration;
    if ((typeof duration !== "number" && typeof duration !== "string") || !spec.durations.some(function (v) { return String(v) === String(duration); })) throw new Error("duration must be one of: " + spec.durations.join(", "));
    payload.duration = typeof spec.durations[0] === "string" ? String(duration) : Number(duration);
    if (spec.audio !== undefined) {
      if (payload.generate_audio === undefined) payload.generate_audio = spec.audio;
      if (typeof payload.generate_audio !== "boolean") throw new Error("generate_audio must be a boolean");
    } else if (payload.generate_audio !== undefined) throw new Error("generate_audio is not supported by this endpoint");
    if (spec.fps) {
      if (payload.fps === undefined) payload.fps = spec.fps[0];
      enumValue(payload.fps, spec.fps, "fps");
    }
    if (spec.sizes && payload.size !== undefined) enumValue(payload.size, spec.sizes, "size");
    if (spec.ratios && payload.ratio !== undefined) enumValue(payload.ratio, spec.ratios, "ratio");
  }
  validateQuantities(payload, 0);
  return { spec: spec, payload: payload };
}

function headers(ctx) {
  if (ctx.upstream && ctx.upstream.kind === "new_api") return { Authorization: ctx.authHeader || "Bearer " + ctx.apiKey };
  return { "x-magnific-api-key": ctx.apiKey };
}
function upstreamURL(ctx, path) {
  return ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/magnific" : "") + AI + path;
}
export function buildSubmitRequest(ctx) {
  const prepared = prepare(ctx);
  const requestHeaders = headers(ctx);
  requestHeaders["Content-Type"] = prepared.spec.immediate && (!ctx.upstream || ctx.upstream.kind !== "new_api") ? "application/x-www-form-urlencoded" : "application/json";
  let body = prepared.payload;
  if (requestHeaders["Content-Type"] === "application/x-www-form-urlencoded") {
    body = Object.keys(body).map(function (k) { return encodeURIComponent(k) + "=" + encodeURIComponent(typeof body[k] === "object" ? JSON.stringify(body[k]) : String(body[k])); }).join("&");
  }
  return { url: upstreamURL(ctx, prepared.spec.path), method: "POST", headers: requestHeaders, body: body };
}

function payloadOf(body) {
  if (!object(body)) throw new Error("Magnific response must be a JSON object");
  return object(body.data) ? body.data : body;
}
function resultURLs(body, immediate) {
  const data = payloadOf(body);
  if (immediate) {
    const url = text(data.high_resolution) || text(data.url);
    return /^https?:\/\//i.test(url) ? [url] : [];
  }
  if (!Array.isArray(data.generated)) return [];
  return data.generated.filter(function (v) { return typeof v === "string" && /^https?:\/\/[^\s]+$/i.test(v); });
}
function normalizeResult(body, spec) {
  const data = payloadOf(body);
  const statuses = { CREATED: "QUEUED", IN_PROGRESS: "IN_PROGRESS", COMPLETED: "SUCCESS", FAILED: "FAILURE" };
  let status = spec.immediate ? "SUCCESS" : statuses[data.status || data.task_status] || "UNKNOWN";
  const urls = resultURLs(body, spec.immediate);
  let reason = text(data.error_message) || text(data.message) || text(data.error) || (object(data.error) ? text(data.error.message) : "");
  if (status === "SUCCESS" && !urls.length) { status = "FAILURE"; reason = "Magnific completed without a downloadable result"; }
  if (status === "FAILURE" && !reason) reason = "Magnific generation failed";
  const result = { status: status, progress: status === "SUCCESS" ? "100%" : "0%" };
  if (reason) result.reason = reason;
  if (urls.length) result.url = urls[0];
  return result;
}
export function parseSubmitResponse(ctx, response) {
  const prepared = prepare(ctx);
  const data = payloadOf(response.body);
  const result = normalizeResult(response.body, prepared.spec);
  const taskId = text(data.task_id);
  if (!taskId && !["SUCCESS", "FAILURE"].includes(result.status)) throw new Error("Magnific response is missing task_id");
  const parsed = { taskId: taskId || ctx.publicTaskId, taskData: response.body, state: { endpoint: prepared.spec.path } };
  if (["SUCCESS", "FAILURE"].includes(result.status)) parsed.immediate = result;
  return parsed;
}
function taskSpec(ctx) {
  const path = ctx.state && ctx.state.endpoint;
  const spec = ENDPOINTS[path] || ENDPOINTS[ctx.action];
  if (!spec) throw new Error("Magnific persisted task endpoint is missing");
  return spec;
}
export function buildQueryRequest(ctx) {
  const spec = taskSpec(ctx);
  if (spec.immediate) throw new Error("This synchronous endpoint has no upstream task query");
  return { url: upstreamURL(ctx, spec.queryPath || spec.path) + "/" + encodeURIComponent(ctx.taskId), method: "GET", headers: headers(ctx) };
}
export function parseTaskResult(ctx, body) { return normalizeResult(body, taskSpec(ctx)); }
export function extractUsage(ctx) {
  const prepared = prepare(ctx), spec = prepared.spec, p = prepared.payload;
  if (ctx.usagePurpose === "billing_ratios") return spec.kind === "image" ? { image_count: p.num_images || 1 } : { seconds: Number(p.duration) };
  const resolution = text(p.resolution).toLowerCase() || (spec.model.endsWith("1080p") ? "1080p" : "auto");
  if (spec.kind === "video") return {
    video_count: 1, seconds: Number(p.duration), resolution: resolution,
    audio: p.generate_audio === undefined ? "auto" : p.generate_audio ? "enabled" : "disabled",
    fps: p.fps === undefined ? "auto" : String(p.fps),
  };
  let megapixels = 0;
  if (spec.dimensions) {
    const d = spec.nestedDimensions ? (p.image_size || {}) : p;
    megapixels = (d.width === undefined ? spec.dimensions[2] : d.width) * (d.height === undefined ? spec.dimensions[3] : d.height) / 1000000;
  }
  return {
    image_count: p.num_images || 1, resolution: resolution, quality: p.quality || "auto",
    operation: spec.operation || (spec.editing || ctx.requestBody.operation === "edit" ? "edit" : "generate"),
    scale_factor: p.scale_factor === undefined ? "auto" : String(p.scale_factor).replace(/x$/, ""),
    variant: p.variant || (spec.vendorModels ? p.model || spec.vendorModels[0] : "auto"), output_megapixels: megapixels,
  };
}
export function extractUsageOnSubmit(ctx, data) {
  const spec = prepare(ctx).spec;
  if (spec.kind !== "image") return {};
  const count = resultURLs(data, spec.immediate).length;
  return count >= 1 && count <= MAX_IMAGE_N ? { image_count: count } : {};
}
export function extractUsageOnComplete(ctx, result, data) {
  if (result.status !== "SUCCESS") return {};
  const spec = taskSpec(ctx);
  const count = resultURLs(data, spec.immediate).length;
  if (!Number.isInteger(count) || count < 1 || count > MAX_IMAGE_N) return {};
  // 没有实际时长/积分字段时保留已冻结的请求事实，不能把任务条目数当成图片数。
  return spec.kind === "image" ? { image_count: count } : { video_count: count };
}

function renderNative(ctx, task) {
  const body = object(task.data) ? utils.json.clone(task.data) : { data: {} };
  const data = payloadOf(body);
  // 抠图是无任务的同步响应，保持其 original/high_resolution/preview/url 外壳。
  if (!has(data.task_id) && (data.high_resolution || data.url)) return body;
  data.task_id = task.task_id;
  const statuses = { QUEUED: "CREATED", NOT_START: "CREATED", SUBMITTED: "CREATED", IN_PROGRESS: "IN_PROGRESS", SUCCESS: "COMPLETED", FAILURE: "FAILED" };
  const status = statuses[task.status];
  if (status) {
    if (data.task_status !== undefined) data.task_status = status;
    else data.status = status;
  }
  if (task.fail_reason) data.error_message = task.fail_reason;
  return body;
}
export const protocols = {
  openai_image: {
    decodeRequest: function (ctx) { return decode(ctx); },
    render: function (ctx, task) {
      const data = payloadOf(task.data);
      const urls = resultURLs(task.data, !!(data.high_resolution || data.url) && !Array.isArray(data.generated));
      if (!urls.length) throw new Error("Magnific returned no images");
      return { created: task.created_at || utils.unixNow(), data: urls.map(function (url) { return { url: url }; }) };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) { return decode(Object.assign({}, ctx, { operation: "video" })); },
    render: function (ctx, task) {
      const urls = resultURLs(task.data, false);
      const result = { id: task.task_id, object: "video", model: ctx.model, status: task.status };
      if (urls.length) result.url = urls[0];
      return result;
    },
  },
};
export function listArtifacts(ctx) {
  const spec = taskSpec(ctx);
  return resultURLs(ctx.data, spec.immediate).map(function (_, i) { return { key: spec.kind + "_" + i, type: spec.kind }; });
}
export function buildContentRequest(ctx) {
  const match = /^(image|video)_(\d+)$/.exec(ctx.artifactKey || "");
  if (!match) throw new Error("Unknown Magnific artifact");
  const spec = taskSpec(ctx);
  if (match[1] !== spec.kind) throw new Error("Magnific artifact type mismatch");
  const url = resultURLs(ctx.data, spec.immediate)[Number(match[2])];
  if (!url) throw new Error("Magnific artifact was not found");
  // 签名 CDN URL 不携带渠道 Key；宿主负责 GET/HEAD/Range 与媒体下载策略。
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

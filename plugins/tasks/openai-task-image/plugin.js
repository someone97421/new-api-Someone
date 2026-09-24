// OpenAI-compatible asynchronous image generation task gateway served as the OpenAI Images API.
//
// Upstream contract (OpenAI-compatible task site or async image generator):
//   POST {baseUrl}/v1/images/generations
//   GET  {baseUrl}/v1/tasks/{task_id}
//
// Client contract (`POST /v1/images/generations`, `POST /v1/images/edits`):
//   model            required, declared model
//   prompt           required, non-empty text
//   n                optional, integer 1..8, default 1
//   response_format  optional, "url" or "b64_json"
//   image, images, image_url, image_urls
//                    optional reference images: Base64, data URL, or HTTP(S) URL
//   multipart files  optional reference images on /v1/images/edits
//
// Host protocol: openai_image. The host handles client polling until terminal status.

// 站点提交接口路径（站点不同则改这里）
const SUBMIT_PATH = "/v1/images/generations";
// 站点查询接口路径（必须含且仅含一个 {task_id}，站点不同则改这里）
const QUERY_PATH = "/v1/tasks/{task_id}";

// 站点支持的模型列表（必须在插件里声明渠道实际提供的模型名，插件 API 要求且不支持通配符；安装到具体站点时改成该站点的模型名）
const VENDOR_MODELS = [
  "gpt-image-1",
  "seedream-4.0",
  "dall-e-task-3",
];

export const meta = {
  apiVersion: 1,
  key: "openai-task-image",
  name: "OpenAI Task Image",
  description: {
    en: "OpenAI-compatible asynchronous image generation task gateway",
    zh: "OpenAI 兼容的异步图片任务网关",
  },
  version: "1.0.0",
  author: { name: "QuantumNous" },
  models: VENDOR_MODELS,
  fetchMode: "per_task",
  usageSchema: {
    image_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张", "zh-TW": "張", fr: "image", ja: "枚", ru: "изображение", vi: "ảnh" },
      description: { en: "Image generation unit price", zh: "图片生成单价" },
    },
  },
  usageExamples: [
    { label: "1 image", facts: { image_count: 1 } },
  ],
  protocols: ["openai_image"],
};

function trimmed(value) {
  return String(value || "").trim();
}

function isHttpUrl(value) {
  return /^https?:\/\//i.test(trimmed(value));
}

function isDataUrl(value) {
  return /^data:/i.test(trimmed(value));
}

function decodeImageRequest(ctx) {
  const model = trimmed(ctx.model);
  if (!model) throw new Error("model is required");

  let req = {};
  const collectedImages = [];

  if (ctx.body && ctx.body.kind === "json") {
    req = ctx.body.value;
    if (!req || typeof req !== "object" || Array.isArray(req)) {
      throw new Error("request body must be an object");
    }
  } else if (ctx.body && ctx.body.kind === "multipart") {
    const fields = ctx.body.fields || {};
    for (const name of Object.keys(fields)) {
      const vals = fields[name] || [];
      if (vals.length > 1) {
        throw new Error(name + " must be provided once");
      }
      req[name] = vals[0];
    }
    for (const file of ctx.body.files || []) {
      collectedImages.push({
        __fileRef: file.ref,
        encoding: "dataUrl",
        mimeType: trimmed(file.mimeType) || "image/png",
      });
    }
  } else {
    throw new Error("JSON or multipart body required");
  }

  const prompt = typeof req.prompt === "string" ? req.prompt : "";
  if (!trimmed(prompt)) {
    throw new Error("field prompt is required");
  }

  let n = req.n === undefined || req.n === null ? 1 : req.n;
  if (typeof n === "string") {
    n = Number(n);
  }
  if (!Number.isInteger(n) || n < 1 || n > 8) {
    throw new Error("n must be an integer between 1 and 8");
  }

  for (const key of ["image", "images", "image_url", "image_urls"]) {
    const val = req[key];
    if (val !== undefined && val !== null) {
      if (Array.isArray(val)) {
        for (const item of val) {
          if (item) collectedImages.push(item);
        }
      } else {
        collectedImages.push(val);
      }
    }
  }

  const requestBody = {
    model: model,
    prompt: trimmed(prompt),
    n: n,
  };

  const excluded = [
    "model",
    "async",
    "task_count",
    "prompt",
    "n",
    "image",
    "images",
    "image_url",
    "image_urls",
  ];
  for (const [key, value] of Object.entries(req)) {
    if (!excluded.includes(key)) {
      requestBody[key] = value;
    }
  }

  if (collectedImages.length > 0) {
    requestBody.images = collectedImages;
  }

  const action = collectedImages.length > 0 ? "image_to_image" : "text_to_image";
  return {
    kind: "submit",
    model: model,
    action: action,
    requestBody: requestBody,
  };
}

export function buildSubmitRequest(ctx) {
  const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  const headers = { "Content-Type": "application/json" };
  if (ctx.apiKey) {
    headers.Authorization = "Bearer " + trimmed(ctx.apiKey);
  }
  return {
    url: base + SUBMIT_PATH,
    method: "POST",
    headers: headers,
    body: ctx.requestBody || {},
  };
}

function extractImageEntries(data) {
  if (!data || typeof data !== "object") return [];
  const entries = [];
  const addEntry = function (item) {
    if (!item) return;
    if (typeof item === "string" && trimmed(item)) {
      entries.push({ url: trimmed(item) });
      return;
    }
    if (typeof item === "object") {
      const u = trimmed(item.url);
      const b = trimmed(item.b64_json);
      if (u) {
        entries.push({ url: u });
      } else if (b) {
        entries.push({ b64_json: b });
      }
    }
  };

  const candidates = [
    data.data,
    data.results,
    data.images,
    data.output && data.output.results,
    data.output && data.output.data,
    data.output && data.output.images,
  ];
  for (const list of candidates) {
    if (Array.isArray(list) && list.length > 0) {
      for (const item of list) addEntry(item);
      if (entries.length > 0) return entries;
    }
  }

  if (trimmed(data.url)) {
    entries.push({ url: trimmed(data.url) });
  } else if (trimmed(data.b64_json)) {
    entries.push({ b64_json: trimmed(data.b64_json) });
  } else if (data.output && trimmed(data.output.url)) {
    entries.push({ url: trimmed(data.output.url) });
  } else if (data.output && trimmed(data.output.b64_json)) {
    entries.push({ b64_json: trimmed(data.output.b64_json) });
  }
  return entries;
}

export function parseSubmitResponse(ctx, resp) {
  const body = (resp && resp.body) || {};
  const entries = extractImageEntries(body);
  if (entries.length > 0) {
    const firstUrl = entries[0].url || entries[0].b64_json || "";
    const publicId = trimmed(ctx.publicTaskId) || "task_immediate";
    return {
      taskId: publicId,
      taskData: body,
      immediate: {
        status: "SUCCESS",
        progress: "100%",
        url: firstUrl,
      },
    };
  }

  const taskId = trimmed(
    body.id ||
    body.task_id ||
    (body.data && (body.data.id || body.data.task_id)) ||
    (body.output && body.output.task_id)
  );
  if (!taskId) {
    throw new Error("missing task id");
  }
  return {
    taskId: taskId,
    taskData: body,
  };
}

export function buildQueryRequest(ctx) {
  const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  const path = QUERY_PATH.replace("{task_id}", encodeURIComponent(trimmed(ctx.taskId)));
  const headers = {};
  if (ctx.apiKey) {
    headers.Authorization = "Bearer " + trimmed(ctx.apiKey);
  }
  return {
    url: base + path,
    method: "GET",
    headers: headers,
  };
}

export function parseTaskResult(ctx, body) {
  const data = body || {};
  const output = (data && data.output) || {};
  const rawStatus = trimmed(
    data.status ||
    data.state ||
    data.task_status ||
    output.task_status
  ).toLowerCase();

  const dataArray = Array.isArray(data.data) ? data.data : [];
  const resultsArray = Array.isArray(data.results) ? data.results : [];
  const resultUrl = trimmed(
    (dataArray[0] && (dataArray[0].url || dataArray[0].b64_json)) ||
    data.url ||
    output.url ||
    (resultsArray[0] && (resultsArray[0].url || resultsArray[0].b64_json))
  );

  const errObj = data.error || output.error || {};
  const failReason = trimmed(
    errObj.message ||
    data.message ||
    output.message ||
    data.fail_reason ||
    output.fail_reason
  ) || "task failed";

  if (["queued", "pending", "created"].includes(rawStatus)) {
    return { status: "QUEUED" };
  }
  if (["processing", "running", "in_progress"].includes(rawStatus)) {
    const res = { status: "IN_PROGRESS" };
    const progress = data.progress !== undefined ? data.progress : output.progress;
    if (progress !== undefined && progress !== null) {
      res.progress = String(progress).endsWith("%") ? String(progress) : progress + "%";
    }
    return res;
  }
  if (["succeeded", "success", "completed", "done"].includes(rawStatus)) {
    const res = { status: "SUCCESS" };
    if (resultUrl) res.url = resultUrl;
    return res;
  }
  if (["failed", "error", "failure", "cancelled", "canceled"].includes(rawStatus)) {
    return { status: "FAILURE", reason: failReason };
  }
  return { status: "UNKNOWN", reason: "unrecognized status: " + (rawStatus || "empty") };
}

function countImagePayloads(body) {
  if (!body || typeof body !== "object") return 0;
  const candidates = [
    body.data,
    body.results,
    body.images,
    body.output && body.output.results,
    body.output && body.output.data,
    body.output && body.output.images,
  ];
  for (const arr of candidates) {
    if (Array.isArray(arr) && arr.length > 0) {
      let urlCount = 0;
      let b64Count = 0;
      for (const item of arr) {
        if (item && typeof item === "object") {
          if (trimmed(item.url)) urlCount++;
          if (trimmed(item.b64_json)) b64Count++;
        } else if (typeof item === "string" && trimmed(item)) {
          urlCount++;
        }
      }
      const total = Math.max(urlCount, b64Count);
      if (total > 0) return total;
    }
  }
  const directUrl = trimmed(body.url || (body.output && body.output.url));
  const directB64 = trimmed(body.b64_json || (body.output && body.output.b64_json));
  if (directUrl || directB64) {
    return 1;
  }
  return 0;
}

export function extractUsage(ctx) {
  const req = ctx.requestBody || {};
  const count = Number.isInteger(req.n) && req.n > 0 ? req.n : 1;
  return { image_count: count };
}

export function extractUsageOnSubmit(ctx, body) {
  const count = countImagePayloads(body);
  return count > 0 ? { image_count: count } : {};
}

export function extractUsageOnComplete(task, result, body) {
  const data = body || (task && task.data) || {};
  const count = countImagePayloads(data);
  return count > 0 ? { image_count: count } : {};
}

export function listArtifacts(task) {
  if ((task && task.status) !== "SUCCESS") return [];
  const entries = extractImageEntries(task && task.data);
  return entries.map(function (_, i) {
    return { key: "image-" + (i + 1), type: "image", mimeType: "image/png" };
  });
}

export function buildContentRequest(ctx) {
  const entries = extractImageEntries(ctx && ctx.data);
  const match = /^image-(\d+)$/.exec(String(ctx.artifactKey || ""));
  if (!match) throw new Error("artifact_not_found");
  const index = parseInt(match[1], 10) - 1;
  if (index < 0 || index >= entries.length) {
    throw new Error("artifact_not_found");
  }
  const entry = entries[index];
  const method = (ctx.clientRequest && ctx.clientRequest.method) || "GET";

  if (entry.b64_json) {
    const raw = trimmed(entry.b64_json);
    const dataUrl = isDataUrl(raw) ? raw : "data:image/png;base64," + raw;
    return { url: dataUrl, method: method, credentialless: true };
  }

  const rawUrl = trimmed(entry.url);
  if (!rawUrl) throw new Error("artifact_not_found");

  if (isDataUrl(rawUrl)) {
    return { url: rawUrl, method: method, credentialless: true };
  }

  if (isHttpUrl(rawUrl)) {
    const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
    let isSameOrigin = false;
    try {
      if (base) {
        const uHost = rawUrl.split("/")[2];
        const bHost = base.split("/")[2];
        isSameOrigin = uHost && bHost && uHost.toLowerCase() === bHost.toLowerCase();
      }
    } catch (_) {}

    if (!isSameOrigin) {
      return { url: rawUrl, method: method, credentialless: true };
    }
    const headers = {};
    if (ctx.apiKey) headers.Authorization = "Bearer " + trimmed(ctx.apiKey);
    return { url: rawUrl, method: method, headers: headers, credentialless: false };
  }

  // 相对路径拼 baseUrl
  const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  const fullUrl = base + (rawUrl.startsWith("/") ? rawUrl : "/" + rawUrl);
  const headers = {};
  if (ctx.apiKey) headers.Authorization = "Bearer " + trimmed(ctx.apiKey);
  return { url: fullUrl, method: method, headers: headers, credentialless: false };
}

export const protocols = {
  openai_image: {
    decodeRequest: decodeImageRequest,
    render: function (ctx, task) {
      const entries = extractImageEntries(task && task.data);
      const data = entries.map(function (entry) {
        if (entry.b64_json) return { b64_json: entry.b64_json };
        return { url: entry.url };
      });
      const res = { data: data };
      if (task && task.created_at) {
        res.created = Number(task.created_at);
      }
      return res;
    },
  },
};

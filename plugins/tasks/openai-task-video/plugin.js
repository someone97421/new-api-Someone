// OpenAI-compatible asynchronous video generation task gateway served as the OpenAI Videos API.
//
// Upstream contract:
//   POST {baseUrl}/v1/videos
//   GET  {baseUrl}/v1/videos/{task_id}
//   GET  {baseUrl}/v1/videos/{task_id}/content
//
// Client contract (`POST /v1/videos`, `GET /v1/videos/:task_id`, `GET /v1/videos/:task_id/content`):
//   model            required, declared model
//   prompt           required unless reference image is provided
//   seconds/duration optional, integer 1..3600, default 5
//   size/resolution  optional, inferred to 480p, 720p, or 1080p
//   input_reference  optional reference image (file upload or URL)
//
// Host protocol: openai_video. The host handles client polling and artifact retrieval.

// 站点提交接口路径（站点不同则改这里）
const SUBMIT_PATH = "/v1/videos";
// 站点查询接口路径（必须含且仅含一个 {task_id}，站点不同则改这里）
const QUERY_PATH = "/v1/videos/{task_id}";
// 站点内容获取接口路径（必须含且仅含一个 {task_id}，站点不同则改这里）
const CONTENT_PATH = "/v1/videos/{task_id}/content";

// 站点支持的模型列表（必须在插件里声明渠道实际提供的模型名，插件 API 要求且不支持通配符；安装到具体站点时改成该站点的模型名）
const VENDOR_MODELS = [
  "gpt-video-1",
  "seedream-video-4.0",
  "sora-task-1",
];

// 最大允许时长秒数（沿用 relaycommon.MaxTaskDurationSeconds 的语义上限）
const MAX_DURATION_SECONDS = 3600;

export const meta = {
  apiVersion: 1,
  key: "openai-task-video",
  name: "OpenAI Task Video",
  description: {
    en: "OpenAI-compatible asynchronous video generation task gateway",
    zh: "OpenAI 兼容的异步视频任务网关",
  },
  version: "1.0.0",
  author: { name: "QuantumNous" },
  models: VENDOR_MODELS,
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    resolution: {
      enum: ["480p", "720p", "1080p"],
      enumLabels: {
        "480p": { en: "480p", zh: "480p" },
        "720p": { en: "720p", zh: "720p" },
        "1080p": { en: "1080p", zh: "1080p" },
      },
      description: { en: "Output video resolution", zh: "输出视频分辨率" },
    },
  },
  usageExamples: [
    { label: "720p · 5s", facts: { seconds: 5, resolution: "720p" } },
    { label: "1080p · 5s", facts: { seconds: 5, resolution: "1080p" } },
    { label: "1080p · 10s", facts: { seconds: 10, resolution: "1080p" } },
  ],
  protocols: ["openai_video"],
};

function trimmed(value) {
  return String(value || "").trim();
}

function isHttpUrl(value) {
  return /^https?:\/\//i.test(trimmed(value));
}

function inferResolution(val) {
  const text = trimmed(val).toLowerCase();
  if (!text) return "";
  if (text.includes("1080") || text === "1080p") return "1080p";
  if (text.includes("480") || text === "480p") return "480p";
  if (text.includes("720") || text === "720p") return "720p";
  return "";
}

function decodeVideoRequest(ctx) {
  const model = trimmed(ctx.model);
  if (!model) throw new Error("model is required");

  let req = {};
  let fileRef = null;

  if (ctx.body && ctx.body.kind === "json") {
    req = ctx.body.value;
    if (!req || typeof req !== "object" || Array.isArray(req)) {
      throw new Error("JSON object required");
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
      if (file.field === "input_reference" || file.field === "image") {
        fileRef = {
          __fileRef: file.ref,
          encoding: "dataUrl",
          mimeType: trimmed(file.mimeType) || "image/png",
        };
        break;
      }
    }
  } else {
    throw new Error("JSON or multipart body required");
  }

  const prompt = typeof req.prompt === "string" ? req.prompt : "";
  const hasRefImage = !!fileRef || !!trimmed(req.input_reference) || !!trimmed(req.image) || (Array.isArray(req.images) && req.images.length > 0);
  if (!trimmed(prompt) && !hasRefImage) {
    throw new Error("field prompt is required when no reference image is provided");
  }

  let rawSeconds = req.seconds !== undefined ? req.seconds : req.duration;
  if (rawSeconds !== undefined && rawSeconds !== null && rawSeconds !== "") {
    const sec = Number(rawSeconds);
    if (!Number.isInteger(sec) || sec <= 0 || sec > MAX_DURATION_SECONDS) {
      throw new Error("seconds must be a positive integer between 1 and " + MAX_DURATION_SECONDS);
    }
    rawSeconds = sec;
  }

  const requestBody = Object.assign({}, req, { model: model });
  delete requestBody["async"];

  if (trimmed(prompt)) {
    requestBody.prompt = trimmed(prompt);
  }
  if (rawSeconds !== undefined && rawSeconds !== null && rawSeconds !== "") {
    requestBody.seconds = rawSeconds;
    requestBody.duration = rawSeconds;
  }
  if (fileRef) {
    requestBody.input_reference = fileRef;
    requestBody.images = [fileRef];
  } else if (trimmed(req.input_reference)) {
    requestBody.input_reference = trimmed(req.input_reference);
    if (!requestBody.images) requestBody.images = [trimmed(req.input_reference)];
  } else if (trimmed(req.image)) {
    requestBody.input_reference = trimmed(req.image);
    if (!requestBody.images) requestBody.images = [trimmed(req.image)];
  }

  const action = hasRefImage ? "image_to_video" : "text_to_video";
  return {
    kind: "submit",
    model: model,
    action: action,
    requestBody: requestBody,
  };
}

export function buildSubmitRequest(ctx) {
  const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  const headers = {};
  if (ctx.apiKey) {
    headers.Authorization = "Bearer " + trimmed(ctx.apiKey);
  }

  const files = ctx.files || [];
  if (files.length > 0) {
    const parts = [];
    const values = Object.assign({}, ctx.requestBody || {});
    for (const key of Object.keys(values)) {
      if (values[key] !== undefined && values[key] !== null && typeof values[key] !== "object") {
        parts.push({ name: key, value: String(values[key]) });
      }
    }
    if (values.metadata && typeof values.metadata === "object") {
      parts.push({ name: "metadata", value: JSON.stringify(values.metadata) });
    }
    for (const file of files) {
      parts.push({ name: file.field, fileRef: file.ref, filename: file.filename });
    }
    return {
      url: base + SUBMIT_PATH,
      method: "POST",
      headers: headers,
      bodyType: "multipart",
      parts: parts,
    };
  }

  headers["Content-Type"] = "application/json";
  return {
    url: base + SUBMIT_PATH,
    method: "POST",
    headers: headers,
    body: ctx.requestBody || {},
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = (resp && resp.body) || {};
  const videoUrl = trimmed(
    body.url ||
    (body.data && body.data.url) ||
    (body.output && body.output.video_url)
  );
  if (videoUrl) {
    const publicId = trimmed(ctx.publicTaskId) || "task_immediate";
    return {
      taskId: publicId,
      taskData: body,
      immediate: {
        status: "SUCCESS",
        progress: "100%",
        url: videoUrl,
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

  const videoUrl = trimmed(
    (data.data && data.data.url) ||
    data.url ||
    (output && output.video_url) ||
    data.video_url ||
    (output && output.url)
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
    if (videoUrl) res.url = videoUrl;
    return res;
  }
  if (["failed", "error", "failure", "cancelled", "canceled"].includes(rawStatus)) {
    return { status: "FAILURE", reason: failReason };
  }
  return { status: "UNKNOWN", reason: "unrecognized status: " + (rawStatus || "empty") };
}

export function extractUsage(ctx) {
  const req = ctx.requestBody || {};
  let seconds = Number(req.seconds || req.duration || 5);
  if (!Number.isFinite(seconds) || seconds <= 0) seconds = 5;
  seconds = Math.min(Math.floor(seconds), MAX_DURATION_SECONDS);

  const res = inferResolution(req.resolution) || inferResolution(req.size) || "720p";
  return { seconds: seconds, resolution: res };
}

export function extractUsageOnComplete(task, result, body) {
  const data = body || (task && task.data) || {};
  const usage = data.usage || {};
  const output = data.output || {};

  const rawSeconds = usage.output_video_duration || usage.duration || data.duration || data.seconds || output.duration || output.seconds;
  const facts = {};
  if (rawSeconds !== undefined && rawSeconds !== null && rawSeconds !== "") {
    const sec = Number(rawSeconds);
    if (Number.isFinite(sec) && sec > 0) {
      facts.seconds = Math.min(Math.floor(sec), MAX_DURATION_SECONDS);
    }
  }

  const rawRes = data.resolution || data.size || output.resolution || output.size || usage.resolution;
  const res = inferResolution(rawRes);
  if (res) {
    facts.resolution = res;
  }
  return facts;
}

function extractVideoUrl(data) {
  if (!data || typeof data !== "object") return "";
  return trimmed(
    (data.data && data.data.url) ||
    data.url ||
    (data.output && data.output.video_url) ||
    data.video_url ||
    (data.output && data.output.url)
  );
}

export function listArtifacts(task) {
  if ((task && task.status) !== "SUCCESS") return [];
  const url = extractVideoUrl(task && task.data);
  const taskId = task && (task.upstream_task_id || task.task_id);
  if (url || taskId) {
    return [{ key: "video", type: "video", mimeType: "video/mp4" }];
  }
  return [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  let targetUrl = extractVideoUrl(ctx && ctx.data);
  if (!targetUrl) {
    const tid = trimmed(ctx.upstreamTaskId || ctx.taskId);
    if (tid) {
      targetUrl = CONTENT_PATH.replace("{task_id}", encodeURIComponent(tid));
    }
  }
  if (!targetUrl) throw new Error("artifact_not_found");

  const method = (ctx.clientRequest && ctx.clientRequest.method) || "GET";
  if (isHttpUrl(targetUrl)) {
    return {
      url: targetUrl,
      method: method,
      credentialless: true,
    };
  }

  const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  const fullUrl = base + (targetUrl.startsWith("/") ? targetUrl : "/" + targetUrl);
  const headers = {};
  if (ctx.apiKey) {
    headers.Authorization = "Bearer " + trimmed(ctx.apiKey);
  }
  return {
    url: fullUrl,
    method: method,
    headers: headers,
    credentialless: false,
  };
}

export const protocols = {
  openai_video: {
    decodeRequest: decodeVideoRequest,
    render: function (ctx, task) {
      const statuses = {
        NOT_START: "queued",
        SUBMITTED: "queued",
        QUEUED: "queued",
        IN_PROGRESS: "in_progress",
        SUCCESS: "completed",
        FAILURE: "failed",
      };
      const rawStatus = String((task && task.status) || "").toUpperCase();
      const status = statuses[rawStatus] || "unknown";
      const props = (task && task.properties) || {};
      const model = trimmed(props.origin_model_name || (ctx && ctx.model) || "");
      const progressVal = Number(String((task && task.progress) || "0").replace("%", ""));
      const progress = Number.isFinite(progressVal) ? progressVal : 0;
      const res = {
        id: (task && task.task_id) || "",
        object: "video",
        model: model,
        status: status,
        progress: progress,
        created_at: Number((task && task.created_at) || 0),
      };
      const completedAt = Number((task && (task.finished_at || task.updated_at)) || 0);
      if (completedAt > 0) {
        res.completed_at = completedAt;
      }
      if (status === "failed") {
        res.error = {
          code: "video_generation_failed",
          message: (task && task.fail_reason) || "The video generation task failed.",
        };
      }
      return res;
    },
  },
};

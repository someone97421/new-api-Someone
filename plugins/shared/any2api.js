// Source(key) 将此驱动与供应商声明合成为可独立上传的单文件插件。
function createAny2API(config) {
  const canonicalModels = config.canonicalModels || {};
  const imageModels = [...new Set(config.images.concat(Object.keys(config.imageAliases || {})).map((model) => canonicalModels[model] || model))];
  const videoModels = [...new Set(config.videos.concat(Object.keys(config.videoAliases || {})).map((model) => canonicalModels[model] || model))];
  const imagePrice = {
    en: "Image generation unit price",
    zh: "图片生成单价",
    "zh-TW": "圖片生成單價",
    fr: "Prix unitaire de génération d’image",
    ru: "Цена генерации изображения",
    ja: "画像生成単価",
    vi: "Đơn giá tạo ảnh",
  };
  const videoPrice = {
    en: "Video generation unit price",
    zh: "视频生成单价",
    "zh-TW": "影片生成單價",
    fr: "Prix unitaire de génération de vidéo",
    ru: "Цена генерации видео",
    ja: "動画生成単価",
    vi: "Đơn giá tạo video",
  };
  const taskBase = "/" + config.key + "/api/v1/tasks";
  const meta = {
    apiVersion: 1,
    key: config.key,
    name: config.name,
    icon: "text:" + config.icon,
    version: "1.0.0",
    author: { name: "new-api" },
    description: { en: config.name + " image and video task API", zh: "通过 Any2API " + config.name + " 后端生成图片和视频" },
    models: imageModels.concat(videoModels),
    fetchMode: "per_task",
    auth: { type: "api_key" },
    upstreams: ["vendor", "new_api"],
    routes: [
      { method: "POST", path: taskBase, type: "submit", decode: "createTask", render: "taskView" },
      { method: "POST", path: taskBase + "/json", type: "submit", decode: "createJSONTask", render: "taskView" },
      { method: "GET", path: taskBase + "/:task_id", type: "query", render: "taskView" },
      { method: "GET", path: taskBase + "/:task_id/content", type: "content" },
      { method: "HEAD", path: taskBase + "/:task_id/content", type: "content" },
    ],
    protocols: [
      { name: "openai_image", models: imageModels },
      { name: "openai_video", models: videoModels },
    ],
    usageSchema: { image_count: { type: "number", unit: "count", description: imagePrice } },
    // 视频按成品数量计费；cost.power/credits 不是可核验的最终消耗。
    usageProfiles: [{ models: videoModels, schema: { video_count: { type: "number", unit: "count", description: videoPrice } } }],
  };

  function object(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value);
  }
  function text(value) {
    return typeof value === "string" ? value.trim() : "";
  }
  function remote(value) {
    return /^https?:\/\/[^\s]+$/i.test(text(value));
  }
  function local(value) {
    return /^\/files\/outputs\/[^?#]+$/.test(text(value)) && !value.includes("..");
  }
  function unwrap(body) {
    if (!object(body) || body.ok !== true || !object(body.data)) throw new Error("上游未返回有效任务 envelope");
    return body.data;
  }
  function video(ctx) {
    return ctx.action ? ctx.action === "generate_video" : videoModels.includes(ctx.upstreamModel || ctx.model);
  }
  function modelName(ctx) {
    const model = ctx.upstreamModel || ctx.model;
    return (config.imageAliases || {})[model] || (config.videoAliases || {})[model] || model;
  }
  function endpoint(ctx, path) {
    return ctx.baseUrl.replace(/\/+$/, "") + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/" + config.key : "") + path;
  }
  function headers(ctx) {
    return ctx.upstream && ctx.upstream.kind === "new_api" ? { Authorization: "Bearer " + ctx.apiKey } : { "X-API-Key": ctx.apiKey };
  }
  function quantity(value, key, maximum) {
    if (!Number.isInteger(value) || value < 1 || value > maximum) throw new Error(key + " 必须是 1 到 " + maximum + " 之间的整数");
    return value;
  }
  function validate(req) {
    if (!object(req.extra)) throw new Error("extra 必须是 JSON 对象");
    for (const values of [req, req.extra]) {
      for (const key of ["count", "generate_count", "n", "numberOfGenerations", "numberOfImages", "numberOfVideos", "batch_size", "task_count"]) {
        if (values[key] !== undefined && values[key] !== null) quantity(values[key], key, 128);
      }
      for (const key of ["duration", "seconds"]) {
        if (values[key] !== undefined && values[key] !== null && (!Number.isInteger(values[key]) || values[key] < 1 || values[key] > 3600))
          throw new Error(key + " 必须是 1 到 3600 之间的整数");
      }
      for (const key of ["draft_task_id", "source_task_id", "draftTaskId"]) {
        if (values[key] !== undefined) throw new Error("暂不支持样片转正片任务");
      }
      if (values.draft !== undefined && values.draft !== false) throw new Error("暂不支持样片生成");
    }
    if (req["async"] !== undefined && typeof req["async"] !== "boolean") throw new Error("async 必须是布尔值");
    if (req.stream !== undefined && req.stream !== false) throw new Error("此接口不支持流式生成");
    if (req.mask !== undefined) throw new Error("此供应商任务接口不支持 mask");
    if (text(req.resolution).toLowerCase() === "draft") throw new Error("暂不支持样片生成");
  }
  function requestBody(ctx) {
    const body = ctx.body || {};
    const uploads = [];
    let req;
    if (body.kind === "json") {
      if (!object(body.value)) throw new Error("请求体必须是 JSON 对象");
      req = utils.json.clone(body.value);
    } else if (body.kind === "multipart") {
      req = {};
      for (const key of Object.keys(body.fields || {})) {
        const values = body.fields[key];
        if (values.length !== 1) throw new Error(key + " 只能提供一次");
        const value = values[0];
        if (["count", "generate_count", "n", "duration", "seconds", "seed"].includes(key)) req[key] = Number(value);
        else if (["extra", "extra_json", "metadata", "async", "stream", "draft"].includes(key)) req[key] = JSON.parse(value);
        else req[key] = value;
      }
      for (const file of body.files || []) {
        if (!["references", "image", "image[]", "input_reference"].includes(file.field)) throw new Error("不支持文件字段 " + file.field);
        uploads.push({ ref: file.ref, filename: file.filename });
      }
    } else throw new Error("需要 JSON 或 multipart 请求体");
    return { req: req, uploads: uploads, multipart: body.kind === "multipart" };
  }
  function decode(ctx, media, jsonOnly) {
    if (jsonOnly && ctx.body.kind !== "json") throw new Error("此入口需要 JSON 请求体");
    const input = requestBody(ctx),
      req = input.req;
    const nativeModel =
      config.workflow && text(req.model) && req.model !== "configured-in-space" ? text(req.model) : text(req.capability_id) || text(req.model);
    const clientModel = text(ctx.model) || canonicalModels[nativeModel] || nativeModel;
    if (!clientModel) throw new Error("必须提供 model 或 capability_id");
    if (!media) {
      if (!imageModels.includes(clientModel) && !videoModels.includes(clientModel)) throw new Error("模型未在插件中声明");
      media = videoModels.includes(clientModel) ? "video" : "image";
    }
    if (req.extra === undefined) req.extra = req.extra_json === undefined ? {} : req.extra_json;
    delete req.extra_json;
    if (!object(req.extra)) throw new Error("extra 必须是 JSON 对象");
    if (media && ctx.model) {
      const target = config.countField;
      if (req.n !== undefined) {
        if (!target && !config.workflow && req.n !== 1) throw new Error("此供应商不支持请求生成数量");
        if (config.workflow) {
          if (req.extra.numberOfGenerations !== undefined && req.extra.numberOfGenerations !== req.n) throw new Error("n 与 numberOfGenerations 必须一致");
          req.extra.numberOfGenerations = req.n;
        }
        if (target) {
          if (req[target] !== undefined && req[target] !== req.n) throw new Error("n 与 " + target + " 必须一致");
          req[target] = req.n;
        }
      }
      if (req.seconds !== undefined) {
        const seconds = typeof req.seconds === "string" && /^\d+$/.test(req.seconds) ? Number(req.seconds) : req.seconds;
        if (req.duration !== undefined && req.duration !== seconds) throw new Error("seconds 与 duration 必须一致");
        req.duration = seconds;
      }
      if (req.aspect_ratio !== undefined) {
        if (req.ratio !== undefined && req.ratio !== req.aspect_ratio) throw new Error("ratio 与 aspect_ratio 必须一致");
        req.ratio = req.aspect_ratio;
      }
      if (req.size !== undefined && req.size !== "auto") {
        const match = /^(\d+)x(\d+)$/.exec(text(req.size));
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
        const ratio = width / a + ":" + height / a;
        const resolution =
          media === "image"
            ? { 1024: "1K", 2048: "2K", 4096: "4K" }[Math.max(width, height)]
            : { 480: "480p", 720: "720p", 1080: "1080p", 2160: "4k" }[Math.min(width, height)];
        if (!resolution) throw new Error("此 size 无法转换，请使用 resolution 和 ratio");
        if (req.ratio !== undefined && req.ratio !== ratio) throw new Error("size 与 ratio 必须一致");
        if (req.resolution !== undefined && req.resolution !== resolution) throw new Error("size 与 resolution 必须一致");
        req.ratio = ratio;
        req.resolution = resolution;
      }
      // Any2API 的 A 系任务端点只通过 references 文件接收素材，没有 URL 下载字段。
      for (const key of ["image", "image[]", "images", "image_url", "image_urls", "input_reference"]) {
        if (req[key] !== undefined) throw new Error("参考素材请通过 multipart 文件上传，当前供应商任务接口不支持 URL 素材");
      }
      for (const key of ["n", "seconds", "aspect_ratio", "size", "stream", "response_format"]) delete req[key];
      delete req.capability_id;
      delete req.model;
    }
    if (ctx.operation === "edit" && !input.uploads.length) throw new Error("图片编辑必须提供 multipart 参考文件");
    if (
      input.multipart &&
      !config.extraField &&
      (Object.keys(req.extra).length ||
        Object.keys(req).some(function (key) {
          return !config.fields.includes(key) && key !== "extra";
        }))
    )
      throw new Error("当前供应商 multipart 不支持 extra；请使用 JSON 或移除扩展参数");
    validate(req);
    return {
      kind: "submit",
      model: clientModel,
      action: "generate_" + media,
      requestBody: { payload: req, uploads: input.uploads, multipart: input.multipart },
    };
  }
  function payload(ctx) {
    const req = utils.json.clone(ctx.requestBody.payload);
    const selected = modelName(ctx),
      isVideo = video(ctx);
    if (!(isVideo ? config.videos : config.images).includes(selected)) throw new Error("上游模型未声明，请配置渠道模型映射");
    req.capability_id = config.workflow ? "magnific-workflow-" + (isVideo ? "video" : "image") : selected;
    req.model = config.workflow && selected.startsWith("magnific-workflow-") ? "configured-in-space" : selected;
    if (req["async"] === undefined) req["async"] = true;
    const defaults = (config.defaults || {})[selected] || {};
    for (const key of Object.keys(defaults)) if (req[key] === undefined) req[key] = defaults[key];
    if (req.ratio === undefined) req.ratio = isVideo ? "16:9" : "1:1";
    if (req.resolution === undefined && !req.quality) req.resolution = isVideo ? "720p" : config.imageResolution || "1K";
    if (config.countField && req[config.countField] === undefined)
      req[config.countField] = config.defaultCounts && config.defaultCounts[selected] !== undefined ? config.defaultCounts[selected] : 1;
    // 未知字段放进供应商的扩展参数，保留嵌套对象与显式零值。
    for (const key of Object.keys(req)) {
      if (config.fields.includes(key) || key === "extra") continue;
      if (req.extra[key] !== undefined) throw new Error("扩展参数重复: " + key);
      req.extra[key] = req[key];
      delete req[key];
    }
    validate(req);
    if (config.countField) quantity(req[config.countField], config.countField, config.maxCount || 128);
    // Magnific 的数量由 extra 覆盖节点默认值，预扣必须读最终发送的值。
    const counts = ["numberOfGenerations", "numberOfImages", "numberOfVideos"].filter(function (key) {
      return req.extra[key] !== undefined;
    });
    if (config.workflow && counts.length > 1) throw new Error("只能提供一个生成数量字段");
    return req;
  }
  function buildSubmitRequest(ctx) {
    const req = payload(ctx);
    const uploads = ctx.requestBody.uploads || [];
    const multipart = uploads.length > 0 || ctx.requestBody.multipart;
    if (!multipart) return { url: endpoint(ctx, "/api/v1/tasks/json"), method: "POST", headers: headers(ctx), body: req };
    if (!config.extraField && Object.keys(req.extra).length) throw new Error("当前供应商 multipart 不支持 extra；请使用 JSON 或移除扩展参数");
    const parts = [];
    for (const key of Object.keys(req)) {
      if (key === "extra") {
        if (config.extraField) parts.push({ name: config.extraField, value: JSON.stringify(req.extra) });
        continue;
      }
      if (req[key] === undefined || req[key] === null) continue;
      parts.push({ name: key, value: typeof req[key] === "object" ? JSON.stringify(req[key]) : String(req[key]) });
    }
    for (const file of uploads) parts.push({ name: "references", fileRef: file.ref, filename: file.filename });
    return { url: endpoint(ctx, "/api/v1/tasks"), method: "POST", headers: headers(ctx), bodyType: "multipart", parts: parts };
  }
  function outputs(body) {
    const data = unwrap(body),
      result = [];
    for (let index = 0; index < (data.images || []).length; index++) {
      const item = data.images[index];
      if (!object(item) || (!remote(item.url) && !local(item.local_path) && !local(item.url))) continue;
      const type = item.media_type === "video" || text(item.media_type).startsWith("video/") ? "video" : "image";
      result.push({ key: type + "_" + index, type: type, item: item });
    }
    return result;
  }
  function parseTaskResult(ctx, body) {
    const data = unwrap(body);
    let status =
      {
        created: "QUEUED",
        validating: "IN_PROGRESS",
        uploading: "IN_PROGRESS",
        preparing: "IN_PROGRESS",
        submitted: "IN_PROGRESS",
        polling: "IN_PROGRESS",
        downloading: "IN_PROGRESS",
        success: "SUCCESS",
        failed: "FAILURE",
        timeout: "FAILURE",
        cancelled: "FAILURE",
      }[data.status] || "UNKNOWN";
    let reason = text(data.error);
    const items = outputs(body).filter(function (item) {
      return item.type === (video(ctx) ? "video" : "image");
    });
    if (status === "SUCCESS" && !items.length) {
      status = "FAILURE";
      reason = "上游未返回可读取的成品";
    }
    return {
      taskId: ctx.taskId || data.id,
      status: status,
      progress: status === "SUCCESS" ? "100%" : "0%",
      reason: reason,
      url: items.length && remote(items[0].item.url) ? items[0].item.url : "",
    };
  }
  function parseSubmitResponse(ctx, response) {
    const data = unwrap(response.body);
    if (!text(data.id)) throw new Error("上游响应缺少任务 id");
    const result = parseTaskResult(ctx, response.body);
    return { taskId: data.id, taskData: response.body, immediate: ["SUCCESS", "FAILURE"].includes(result.status) ? result : undefined };
  }
  function extractUsage(ctx) {
    const req = payload(ctx);
    let count = config.countField ? req[config.countField] : 1;
    if (config.workflow) {
      for (const key of ["numberOfGenerations", "numberOfImages", "numberOfVideos"]) if (req.extra[key] !== undefined) count = req.extra[key];
    }
    return video(ctx) ? { video_count: count } : { image_count: count };
  }
  function extractUsageOnComplete(ctx, result, body) {
    if (result.status !== "SUCCESS") return {};
    const type = video(ctx) ? "video" : "image";
    const count = outputs(body).filter(function (item) {
      return item.type === type;
    }).length;
    if (count < 1 || count > 128) return {};
    return video(ctx) ? { video_count: count } : { image_count: count };
  }
  function listArtifacts(task) {
    if (task.status !== "SUCCESS") return [];
    return outputs(task.data).map(function (output) {
      return { key: output.key, type: output.type };
    });
  }
  function buildContentRequest(ctx) {
    const output = outputs(ctx.data).find(function (item) {
      return item.key === ctx.artifactKey;
    });
    if (!output) throw new Error("artifact_not_found");
    if (!local(output.item.local_path) && remote(output.item.url)) return { url: output.item.url, method: ctx.clientRequest.method, credentialless: true };
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
      url: endpoint(ctx, local(output.item.local_path) ? output.item.local_path : output.item.url),
      method: ctx.clientRequest.method,
      headers: headers(ctx),
    };
  }
  function taskView(ctx, task) {
    const body = utils.json.clone(task.data || { ok: true, data: {} });
    const data = unwrap(body);
    data.id = task.task_id;
    const status = { NOT_START: "created", SUBMITTED: "submitted", QUEUED: "created", IN_PROGRESS: "polling", SUCCESS: "success", FAILURE: "failed" }[
      task.status
    ];
    if (task.status === "FAILURE" && !["failed", "timeout", "cancelled"].includes(data.status)) data.status = status;
    else if (!["SUCCESS", "FAILURE"].includes(task.status) && !data.status) data.status = status || "created";
    if (task.fail_reason) data.error = task.fail_reason;
    data.raw = {};
    data.events = [];
    data.remote_thread_id = null;
    data.remote_run_id = null;
    if (object(data.params)) delete data.params.pending_references;
    return body;
  }
  return {
    meta: meta,
    buildSubmitRequest: buildSubmitRequest,
    parseSubmitResponse: parseSubmitResponse,
    buildQueryRequest: function (ctx) {
      return { url: endpoint(ctx, "/api/v1/tasks/" + encodeURIComponent(ctx.taskId)), method: "GET", headers: headers(ctx) };
    },
    parseTaskResult: parseTaskResult,
    extractUsage: extractUsage,
    extractUsageOnComplete: extractUsageOnComplete,
    listArtifacts: listArtifacts,
    buildContentRequest: buildContentRequest,
    native: {
      createTask: function (ctx) {
        return decode(ctx, "", false);
      },
      createJSONTask: function (ctx) {
        return decode(ctx, "", true);
      },
      taskView: taskView,
    },
    protocols: {
      openai_image: {
        decodeRequest: function (ctx) {
          return decode(ctx, "image", false);
        },
        render: function (ctx, task) {
          return {
            data: outputs(task.data)
              .filter(function (item) {
                return item.type === "image";
              })
              .map(function (output) {
                const item = { artifact_key: output.key };
                if (remote(output.item.url)) item.url = output.item.url;
                return item;
              }),
          };
        },
      },
      openai_video: {
        decodeRequest: function (ctx) {
          return decode(ctx, "video", false);
        },
        render: function (ctx, task) {
          const output = outputs(task.data).find(function (item) {
            return item.type === "video";
          });
          return output && remote(output.item.url) ? { url: output.item.url } : {};
        },
      },
    },
  };
}

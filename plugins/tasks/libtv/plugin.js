// 模型与默认值来自 Any2API LibTV capability registry。
const plugin = createAny2API({
  key: "libtv",
  name: "LibTV",
  icon: "Li",
  extraField: "extra",
  countField: "count",
  maxCount: 4,
  fields: ["capability_id", "prompt", "model", "resolution", "quality", "ratio", "count", "duration", "async"],
  images: [
    "seedream-4",
    "seedream-4.5",
    "jimeng-4.6",
    "doubao-seedream-5-0-pro",
    "nebula-2-flash",
    "nebula-ultra",
    "lib-image-2",
    "lib-image-2.5-s",
    "lib-image-2.5-f",
    "qwen-image-3",
    "mj-v8.2",
    "mj-v8.1",
    "mj-v7",
    "mj-niji7",
  ],
  videos: [
    "star-video2.5",
    "star-video2",
    "star-video2-fast",
    "star-video2-mini",
    "MiniMax-Hailuo-H3",
    "wanx3.0",
    "happy-horse-1.1",
    "happy-horse-1",
    "kling-v4",
    "kling-v3-omni",
    "kling-v3-turbo",
    "kling-video-o3",
  ],
  imageAliases: {
    "gpt-image-2": "lib-image-2",
    "gpt-image-2.5-flare": "lib-image-2.5-f",
    "gpt-image-2.5-sunburst": "lib-image-2.5-s",
    "nano-banana-2": "nebula-2-flash",
    "nano-banana-pro": "nebula-ultra",
    "seedream-5.0-pro": "doubao-seedream-5-0-pro",
  },
  videoAliases: {
    "doubao-seedance-2-0-260128": "star-video2",
    "doubao-seedance-2-0-fast-260128": "star-video2-fast",
    "seedance-2.5": "star-video2.5",
    "kling-4.0": "kling-v4",
  },
  defaultCounts: { "mj-v8.2": 4, "mj-v8.1": 4, "mj-v7": 4, "mj-niji7": 4 },
  imageResolution: "2K",
  defaults: {
    "mj-v8.2": { resolution: "auto" },
    "mj-v8.1": { resolution: "auto" },
    "mj-v7": { resolution: "auto" },
    "mj-niji7": { resolution: "auto" },
    "MiniMax-Hailuo-H3": { resolution: "2K" },
    "wanx3.0": { resolution: "1080P", ratio: "auto" },
    "happy-horse-1.1": { resolution: "720P" },
    "happy-horse-1": { resolution: "720P" },
    "kling-v4": { resolution: "1080p" },
    "kling-v3-omni": { quality: "low" },
    "kling-video-o3": { quality: "low" },
  },
});
export const meta = plugin.meta;
export const native = plugin.native;
export const protocols = plugin.protocols;
export const buildSubmitRequest = plugin.buildSubmitRequest;
export const parseSubmitResponse = plugin.parseSubmitResponse;
export const buildQueryRequest = plugin.buildQueryRequest;
export const parseTaskResult = plugin.parseTaskResult;
export const extractUsage = plugin.extractUsage;
export const extractUsageOnComplete = plugin.extractUsageOnComplete;
export const listArtifacts = plugin.listArtifacts;
export const buildContentRequest = plugin.buildContentRequest;

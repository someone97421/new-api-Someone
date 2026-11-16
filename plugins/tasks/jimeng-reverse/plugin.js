// 模型与数量默认值来自 Any2API 即梦 capability registry。
const plugin = createAny2API({
  key: "jimeng-reverse",
  name: "Jimeng Reverse",
  icon: "即",
  extraField: "",
  countField: "generate_count",
  maxCount: 8,
  fields: ["capability_id", "prompt", "model", "resolution", "ratio", "duration", "generate_count", "async"],
  images: ["image_5.0_pro", "image_5.0_lite", "image_4.7", "image_4.6", "image_4.5", "image_4.1", "image_4.0"],
  videos: ["seedance_2.5_pro", "seedance_2.0_mini", "seedance_2.0_fast", "seedance_2.0", "seedance_2.0_fast_vip", "seedance_2.0_vip"],
  imageAliases: { "seedream-5.0-pro": "image_5.0_pro", "seedream-4.5": "image_4.5" },
  videoAliases: { "doubao-seedance-2-0-260128": "seedance_2.0", "doubao-seedance-2-0-fast-260128": "seedance_2.0_fast", "seedance-2.5": "seedance_2.5_pro" },
  defaultCounts: { "image_5.0_pro": 2, "image_5.0_lite": 4, "image_4.7": 4, "image_4.6": 4, "image_4.5": 4, "image_4.0": 4 },
  imageResolution: "2K",
  defaults: { "image_5.0_pro": { resolution: "1K" }, "seedance_2.5_pro": { resolution: "480p" } },
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

// 模型来自 Any2API XYQ capability registry。
const plugin = createAny2API({
  key: "xyq",
  name: "小云雀",
  icon: "XY",
  extraField: "",
  countField: "",
  canonicalModels: { "Seedance_2.0_mini": "seedance_2.0_mini" },
  fields: ["capability_id", "prompt", "model", "resolution", "ratio", "duration", "seed", "async"],
  images: ["seedream_5.0_pro", "seedream_5.0", "seedream_4.3", "nova2", "seedream_4.5", "seedream_4.1", "seedream_4", "nano_banana_pro_1", "gpt_image_2"],
  videos: [
    "Seedance_2.5",
    "Seedance_2.0_mini_lite",
    "Seedance_2.0_mini",
    "seedance2.0_fast_vision",
    "seedance2.0_vision",
    "seedance2.0_fast_direct",
    "seedance2.0_direct",
  ],
  imageAliases: { "gpt-image-2": "gpt_image_2", "nano-banana-2": "nova2", "nano-banana-pro": "nano_banana_pro_1", "seedream-5.0-pro": "seedream_5.0_pro" },
  videoAliases: {
    "seedance_2.0_mini": "Seedance_2.0_mini",
    "doubao-seedance-2-0-260128": "seedance2.0_direct",
    "doubao-seedance-2-0-fast-260128": "seedance2.0_fast_direct",
    "seedance-2.5": "Seedance_2.5",
  },
  imageResolution: "2K",
  defaults: { "seedream_5.0_pro": { resolution: "1K" } },
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

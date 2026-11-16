// 此插件驱动 Any2API Magnific Space 工作流，官方 /v1/ai 接口由 magnific 插件提供。
const plugin = createAny2API({
  key: "magnific-reverse",
  name: "Magnific Reverse",
  icon: "MR",
  workflow: true,
  extraField: "extra_json",
  countField: "",
  fields: ["capability_id", "prompt", "model", "resolution", "ratio", "duration", "async"],
  images: [
    "magnific-workflow-image",
    "gpt-2.5",
    "gpt-2",
    "imagen-nano-banana-2-1",
    "imagen-nano-banana-2-flash",
    "imagen-nano-banana-2",
    "flux-3-image",
    "ideogram-4-5",
    "flux-2-max",
    "seedream-5-pro",
    "luma-uni-1",
  ],
  videos: [
    "magnific-workflow-video",
    "wan-3-0",
    "wan-3-0-prime",
    "minimax-video-3_0",
    "minimax-video-3_0-max",
    "bytedance-seedance-pro-2.5",
    "gemini-omni-1_1",
    "bytedance-seedance-pro-2.0",
    "bytedance-seedance-fast-2.0",
    "bytedance-seedance-mini-2.0",
    "kling-40",
    "kling-40-omni",
    "kling-30",
    "kling-30-turbo",
    "happy-horse-1-1",
  ],
  imageAliases: {
    "gpt-image-2": "gpt-2",
    "gpt-image-2.5-sunburst": "gpt-2.5",
    "nano-banana-2.1": "imagen-nano-banana-2-1",
    "nano-banana-2": "imagen-nano-banana-2-flash",
    "nano-banana-pro": "imagen-nano-banana-2",
    "seedream-5.0-pro": "seedream-5-pro",
  },
  videoAliases: {
    "doubao-seedance-2-0-260128": "bytedance-seedance-pro-2.0",
    "doubao-seedance-2-0-fast-260128": "bytedance-seedance-fast-2.0",
    "seedance-2.5": "bytedance-seedance-pro-2.5",
    "kling-4.0": "kling-40",
  },
  imageResolution: "2K",
  defaults: {
    "magnific-workflow-image": { resolution: "1K" },
    "flux-2-max": { resolution: "1K" },
    "minimax-video-3_0": { resolution: "768p", ratio: "21:9" },
    "minimax-video-3_0-max": { resolution: "768p", ratio: "21:9" },
    "bytedance-seedance-pro-2.5": { ratio: "21:9" },
    "luma-uni-1": { ratio: "16:9" },
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

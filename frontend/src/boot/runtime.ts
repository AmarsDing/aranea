import { defineBoot } from "#q-app/wrappers";
import { loadRuntimeConfig } from "../config/runtime";
import { syncApiBaseURL } from "../api/client";

export default defineBoot(async () => {
  await loadRuntimeConfig();
  syncApiBaseURL();
});

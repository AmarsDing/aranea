import { createPinia } from "pinia";
import { defineBoot } from "#q-app/wrappers";

export default defineBoot(({ app }) => {
  app.use(createPinia());
});

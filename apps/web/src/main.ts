import ElementPlus from "element-plus";
import "element-plus/dist/index.css";
import { createPinia } from "pinia";
import { createApp } from "vue";

import App from "./App.vue";
import { createConsoleRouter } from "./router";
import "./style.css";

createApp(App)
  .use(createPinia())
  .use(createConsoleRouter())
  .use(ElementPlus)
  .mount("#app");

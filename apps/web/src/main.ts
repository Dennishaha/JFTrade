// FontAwesome 只用到 solid / regular 两族（见 fontAwesomeIcons.ts 与模板），
// 不引入 all.min.css，避免 brands / v4compatibility 字体进产物。
import "@fortawesome/fontawesome-free/css/fontawesome.min.css";
import "@fortawesome/fontawesome-free/css/solid.min.css";
import "@fortawesome/fontawesome-free/css/regular.min.css";
import { VueQueryPlugin } from "@tanstack/vue-query";
import "splitpanes/dist/splitpanes.css";
import { createApp } from "vue";
import { createVuetify } from "vuetify";
// 全局样式（reset / 工具类 / 调色板）仍需全量：模板大量使用 vuetify 工具类
// 与 color="teal" 等调色板颜色。组件级样式由 vite-plugin-vuetify 按需引入。
import "vuetify/styles";

import App from "./App.vue";
import { fontAwesomeIcons } from "./fontAwesomeIcons";
import { queryClient } from "@/composables/settings/serverState";
import { createConsoleRouter } from "./router";
import { vuetifyTheme } from "./vuetifyTheme";
import "./styles/tokens.css";
import "./styles/adk-tokens.css";
import "./styles/components.css";
import "./styles/product-controls.css";
import "./styles/adk.css";
import "./style.css";

const vuetify = createVuetify({
  icons: fontAwesomeIcons,
  theme: vuetifyTheme,
});

const app = createApp(App);

app.use(VueQueryPlugin, { queryClient });
app.use(createConsoleRouter());
app.use(vuetify);
app.mount("#app");

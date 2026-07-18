import "@fortawesome/fontawesome-free/css/all.min.css";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { createPinia } from "pinia";
import "splitpanes/dist/splitpanes.css";
import { createApp, type Plugin } from "vue";
import * as components from "vuetify/components";
import * as directives from "vuetify/directives";
import { createVuetify } from "vuetify";
import "vuetify/styles";

import App from "./App.vue";
import { fontAwesomeIcons } from "./fontAwesomeIcons";
import { queryClient } from "./composables/serverState";
import { createConsoleRouter } from "./router";
import "./styles/adk-tokens.css";
import "./styles/product-controls.css";
import "./style.css";

const vuetify = createVuetify({
  components,
  directives,
  icons: fontAwesomeIcons,
  theme: {
    defaultTheme: "dark",
    themes: {
      light: {
        dark: false,
        colors: {
          background: "#f4f7fb",
          surface: "#ffffff",
        },
      },
      dark: {
        dark: true,
        colors: {
          background: "#0a0a0a",
          surface: "#141414",
        },
      },
    },
  },
});

const app = createApp(App);

app.use(createPinia() as unknown as Plugin);
app.use(VueQueryPlugin, { queryClient });
app.use(createConsoleRouter() as unknown as Plugin);
app.use(vuetify as unknown as Plugin);
app.mount("#app");

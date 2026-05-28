import "@fortawesome/fontawesome-free/css/all.min.css";
import { createPinia } from "pinia";
import { createApp, type Plugin } from "vue";
import * as components from "vuetify/components";
import * as directives from "vuetify/directives";
import { createVuetify } from "vuetify";
import "vuetify/styles";

import App from "./App.vue";
import { fontAwesomeIcons } from "./fontAwesomeIcons";
import { createConsoleRouter } from "./router";
import "./style.css";

const vuetify = createVuetify({
  components,
  directives,
  icons: fontAwesomeIcons,
});

const app = createApp(App);

app.use(createPinia() as unknown as Plugin);
app.use(createConsoleRouter() as unknown as Plugin);
app.use(vuetify as unknown as Plugin);
app.mount("#app");

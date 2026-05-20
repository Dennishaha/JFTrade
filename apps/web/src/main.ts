import "@fortawesome/fontawesome-free/css/all.min.css";
import { createPinia } from "pinia";
import { createApp } from "vue";
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

createApp(App)
  .use(createPinia())
  .use(createConsoleRouter())
  .use(vuetify)
  .mount("#app");

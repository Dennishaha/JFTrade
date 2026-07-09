package servercore

import (
	"net/http"

	"github.com/gin-gonic/gin"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	swaggerdocs "github.com/jftrade/jftrade-main/docs/swagger"
)

var swaggerUIHandler = gin.WrapF(httpSwagger.Handler(
	httpSwagger.DefaultModelsExpandDepth(1),
	httpSwagger.DocExpansion("list"),
	httpSwagger.PersistAuthorization(true),
	httpSwagger.URL("/swagger/doc.json"),
))

func init() {
	swaggerdocs.SwaggerInfo.BasePath = "/"
}

func (s *Server) handleSwaggerRoot(c *gin.Context) {
	http.Redirect(c.Writer, c.Request, "/swagger/index.html", http.StatusTemporaryRedirect)
}

func (s *Server) handleSwaggerUI(c *gin.Context) {
	if c.Request.URL.Path == "/swagger/" {
		http.Redirect(c.Writer, c.Request, "/swagger/index.html", http.StatusTemporaryRedirect)
		return
	}
	if c.Request.URL.Path == "/swagger/swagger-initializer.js" {
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(`window.onload = function() {
  window.ui = SwaggerUIBundle({
    url: "/swagger/doc.json",
    dom_id: "#swagger-ui",
    deepLinking: true,
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    plugins: [
      SwaggerUIBundle.plugins.DownloadUrl
    ],
    layout: "StandaloneLayout"
  });
};
`))
		return
	}
	swaggerUIHandler(c)
}

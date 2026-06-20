package servercore

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	swaggerdocs "github.com/jftrade/jftrade-main/docs/swagger"
)

var swaggerUIHandler = ginSwagger.WrapHandler(
	swaggerfiles.Handler,
	ginSwagger.DefaultModelsExpandDepth(1),
	ginSwagger.DocExpansion("list"),
	ginSwagger.PersistAuthorization(true),
	ginSwagger.URL("/swagger/doc.json"),
)

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
	swaggerUIHandler(c)
}

package servercore

import (
	"net/http"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
)

type sseWriter = httpserver.SSEWriter

func prepareSSEWriter(w http.ResponseWriter) (sseWriter, bool) {
	return httpserver.PrepareSSEWriter(w)
}

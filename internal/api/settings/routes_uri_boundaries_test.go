package settings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResourceHandlersRejectMissingURIParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "broker integration", handler: handleSaveBrokerIntegration(nil)},
		{name: "account update", handler: handleUpdateManagedBrokerAccount(nil)},
		{name: "account delete", handler: handleDeleteManagedBrokerAccount(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/", nil)

			test.handler(context)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

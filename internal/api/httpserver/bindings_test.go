package httpserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindURIRejectsMalformedEscapeInRequestURI(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method:     http.MethodGet,
		RequestURI: "/items/bad%ZZ",
		URL:        &url.URL{Path: "/items/bad%ZZ"},
	}
	c.Params = gin.Params{{Key: "id", Value: "bad%ZZ"}}

	var uri struct {
		ID string `uri:"id" binding:"required"`
	}

	if err := BindURI(c, &uri); err == nil {
		t.Fatal("BindURI error = nil, want malformed URL escape error")
	}
}

func TestBindURIAllowsEscapedLiteralPercent(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/items/:id", func(c *gin.Context) {
		var uri struct {
			ID string `uri:"id" binding:"required"`
		}
		if err := BindURI(c, &uri); err != nil {
			t.Fatalf("BindURI: %v", err)
		}
		if uri.ID != "value%" {
			t.Fatalf("uri.ID = %q, want %q", uri.ID, "value%")
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/items/value%25", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

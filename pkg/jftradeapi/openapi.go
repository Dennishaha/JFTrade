package jftradeapi

import (
	"encoding/json"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

var swaggerUIHandler = httpSwagger.Handler(
	httpSwagger.URL("/openapi.json"),
	httpSwagger.DeepLinking(true),
	httpSwagger.DocExpansion("list"),
	httpSwagger.DefaultModelsExpandDepth(httpSwagger.ShowModel),
	httpSwagger.PersistAuthorization(true),
	httpSwagger.UIConfig(map[string]string{
		"displayRequestDuration": "true",
	}),
)

func (s *Server) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/swagger":
		http.Redirect(w, r, "/swagger/", http.StatusTemporaryRedirect)
	case "/swagger/":
		request := r.Clone(r.Context())
		request.URL.Path = "/swagger/index.html"
		swaggerUIHandler.ServeHTTP(w, request)
	default:
		swaggerUIHandler.ServeHTTP(w, r)
	}
}

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(buildOpenAPISpec())
}

func buildOpenAPISpec() map[string]any {
	genericObject := map[string]any{"type": "object", "additionalProperties": true}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "JFTrade Debug API",
			"version":     "1.0.0",
			"description": "JFTrade sidecar API 的调试文档。Swagger UI 主要覆盖当前常用的 HTTP 调试入口，并展示 SSE 实时流连接说明。",
		},
		"servers": []map[string]any{{
			"url":         "/",
			"description": "当前 JFTrade API 主机",
		}},
		"tags": []map[string]any{
			{"name": "auth", "description": "单管理员登录、会话与注销"},
			{"name": "settings", "description": "Broker 与账户设置"},
			{"name": "system", "description": "运行时与 OpenD 诊断"},
			{"name": "market-data", "description": "行情订阅、快照与 K 线"},
			{"name": "broker", "description": "Broker 运行态与账户视图"},
			{"name": "streaming", "description": "SSE 实时流"},
			{"name": "strategy", "description": "策略与插件只读视图"},
		},
		"paths":      buildOpenAPIPaths(genericObject),
		"components": buildOpenAPIComponents(),
	}
}

func operation(summary string, description string, tags []string, parameters []any, requestBody any, responses map[string]any) map[string]any {
	result := map[string]any{
		"summary":     summary,
		"description": description,
		"tags":        tags,
		"responses":   responses,
	}
	if len(parameters) > 0 {
		result["parameters"] = parameters
	}
	if requestBody != nil {
		result["requestBody"] = requestBody
	}
	return result
}

func streamOperation(summary string, description string) map[string]any {
	return operation(summary, description, []string{"streaming"}, nil, nil, map[string]any{
		"200": map[string]any{
			"description": "SSE 连接成功",
			"content": map[string]any{
				"text/event-stream": map[string]any{
					"schema": map[string]any{"type": "string"},
				},
			},
		},
	})
}

func jsonRequestBody(schema any, required bool) map[string]any {
	return map[string]any{
		"required": required,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func jsonResponse(description string, schema any) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func envelopeSchema(dataSchema any) map[string]any {
	if dataSchema == nil {
		dataSchema = map[string]any{"type": "object", "additionalProperties": true}
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok":        map[string]any{"type": "boolean"},
			"data":      dataSchema,
			"error":     map[string]any{"nullable": true, "$ref": "#/components/schemas/ApiError"},
			"timestamp": map[string]any{"type": "string", "format": "date-time"},
		},
		"required": []string{"ok", "timestamp"},
	}
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func pathParameter(name string, description string, example any) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"required":    true,
		"description": description,
		"schema":      map[string]any{"type": "string"},
		"example":     example,
	}
}

func queryParameter(name string, description string, required bool, schema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"required":    required,
		"description": description,
		"schema":      schema,
	}
}

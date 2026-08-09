package model

import "strings"

// UserFacingADKError maps common GO-ADK/provider errors to user-facing
// messages, falling back to the original error text.
func UserFacingADKError(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "wrote more than the declared content-length"):
		return "模型服务响应异常，请检查模型服务配置或稍后重试。"
	case strings.Contains(lower, "database is locked") || strings.Contains(lower, "sqlite_busy"):
		return "数据库繁忙，请稍后重试。"
	default:
		return err.Error()
	}
}

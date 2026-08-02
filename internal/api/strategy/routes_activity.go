package strategy

import (
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
)

// logPageQuery 是日志/审计分页查询参数。
type logPageQuery struct {
	Limit    httpserver.OptionalIntValue  `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset   httpserver.OptionalIntValue  `form:"offset,parser=encoding.TextUnmarshaler"`
	Level    string                       `form:"level"`
	Kind     string                       `form:"kind"`
	FromTime httpserver.OptionalTimeValue `form:"fromTime,parser=encoding.TextUnmarshaler"`
	ToTime   httpserver.OptionalTimeValue `form:"toTime,parser=encoding.TextUnmarshaler"`
}

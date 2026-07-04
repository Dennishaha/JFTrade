package adk

import (
	"context"
	"fmt"
	"maps"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type sqliteDialector struct {
	Conn gorm.ConnPool
}

func (dialector sqliteDialector) Name() string {
	return "sqlite"
}

func (dialector sqliteDialector) Initialize(db *gorm.DB) error {
	if dialector.Conn == nil {
		return fmt.Errorf("managed SQLite connection is required")
	}
	db.ConnPool = dialector.Conn

	var version string
	if err := db.ConnPool.QueryRowContext(context.Background(), "select sqlite_version()").Scan(&version); err != nil {
		return err
	}
	if compareSQLiteVersion(version, "3.35.0") >= 0 {
		callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
			CreateClauses:        []string{"INSERT", "VALUES", "ON CONFLICT", "RETURNING"},
			UpdateClauses:        []string{"UPDATE", "SET", "WHERE", "RETURNING"},
			DeleteClauses:        []string{"DELETE", "FROM", "WHERE", "RETURNING"},
			LastInsertIDReversed: true,
		})
	} else {
		callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
			LastInsertIDReversed: true,
		})
	}

	maps.Copy(db.ClauseBuilders, dialector.ClauseBuilders())
	return nil
}

func (dialector sqliteDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return migrator.Migrator{Config: migrator.Config{
		DB:                          db,
		Dialector:                   dialector,
		CreateIndexAfterCreateTable: true,
	}}
}

func (dialector sqliteDialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Bool:
		return "numeric"
	case schema.Int, schema.Uint:
		if field.AutoIncrement {
			return "integer PRIMARY KEY AUTOINCREMENT"
		}
		return "integer"
	case schema.Float:
		return "real"
	case schema.String:
		return "text"
	case schema.Time:
		if field.NotNull || field.PrimaryKey {
			return "datetime"
		}
		return "timestamp"
	case schema.Bytes:
		return "blob"
	}
	return string(field.DataType)
}

func (dialector sqliteDialector) DefaultValueOf(field *schema.Field) clause.Expression {
	if field.AutoIncrement {
		return clause.Expr{SQL: "NULL"}
	}
	return clause.Expr{SQL: "DEFAULT"}
}

func (dialector sqliteDialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v any) {
	jftradeLogError(writer.WriteByte('?'))
}

func (dialector sqliteDialector) QuoteTo(writer clause.Writer, str string) {
	var (
		underQuoted, selfQuoted bool
		continuousBacktick      int8
		shiftDelimiter          int8
	)

	for _, value := range []byte(str) {
		switch value {
		case '`':
			continuousBacktick++
			if continuousBacktick == 2 {
				jftradeLogError(writer.WriteString("``"))
				continuousBacktick = 0
			}
		case '.':
			if continuousBacktick > 0 || !selfQuoted {
				shiftDelimiter = 0
				underQuoted = false
				continuousBacktick = 0
				jftradeLogError(writer.WriteString("`"))
			}
			jftradeLogError(writer.WriteByte(value))
			continue
		default:
			if shiftDelimiter-continuousBacktick <= 0 && !underQuoted {
				jftradeLogError(writer.WriteString("`"))
				underQuoted = true
				if selfQuoted = continuousBacktick > 0; selfQuoted {
					continuousBacktick--
				}
			}
			for ; continuousBacktick > 0; continuousBacktick-- {
				jftradeLogError(writer.WriteString("``"))
			}
			jftradeLogError(writer.WriteByte(value))
		}
		shiftDelimiter++
	}

	if continuousBacktick > 0 && !selfQuoted {
		jftradeLogError(writer.WriteString("``"))
	}
	jftradeLogError(writer.WriteString("`"))
}

func (dialector sqliteDialector) Explain(sql string, vars ...any) string {
	return logger.ExplainSQL(sql, nil, `"`, vars...)
}

func (dialector sqliteDialector) ClauseBuilders() map[string]clause.ClauseBuilder {
	return map[string]clause.ClauseBuilder{
		"INSERT": func(c clause.Clause, builder clause.Builder) {
			if insert, ok := c.Expression.(clause.Insert); ok {
				if stmt, ok := builder.(*gorm.Statement); ok {
					jftradeLogError(stmt.WriteString("INSERT "))
					if insert.Modifier != "" {
						jftradeLogError(stmt.WriteString(insert.Modifier))
						jftradeLogError(stmt.WriteByte(' '))
					}
					jftradeLogError(stmt.WriteString("INTO "))
					if insert.Table.Name == "" {
						stmt.WriteQuoted(stmt.Table)
					} else {
						stmt.WriteQuoted(insert.Table)
					}
					return
				}
			}
			c.Build(builder)
		},
		"LIMIT": func(c clause.Clause, builder clause.Builder) {
			if limit, ok := c.Expression.(clause.Limit); ok {
				limitValue := -1
				if limit.Limit != nil && *limit.Limit >= 0 {
					limitValue = *limit.Limit
				}
				if limitValue >= 0 || limit.Offset > 0 {
					jftradeLogError(builder.WriteString("LIMIT "))
					jftradeLogError(builder.WriteString(strconv.Itoa(limitValue)))
				}
				if limit.Offset > 0 {
					jftradeLogError(builder.WriteString(" OFFSET "))
					jftradeLogError(builder.WriteString(strconv.Itoa(limit.Offset)))
				}
			}
		},
		"FOR": func(c clause.Clause, builder clause.Builder) {
			if _, ok := c.Expression.(clause.Locking); ok {
				return
			}
			c.Build(builder)
		},
	}
}

func compareSQLiteVersion(version string, required string) int {
	parse := func(value string) []int {
		parts := make([]int, 0, 3)
		current := 0
		hasDigit := false
		for i := 0; i < len(value); i++ {
			ch := value[i]
			if ch >= '0' && ch <= '9' {
				current = current*10 + int(ch-'0')
				hasDigit = true
				continue
			}
			if ch == '.' {
				if hasDigit {
					parts = append(parts, current)
				} else {
					parts = append(parts, 0)
				}
				current = 0
				hasDigit = false
			}
		}
		if hasDigit {
			parts = append(parts, current)
		}
		return parts
	}

	left := parse(version)
	right := parse(required)
	size := max(len(right), len(left))
	for i := range size {
		leftValue := 0
		rightValue := 0
		if i < len(left) {
			leftValue = left[i]
		}
		if i < len(right) {
			rightValue = right[i]
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}

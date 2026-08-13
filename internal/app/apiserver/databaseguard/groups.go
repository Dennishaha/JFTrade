// Package databaseguard binds database availability requirements to route
// groups so dependency rules are visible at registration time.
package databaseguard

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
)

type Groups struct {
	Assistant *gin.RouterGroup
	Backtest  *gin.RouterGroup
	Execution *gin.RouterGroup
	Research  *gin.RouterGroup
	Strategy  *gin.RouterGroup
	Watchlist *gin.RouterGroup
}

func New(api *gin.RouterGroup, availability dmsrv.Availability) Groups {
	return Groups{
		Assistant: group(api, availability, dmsrv.DatabaseADK, dmsrv.DatabaseADKSession),
		Backtest:  group(api, availability, dmsrv.DatabaseBacktest, dmsrv.DatabaseBacktestRuns),
		Execution: group(api, availability, dmsrv.DatabaseExecution),
		Research:  group(api, availability, dmsrv.DatabaseResearch),
		Strategy:  group(api, availability, dmsrv.DatabaseStrategy),
		Watchlist: group(api, availability, dmsrv.DatabaseWatchlist),
	}
}

func group(api *gin.RouterGroup, availability dmsrv.Availability, ids ...dmsrv.DatabaseID) *gin.RouterGroup {
	return api.Group("", func(c *gin.Context) {
		for _, id := range ids {
			if availability != nil && availability.Unavailable(id) != nil {
				httpserver.WriteError(c, http.StatusServiceUnavailable, "DATABASE_INCOMPATIBLE", fmt.Sprintf("%s database is unavailable; rebuild it in Settings > 数据库重建 and restart JFTrade", id))
				return
			}
		}
		c.Next()
	})
}

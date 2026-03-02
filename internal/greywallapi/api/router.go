package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
)

// Shared holds shared state passed to all handlers.
type Shared struct {
	DB    *greywallapi.DB
	Cache *greywallapi.DNSCache
	Bus   *greywallapi.EventBus
}

// NewRouter creates the Gin router with all routes.
// It returns the engine and the router group for the given pathPrefix,
// so callers can register additional routes under the same group
// without double-nesting the prefix.
func NewRouter(s *Shared, pathPrefix string) (*gin.Engine, *gin.RouterGroup) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Normalize pathPrefix
	if pathPrefix == "" {
		pathPrefix = "/"
	}

	// Redirect engine root to dashboard when pathPrefix is not "/"
	if pathPrefix != "/" {
		dest := strings.TrimRight(pathPrefix, "/") + "/dashboard"
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, dest)
		})
	}

	g := r.Group(pathPrefix)

	// REST API
	api := g.Group("/api")
	{
		api.GET("/dashboard", DashboardHandler(s))

		api.GET("/pending/count", PendingCountHandler(s))
		api.GET("/pending", PendingListHandler(s))
		api.POST("/pending/:id/allow", PendingAllowHandler(s))
		api.POST("/pending/:id/deny", PendingDenyHandler(s))
		api.DELETE("/pending/:id", PendingDeleteHandler(s))
		api.POST("/pending/bulk-allow", PendingBulkAllowHandler(s))
		api.POST("/pending/bulk-dismiss", PendingBulkDismissHandler(s))

		api.GET("/rules", RulesListHandler(s))
		api.POST("/rules", RulesCreateHandler(s))
		api.POST("/rules/ingest", RulesIngestHandler(s))
		api.PUT("/rules/:id", RulesUpdateHandler(s))
		api.DELETE("/rules/:id", RulesDeleteHandler(s))

		api.GET("/logs", LogsListHandler(s))
		api.GET("/logs/stats", LogsStatsHandler(s))
	}

	// WebSocket
	g.GET("/ws", WebSocketHandler(s))

	// Static files (embedded)
	g.GET("/static/*filepath", func(c *gin.Context) {
		c.FileFromFS("static"+c.Param("filepath"), http.FS(greywallapi.StaticFS))
	})

	return r, g
}

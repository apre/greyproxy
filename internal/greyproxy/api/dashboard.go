package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	greyproxy "github.com/greyhavenhq/greyproxy/internal/greyproxy"
)

func DashboardHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now().UTC()
		var fromDate, toDate time.Time

		period := c.DefaultQuery("period", "today")
		switch period {
		case "7d":
			fromDate = now.AddDate(0, 0, -7)
		case "30d":
			fromDate = now.AddDate(0, 0, -30)
		default: // today
			fromDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		}
		toDate = now

		if v := c.Query("from_date"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				fromDate = t
			}
		}
		if v := c.Query("to_date"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				toDate = t
			}
		}

		groupBy := c.DefaultQuery("group_by", "auto")
		if groupBy == "auto" {
			diff := toDate.Sub(fromDate)
			switch {
			case diff > 14*24*time.Hour:
				groupBy = "day"
			case diff > 48*time.Hour:
				groupBy = "day"
			default:
				groupBy = "hour"
			}
		}

		recentLimit, _ := strconv.Atoi(c.DefaultQuery("recent_limit", "10"))

		stats, err := greyproxy.GetDashboardStats(s.DB, fromDate, toDate, groupBy, recentLimit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}

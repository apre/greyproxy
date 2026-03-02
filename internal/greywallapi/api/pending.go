package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
)

func PendingCountHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, err := greywallapi.GetPendingCount(s.DB)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"count": count})
	}
}

func PendingListHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

		items, total, err := greywallapi.GetPendingRequests(s.DB, greywallapi.PendingFilter{
			Container:   c.Query("container"),
			Destination: c.Query("destination"),
			Limit:       limit,
			Offset:      offset,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		jsonItems := make([]greywallapi.PendingRequestJSON, len(items))
		for i, item := range items {
			jsonItems[i] = item.ToJSON()
		}

		c.JSON(http.StatusOK, gin.H{
			"items": jsonItems,
			"total": total,
		})
	}
}

type allowRequest struct {
	Scope    string  `json:"scope"`
	Duration string  `json:"duration"`
	Notes    *string `json:"notes"`
}

func PendingAllowHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		var req allowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Scope == "" {
			req.Scope = "exact"
		}
		if req.Duration == "" {
			req.Duration = "permanent"
		}

		rule, err := greywallapi.AllowPending(s.DB, id, req.Scope, req.Duration, req.Notes)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		s.Bus.Publish(greywallapi.Event{
			Type: greywallapi.EventPendingAllowed,
			Data: gin.H{"pending_id": id, "rule": rule.ToJSON()},
		})

		c.JSON(http.StatusOK, gin.H{
			"rule":            rule.ToJSON(),
			"pending_removed": true,
		})
	}
}

func PendingDenyHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		var req allowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Scope == "" {
			req.Scope = "exact"
		}
		if req.Duration == "" {
			req.Duration = "permanent"
		}

		rule, err := greywallapi.DenyPending(s.DB, id, req.Scope, req.Duration, req.Notes)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		s.Bus.Publish(greywallapi.Event{
			Type: greywallapi.EventPendingDismissed,
			Data: gin.H{"pending_id": id, "rule": rule.ToJSON()},
		})

		c.JSON(http.StatusOK, gin.H{
			"rule":            rule.ToJSON(),
			"pending_removed": true,
		})
	}
}

func PendingDeleteHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		removed, err := greywallapi.DeletePending(s.DB, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if removed {
			s.Bus.Publish(greywallapi.Event{
				Type: greywallapi.EventPendingDismissed,
				Data: gin.H{"pending_id": id},
			})
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "removed": removed})
	}
}

type bulkAllowRequest struct {
	PendingIDs []int64 `json:"pending_ids"`
	Duration   string  `json:"duration"`
}

func PendingBulkAllowHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req bulkAllowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Duration == "" {
			req.Duration = "permanent"
		}

		var rules []greywallapi.RuleJSON
		removed := 0
		for _, pid := range req.PendingIDs {
			rule, err := greywallapi.AllowPending(s.DB, pid, "exact", req.Duration, nil)
			if err != nil {
				continue
			}
			rules = append(rules, rule.ToJSON())
			removed++
			s.Bus.Publish(greywallapi.Event{
				Type: greywallapi.EventPendingAllowed,
				Data: gin.H{"pending_id": pid, "rule": rule.ToJSON()},
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"rules_created":   rules,
			"pending_removed": removed,
		})
	}
}

type bulkDismissRequest struct {
	PendingIDs []int64 `json:"pending_ids"`
}

func PendingBulkDismissHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req bulkDismissRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		removed := 0
		for _, pid := range req.PendingIDs {
			ok, err := greywallapi.DeletePending(s.DB, pid)
			if err == nil && ok {
				removed++
				s.Bus.Publish(greywallapi.Event{
					Type: greywallapi.EventPendingDismissed,
					Data: gin.H{"pending_id": pid},
				})
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "removed": removed})
	}
}

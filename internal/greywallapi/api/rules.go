package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
)

func RulesListHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		includeExpired := c.Query("include_expired") == "true"

		items, total, err := greywallapi.GetRules(s.DB, greywallapi.RuleFilter{
			Container:      c.Query("container"),
			Destination:    c.Query("destination"),
			Action:         c.Query("action"),
			IncludeExpired: includeExpired,
			Limit:          limit,
			Offset:         offset,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		jsonItems := make([]greywallapi.RuleJSON, len(items))
		for i, item := range items {
			jsonItems[i] = item.ToJSON()
		}

		c.JSON(http.StatusOK, gin.H{
			"items": jsonItems,
			"total": total,
		})
	}
}

func RulesCreateHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input greywallapi.RuleCreateInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rule, err := greywallapi.CreateRule(s.DB, input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, rule.ToJSON())
	}
}

func RulesUpdateHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		var input greywallapi.RuleUpdateInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rule, err := greywallapi.UpdateRule(s.DB, id, input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if rule == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
			return
		}

		c.JSON(http.StatusOK, rule.ToJSON())
	}
}

func RulesDeleteHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		deleted, err := greywallapi.DeleteRule(s.DB, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "deleted": deleted})
	}
}

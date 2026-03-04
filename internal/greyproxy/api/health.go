package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type healthResponse struct {
	Service string         `json:"service"`
	Version string         `json:"version"`
	Status  string         `json:"status"`
	Ports   map[string]int `json:"ports"`
}

func HealthHandler(s *Shared) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "ok"
		code := http.StatusOK

		if err := s.DB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, healthResponse{
				Service: "greyproxy",
				Version: s.Version,
				Status:  "unhealthy",
				Ports:   s.Ports,
			})
			return
		}

		c.JSON(code, healthResponse{
			Service: "greyproxy",
			Version: s.Version,
			Status:  status,
			Ports:   s.Ports,
		})
	}
}

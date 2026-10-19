package codehandlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ErrorNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"status":  404,
		"error":   "endpoint_not_found",
		"message": "The requested resource does not exist",
	})
}

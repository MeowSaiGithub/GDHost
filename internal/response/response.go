package response

import (
	"GDHost/internal/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func StatusInternalServerError(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"message": "internal server error",
		"ts":      time.Now(),
	})
}

func StatusBadRequest(c *gin.Context, payload string) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"message": payload,
		"ts":      time.Now(),
	})
}

func StatusDeployment(c *gin.Context, dep *model.Deployment) {
	payload := map[string]interface{}{
		"ID":         dep.Id,
		"created_at": dep.CreatedAt,
		"updated_at": dep.UpdatedAt,
		"name":       dep.Name,
		"stage":      dep.Stage.String(),
	}
	c.JSON(http.StatusOK, gin.H{
		"deployment": payload,
		"ts":         time.Now(),
	})
}

func StatusCommonOK(c *gin.Context, payload string) {
	c.JSON(http.StatusOK, gin.H{
		"message": payload,
		"ts":      time.Now(),
	})
}

func StatusNotFound(c *gin.Context, payload string) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"message": payload,
		"ts":      time.Now(),
	})
}

func StatusAccepted(c *gin.Context, payload string) {
	c.JSON(http.StatusAccepted, gin.H{
		"message": payload,
		"ts":      time.Now(),
	})
}

func StatusNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func StatusDeployments(c *gin.Context, deps *[]model.Deployment) {
	var payload []map[string]interface{}
	for _, dep := range *deps {
		depMap := map[string]interface{}{
			"ID":         dep.Id,
			"created_at": dep.CreatedAt,
			"updated_at": dep.UpdatedAt,
			"name":       dep.Name,
			"stage":      dep.Stage.String(),
		}
		payload = append(payload, depMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"deployments": payload,
		"ts":          time.Now(),
	})
}

func StatusUnProcessed(c *gin.Context, payload string) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{
		"message": payload,
		"ts":      time.Now(),
	})
}

func StatusConflicted(c *gin.Context, payload string) {
	c.JSON(http.StatusConflict, gin.H{
		"message": payload,
		"ts":      time.Now,
	})
}

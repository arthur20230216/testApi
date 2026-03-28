package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"modelprobe/backend/internal/model"
)

func (h *ProbeHandler) getSystemSettings(context *gin.Context) {
	response, err := h.systemSettings.GetResponse(context.Request.Context())
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get system settings", "detail": err.Error()})
		return
	}

	context.JSON(http.StatusOK, response)
}

func (h *ProbeHandler) updateSystemSettings(context *gin.Context) {
	var request model.SystemSettingsUpdateRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	response, err := h.systemSettings.Update(context.Request.Context(), request)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update system settings", "detail": err.Error()})
		return
	}

	context.JSON(http.StatusOK, response)
}

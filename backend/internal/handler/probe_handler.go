package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"modelprobe/backend/internal/model"
	"modelprobe/backend/internal/repository"
	"modelprobe/backend/internal/service"
)

type ProbeHandler struct {
	repo         *repository.PostgresRepository
	probeService *service.ProbeService
}

func NewProbeHandler(repo *repository.PostgresRepository, probeService *service.ProbeService) *ProbeHandler {
	return &ProbeHandler{
		repo:         repo,
		probeService: probeService,
	}
}

func (h *ProbeHandler) Register(api *gin.RouterGroup) {
	api.GET("/health", h.health)
	api.POST("/probes", h.createProbe)
	api.GET("/probes", h.listProbes)
	api.GET("/probes/:id", h.getProbe)
	api.GET("/rankings/stations", h.getStationRanking)
	api.GET("/rankings/groups", h.getGroupRanking)
}

func (h *ProbeHandler) health(context *gin.Context) {
	context.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"service": "model-probe-go",
		"now":     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (h *ProbeHandler) createProbe(context *gin.Context) {
	var request model.ProbeRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	record, err := h.probeService.RunProbe(context.Request.Context(), request)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "probe failed", "detail": err.Error()})
		return
	}

	if err := h.repo.CreateProbe(context.Request.Context(), record); err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save probe", "detail": err.Error()})
		return
	}

	response := model.ProbeResponse{
		Probe: record,
		Summary: model.ProbeSummary{
			Verdict:       record.Verdict,
			TrustScore:    record.TrustScore,
			PrimaryFamily: record.PrimaryFamily,
			Suspicious:    len(record.SuspicionReasons) > 0,
		},
	}

	context.JSON(http.StatusCreated, response)
}

func (h *ProbeHandler) listProbes(context *gin.Context) {
	limit := parseLimit(context.Query("limit"), 20, 1, 100)

	items, err := h.repo.ListRecentProbes(context.Request.Context(), limit)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list probes", "detail": err.Error()})
		return
	}

	context.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ProbeHandler) getProbe(context *gin.Context) {
	record, err := h.repo.GetProbeByID(context.Request.Context(), context.Param("id"))
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get probe", "detail": err.Error()})
		return
	}

	if record == nil {
		context.JSON(http.StatusNotFound, gin.H{"error": "probe not found"})
		return
	}

	context.JSON(http.StatusOK, gin.H{"probe": record})
}

func (h *ProbeHandler) getStationRanking(context *gin.Context) {
	h.getRanking(context, "station")
}

func (h *ProbeHandler) getGroupRanking(context *gin.Context) {
	h.getRanking(context, "group")
}

func (h *ProbeHandler) getRanking(context *gin.Context, scope string) {
	limit := parseLimit(context.Query("limit"), 10, 1, 50)

	ranking, err := h.repo.GetRanking(context.Request.Context(), scope, limit)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build ranking", "detail": err.Error()})
		return
	}

	context.JSON(http.StatusOK, ranking)
}

func parseLimit(raw string, fallback int, min int, max int) int {
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	if value < min {
		return min
	}

	if value > max {
		return max
	}

	return value
}

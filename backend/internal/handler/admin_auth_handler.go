package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"modelprobe/backend/internal/model"
	"modelprobe/backend/internal/service"
)

const adminUserContextKey = "adminUser"

func (h *ProbeHandler) requireAdminSession() gin.HandlerFunc {
	return func(context *gin.Context) {
		user, err := h.adminAuth.RequireSession(context.Request.Context(), h.readSessionToken(context))
		if err != nil {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "admin authentication required"})
			return
		}

		context.Set(adminUserContextKey, user)
		context.Next()
	}
}

func (h *ProbeHandler) getAdminSession(context *gin.Context) {
	response, err := h.adminAuth.GetSessionState(context.Request.Context(), h.readSessionToken(context))
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to inspect admin session", "detail": err.Error()})
		return
	}

	context.JSON(http.StatusOK, response)
}

func (h *ProbeHandler) loginAdmin(context *gin.Context) {
	var request model.AdminLoginRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	request = request.Normalize()
	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	user, rawToken, expiresAt, err := h.adminAuth.Login(
		context.Request.Context(),
		request.Username,
		request.Password,
		context.GetHeader("User-Agent"),
		context.ClientIP(),
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminNotConfigured):
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin account is not configured"})
		case errors.Is(err, service.ErrInvalidCredentials):
			context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin credentials"})
		default:
			context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to login admin", "detail": err.Error()})
		}
		return
	}

	h.writeSessionCookie(context, rawToken, expiresAt)
	context.JSON(http.StatusOK, model.AdminSessionResponse{
		Configured:    true,
		Authenticated: true,
		User:          user,
	})
}

func (h *ProbeHandler) logoutAdmin(context *gin.Context) {
	if err := h.adminAuth.Logout(context.Request.Context(), h.readSessionToken(context)); err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to logout admin", "detail": err.Error()})
		return
	}

	h.clearSessionCookie(context)
	context.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ProbeHandler) updateAdminAccount(context *gin.Context) {
	adminUser, ok := currentAdminUser(context)
	if !ok {
		context.JSON(http.StatusUnauthorized, gin.H{"error": "admin authentication required"})
		return
	}

	var request model.AdminAccountUpdateRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	request.Username = strings.TrimSpace(request.Username)
	request.CurrentPassword = strings.TrimSpace(request.CurrentPassword)
	request.NewPassword = strings.TrimSpace(request.NewPassword)
	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "detail": err.Error()})
		return
	}

	user, err := h.adminAuth.UpdateAdminAccount(
		context.Request.Context(),
		adminUser.ID,
		request.CurrentPassword,
		request.Username,
		request.NewPassword,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			context.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin credentials"})
		default:
			if strings.Contains(err.Error(), "already exists") {
				context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			context.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update admin account", "detail": err.Error()})
		}
		return
	}

	context.JSON(http.StatusOK, gin.H{"user": user})
}

func currentAdminUser(context *gin.Context) (*model.AdminUserRecord, bool) {
	value, ok := context.Get(adminUserContextKey)
	if !ok {
		return nil, false
	}

	user, ok := value.(*model.AdminUserRecord)
	return user, ok
}

func (h *ProbeHandler) readSessionToken(context *gin.Context) string {
	value, err := context.Cookie(h.sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (h *ProbeHandler) writeSessionCookie(context *gin.Context, rawToken string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	context.SetSameSite(http.SameSiteLaxMode)
	context.SetCookie(
		h.sessionCookieName,
		rawToken,
		maxAge,
		"/",
		"",
		isSecureRequest(context),
		true,
	)
}

func (h *ProbeHandler) clearSessionCookie(context *gin.Context) {
	context.SetSameSite(http.SameSiteLaxMode)
	context.SetCookie(
		h.sessionCookieName,
		"",
		-1,
		"/",
		"",
		isSecureRequest(context),
		true,
	)
}

func isSecureRequest(context *gin.Context) bool {
	if context.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(context.GetHeader("X-Forwarded-Proto"), "https")
}

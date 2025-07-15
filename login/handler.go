package login

import (
	"CarBN/common"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

type HTTPHandler struct {
	service  *Service
	validate *validator.Validate
}

func NewHTTPHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{
		service:  service,
		validate: validator.New(),
	}
}

type GoogleSignInRequest struct {
	IDToken     string `json:"idToken" validate:"required"`
	DisplayName string `json:"displayName" validate:"required,min=3,max=50"`
}

type AppleSignInRequest struct {
	IDToken     string `json:"idToken" validate:"required"`
	DisplayName string `json:"displayName" validate:"required,min=3,max=50"`
}

type AuthResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
}

type ErrorResponse struct {
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// HandleGoogleSignIn handles the Google Sign-In authentication
func (h *HTTPHandler) HandleGoogleSignIn(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Google SignIn] Received request from IP: %s, User-Agent: %s", r.RemoteAddr, r.UserAgent())

	if r.Method != http.MethodPost {
		logger.Printf("[Google SignIn] Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GoogleSignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("[Google SignIn] Failed to decode request body: %v", err)
		sendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Log sanitized request info (don't log the full token)
	if len(req.IDToken) > 10 {
		logger.Printf("[Google SignIn] Processing sign-in for DisplayName: '%s', Token prefix: %s...",
			req.DisplayName, req.IDToken[:10])
	} else {
		logger.Printf("[Google SignIn] Processing sign-in with invalid token format")
	}

	// Validate request
	if err := h.validate.Struct(req); err != nil {
		logger.Printf("[Google SignIn] Validation failed: %v", err)
		sendValidationError(w, err)
		return
	}
	logger.Printf("[Google SignIn] Request validation successful")

	startTime := time.Now()
	// Process Google sign-in
	accessToken, refreshToken, err := h.service.GoogleSignIn(r.Context(), req.IDToken, req.DisplayName)
	if err != nil {
		logger.Printf("[Google SignIn] Authentication failed: %v", err)
		sendJSONError(w, err.Error(), http.StatusUnauthorized)
		return
	}
	processingTime := time.Since(startTime)

	// Return tokens
	resp := AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.service.config.AccessTokenExpiry.Seconds()),
	}

	logger.Printf("[Google SignIn] Authentication successful, processing time: %v, token expires in: %d seconds",
		processingTime, resp.ExpiresIn)
	sendJSONResponse(w, resp, http.StatusOK)
}

// HandleAppleSignIn handles the Apple Sign-In authentication
func (h *HTTPHandler) HandleAppleSignIn(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Apple SignIn] Received request from IP: %s, User-Agent: %s", r.RemoteAddr, r.UserAgent())

	if r.Method != http.MethodPost {
		logger.Printf("[Apple SignIn] Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AppleSignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("[Apple SignIn] Failed to decode request body: %v", err)
		sendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Log sanitized request info
	if len(req.IDToken) > 10 {
		logger.Printf("[Apple SignIn] Processing sign-in for DisplayName: '%s', Token prefix: %s...",
			req.DisplayName, req.IDToken[:10])
	} else {
		logger.Printf("[Apple SignIn] Processing sign-in with invalid token format")
	}

	// Validate request
	if err := h.validate.Struct(req); err != nil {
		logger.Printf("[Apple SignIn] Validation failed: %v", err)
		sendValidationError(w, err)
		return
	}
	logger.Printf("[Apple SignIn] Request validation successful")

	startTime := time.Now()
	// Process Apple sign-in
	accessToken, refreshToken, err := h.service.AppleSignIn(r.Context(), req.IDToken, req.DisplayName)
	if err != nil {
		logger.Printf("[Apple SignIn] Authentication failed: %v", err)
		sendJSONError(w, err.Error(), http.StatusUnauthorized)
		return
	}
	processingTime := time.Since(startTime)

	// Return tokens
	resp := AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.service.config.AccessTokenExpiry.Seconds()),
	}

	logger.Printf("[Apple SignIn] Authentication successful, processing time: %v, token expires in: %d seconds",
		processingTime, resp.ExpiresIn)
	sendJSONResponse(w, resp, http.StatusOK)
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (h *HTTPHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Token Refresh] Request received from IP: %s, User-Agent: %s", r.RemoteAddr, r.UserAgent())

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("[Token Refresh] Failed to decode request body: %v", err)
		sendJSONError(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Log sanitized token
	if len(req.RefreshToken) > 10 {
		logger.Printf("[Token Refresh] Processing refresh token with prefix: %s...", req.RefreshToken[:10])
	} else {
		logger.Printf("[Token Refresh] Processing with invalid token format")
	}

	if err := h.validate.Struct(req); err != nil {
		logger.Printf("[Token Refresh] Request validation failed: %v", err)
		sendValidationError(w, err)
		return
	}

	logger.Printf("[Token Refresh] Request validated, attempting to refresh tokens")
	startTime := time.Now()
	newAccessToken, newRefreshToken, err := h.service.RefreshTokens(r.Context(), req.RefreshToken)
	processingTime := time.Since(startTime)

	if err != nil {
		logger.Printf("[Token Refresh] Failed to refresh tokens: %v (processing time: %v)", err, processingTime)
		statusCode := http.StatusUnauthorized
		message := "Token refresh failed"

		if strings.Contains(err.Error(), "invalid or expired refresh token") {
			logger.Printf("[Token Refresh] Token was invalid or expired")
			message = "Refresh token has expired or is invalid"
		} else if strings.Contains(err.Error(), "failed to sign") {
			logger.Printf("[Token Refresh] Internal error while signing new tokens")
			statusCode = http.StatusInternalServerError
			message = "Failed to generate new tokens"
		}

		sendJSONError(w, message, statusCode)
		return
	}

	resp := AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.service.config.AccessTokenExpiry.Seconds()),
	}

	logger.Printf("[Token Refresh] Successfully generated new tokens in %v, expires in %d seconds",
		processingTime, resp.ExpiresIn)
	sendJSONResponse(w, resp, http.StatusOK)
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (h *HTTPHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Logout] Request received from IP: %s, User-Agent: %s", r.RemoteAddr, r.UserAgent())

	var req LogoutRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("[Logout] Failed to decode request body: %v", err)
		sendJSONError(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Log sanitized token
	if len(req.RefreshToken) > 10 {
		logger.Printf("[Logout] Processing logout with token prefix: %s...", req.RefreshToken[:10])
	} else {
		logger.Printf("[Logout] Processing with invalid token format")
	}

	if err := h.validate.Struct(req); err != nil {
		logger.Printf("[Logout] Validation failed: %v", err)
		sendValidationError(w, err)
		return
	}

	startTime := time.Now()
	if err := h.service.Logout(r.Context(), req.RefreshToken); err != nil {
		logger.Printf("[Logout] Failed after %v: %v", time.Since(startTime), err)
		sendJSONError(w, "Logout failed", http.StatusUnauthorized)
		return
	}

	logger.Printf("[Logout] User successfully logged out in %v", time.Since(startTime))
	sendJSONResponse(w, map[string]string{
		"message": "Successfully logged out",
	}, http.StatusOK)
}

func (h *HTTPHandler) HandleCheckLoginStatus(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Auth Check] Login status check from IP: %s, User-Agent: %s", r.RemoteAddr, r.UserAgent())

	// Since this handler would be called after AuthMiddleware, we can assume authentication was successful
	userID := r.Context().Value(common.UserIDCtxKey).(int)
	logger.Printf("[Auth Check] User ID %d is authenticated", userID)

	sendJSONResponse(w, map[string]interface{}{
		"status": "authenticated",
		"userId": userID,
	}, http.StatusOK)
}

// Helper functions
func sendJSONResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func sendJSONError(w http.ResponseWriter, message string, status int) {
	sendJSONResponse(w, ErrorResponse{Message: message}, status)
}

func sendValidationError(w http.ResponseWriter, err error) {
	errors := make(map[string]string)
	for _, err := range err.(validator.ValidationErrors) {
		field := err.Field()
		switch err.Tag() {
		case "required":
			errors[field] = "This field is required"
		case "min":
			errors[field] = fmt.Sprintf("Minimum length is %s", err.Param())
		case "max":
			errors[field] = fmt.Sprintf("Maximum length is %s", err.Param())
		default:
			errors[field] = "Invalid value"
		}
	}

	sendJSONResponse(w, ErrorResponse{
		Message: "Validation failed",
		Errors:  errors,
	}, http.StatusBadRequest)
}

// In your HTTP handlers file
func (h *HTTPHandler) HandleAppleEmailUpdateNotification(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Apple Email Update] Notification received from IP: %s", r.RemoteAddr)

	var payload AppleEmailUpdatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		logger.Printf("[Apple Email Update] Failed to decode payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Log sanitized information
	logger.Printf("[Apple Email Update] Processing update for type: %s, user sub: %s...",
		payload.Type, payload.Sub[:5])

	startTime := time.Now()
	if err := h.service.HandleAppleEmailUpdate(r.Context(), payload); err != nil {
		logger.Printf("[Apple Email Update] Failed after %v: %v", time.Since(startTime), err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	logger.Printf("[Apple Email Update] Successfully processed email update in %v", time.Since(startTime))
	w.WriteHeader(http.StatusOK)
}

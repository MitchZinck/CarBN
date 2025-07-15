package scan

import (
	"CarBN/common"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
)

type HTTPHandler struct {
	service *Service
}

func NewHTTPHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{
		service: service,
	}
}

func (h *HTTPHandler) HandleScanPost(w http.ResponseWriter, r *http.Request) {
	loggerIntf := r.Context().Value(common.LoggerCtxKey)
	logger, ok := loggerIntf.(*log.Logger)
	if !ok || logger == nil {
		logger = log.New(os.Stdout, "[CarBN] ", log.LstdFlags)
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	defer r.Body.Close()

	// Parse and validate request
	req, err := h.parseScanRequest(r)
	if err != nil {
		logger.Printf("Error parsing scan request: %v", err)
		h.handleError(w, err, http.StatusBadRequest)
		return
	}

	// Process the scan
	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("Invalid user ID in context")
		h.handleError(w, errors.New("invalid user ID in context"), http.StatusInternalServerError)
		return
	}

	logger.Printf("Processing scan for user %d", userID)
	response, err := h.service.ScanImage(r.Context(), userID, req.Base64Image)
	if err != nil {
		logger.Printf("Scan processing failed for user %d: %v", userID, err)
		switch err.Error() {
		case common.DuplicateScanError:
			h.handleError(w, err, http.StatusConflict) // 409 Conflict for duplicates
		case common.RejectedScanError:
			h.handleError(w, err, http.StatusBadRequest)
		default:
			h.handleError(w, fmt.Errorf("scan processing failed: %w", err), http.StatusInternalServerError)
		}
		return
	}

	logger.Printf("Successfully processed scan for user %d, car ID: %d", userID, response.ID)
	h.writeJSONResponse(w, http.StatusCreated, response)
}

type scanRequest struct {
	Base64Image string `json:"base64_image"`
}

func (h *HTTPHandler) parseScanRequest(r *http.Request) (*scanRequest, error) {
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if req.Base64Image == "" {
		return nil, errors.New("missing base64_image")
	}

	// Basic Base64 validation
	if !isValidBase64(req.Base64Image) {
		return nil, errors.New("invalid base64 format")
	}

	return &req, nil
}

func (h *HTTPHandler) handleError(w http.ResponseWriter, err error, statusCode int) {
	response := struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}{
		Error: err.Error(),
		Code:  statusCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode error response", http.StatusInternalServerError)
	}
}

func (h *HTTPHandler) writeJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
	}
}

func isValidBase64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

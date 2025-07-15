package trade

import (
	"CarBN/common"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

type HTTPHandler struct {
	service *Service
}

func NewHTTPHandler(s *Service) *HTTPHandler {
	return &HTTPHandler{service: s}
}

type tradeRequest struct {
	UserIDTo       int   `json:"user_id_to"`
	UserFromCarIDs []int `json:"user_from_user_car_ids"`
	UserToCarIDs   []int `json:"user_to_user_car_ids"`
}

type tradeRequestResponse struct {
	TradeID  int    `json:"trade_id"`
	Response string `json:"response"`
}

type getUserTradesResponse struct {
	Trades     []TradeInfo `json:"trades"`
	TotalCount int         `json:"total_count"`
}

func (h *HTTPHandler) HandleCreateTrade(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	var req tradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Failed to decode create trade request: %v", err)
		http.Error(w, "invalid request data", http.StatusBadRequest)
		return
	}

	userIDFrom, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("User ID not found in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	logger.Printf("Creating trade request from user %d to user %d", userIDFrom, req.UserIDTo)
	if err := h.service.CreateTrade(r.Context(), userIDFrom, req.UserIDTo, req.UserFromCarIDs, req.UserToCarIDs); err != nil {
		logger.Printf("Failed to create trade: %v", err)
		http.Error(w, "failed to create trade", http.StatusInternalServerError)
		return
	}

	logger.Printf("Trade created successfully between users %d and %d", userIDFrom, req.UserIDTo)
	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) HandleTradeRequestResponse(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	var req tradeRequestResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Failed to decode trade response request: %v", err)
		http.Error(w, "invalid request data", http.StatusBadRequest)
		return
	}

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("User ID not found in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	logger.Printf("Processing trade %d response: %s", req.TradeID, req.Response)
	switch req.Response {
	case "accept":
		if err := h.service.AcceptTrade(r.Context(), userID, req.TradeID); err != nil {
			logger.Printf("Failed to accept trade %d: %v", req.TradeID, err)
			http.Error(w, "failed to accept trade", http.StatusInternalServerError)
			return
		}
		logger.Printf("Trade %d accepted successfully", req.TradeID)
	case "decline":
		if err := h.service.DeclineTrade(r.Context(), req.TradeID); err != nil {
			logger.Printf("Failed to decline trade %d: %v", req.TradeID, err)
			http.Error(w, "failed to decline trade", http.StatusInternalServerError)
			return
		}
		logger.Printf("Trade %d declined successfully", req.TradeID)
	default:
		logger.Printf("Invalid trade response received: %s", req.Response)
		http.Error(w, "invalid response type, must be 'accept' or 'decline'", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) HandleGetUserTrades(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("User ID not found in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	// Get page and pageSize from query parameters
	page := 1
	pageSize := 10 // Default page size

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if parsedPage, err := strconv.Atoi(pageStr); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if parsedPageSize, err := strconv.Atoi(pageSizeStr); err == nil && parsedPageSize > 0 {
			pageSize = parsedPageSize
		}
	}

	trades, totalCount, err := h.service.GetUserTrades(r.Context(), userID, page, pageSize)
	if err != nil {
		logger.Printf("Failed to get user trades: %v", err)
		http.Error(w, "failed to get trades", http.StatusInternalServerError)
		return
	}

	response := getUserTradesResponse{
		Trades:     trades,
		TotalCount: totalCount,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Printf("Failed to encode response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *HTTPHandler) HandleGetTrade(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	// Extract trade ID from URL path
	tradeIDStr := r.PathValue("trade_id")
	tradeID, err := strconv.Atoi(tradeIDStr)
	if err != nil {
		logger.Printf("Invalid trade ID format: %s", tradeIDStr)
		http.Error(w, "invalid trade ID", http.StatusBadRequest)
		return
	}

	trade, err := h.service.GetTradeByID(r.Context(), tradeID)
	if err != nil {
		logger.Printf("Failed to get trade: %v", err)
		http.Error(w, "failed to get trade", http.StatusInternalServerError)
		return
	}

	if trade == nil {
		http.Error(w, "trade not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(trade); err != nil {
		logger.Printf("Failed to encode response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

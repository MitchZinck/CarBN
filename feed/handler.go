package feed

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

func (h *HTTPHandler) HandleGetFeed(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		http.Error(w, "logger not found in context", http.StatusInternalServerError)
		return
	}

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	pageSizeStr := r.URL.Query().Get("page_size")
	pageSize := 10 // default
	if pageSizeStr != "" {
		var err error
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			logger.Printf("invalid page_size parameter: %v", err)
			http.Error(w, "invalid page_size", http.StatusBadRequest)
			return
		}
	}

	cursor := r.URL.Query().Get("cursor")
	feedType := r.URL.Query().Get("feed_type")
	if feedType != "global" {
		feedType = "friends"
	}

	feed, err := h.service.GetFeed(r.Context(), userID, cursor, pageSize, feedType)
	if err != nil {
		logger.Printf("failed to get feed: %v", err)
		http.Error(w, "failed to get feed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(feed); err != nil {
		logger.Printf("failed to encode feed response: %v", err)
		http.Error(w, "failed to encode feed", http.StatusInternalServerError)
		return
	}

	logger.Printf("successfully retrieved feed for user %d with %d items", userID, len(feed.Items))
}

func (h *HTTPHandler) HandleGetFeedItem(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	if !ok {
		http.Error(w, "logger not found in context", http.StatusInternalServerError)
		return
	}

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	feedItemID, err := strconv.Atoi(r.PathValue("feed_item_id"))
	if err != nil {
		logger.Printf("invalid feed item ID: %v", err)
		http.Error(w, "invalid feed item ID", http.StatusBadRequest)
		return
	}

	item, err := h.service.GetFeedItem(r.Context(), feedItemID, userID)
	if err != nil {
		logger.Printf("failed to get feed item: %v", err)
		http.Error(w, "failed to get feed item", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(item); err != nil {
		logger.Printf("failed to encode feed item response: %v", err)
		http.Error(w, "failed to encode feed item", http.StatusInternalServerError)
		return
	}

	logger.Printf("successfully retrieved feed item %d", feedItemID)
}

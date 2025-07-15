package friends

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

type friendRequest struct {
	FriendID int `json:"friend_id"`
}

type friendRequestResponse struct {
	RequestID int    `json:"request_id"`
	Response  string `json:"response"`
}

func (h *HTTPHandler) HandleSendFriendRequest(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	var req friendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Invalid friend request data: %v", err)
		http.Error(w, "invalid request data", http.StatusBadRequest)
		return
	}

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("Invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	if err := h.service.SendFriendRequest(r.Context(), userID, req.FriendID); err != nil {
		logger.Printf("Failed to process friend request from user %d to user %d: %v", userID, req.FriendID, err)
		http.Error(w, "failed to send friend request", http.StatusInternalServerError)
		return
	}

	logger.Printf("Friend request processed successfully from user %d to user %d", userID, req.FriendID)
	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) HandleFriendRequestResponse(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	var req friendRequestResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Invalid friend request response data: %v", err)
		http.Error(w, "invalid request data", http.StatusBadRequest)
		return
	}

	if req.Response == "accept" {
		if err := h.service.AcceptFriendRequest(r.Context(), req.RequestID); err != nil {
			logger.Printf("Failed to accept friend request %d: %v", req.RequestID, err)
			http.Error(w, "failed to accept request", http.StatusInternalServerError)
			return
		}
		logger.Printf("Friend request %d accepted successfully", req.RequestID)
	} else if req.Response == "reject" {
		if err := h.service.RejectFriendRequest(r.Context(), req.RequestID); err != nil {
			logger.Printf("Failed to reject friend request %d: %v", req.RequestID, err)
			http.Error(w, "failed to reject request", http.StatusInternalServerError)
			return
		}
		logger.Printf("Friend request %d rejected successfully", req.RequestID)
	} else {
		logger.Printf("Invalid friend request response type: %s", req.Response)
		http.Error(w, "invalid response", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) HandleGetFriends(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	// Check for optional user_id parameter
	targetUserIDStr := r.PathValue("user_id")
	targetUserID, err := strconv.Atoi(targetUserIDStr)
	if err != nil {
		logger.Printf("Invalid user ID: %s", targetUserIDStr)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = 10
	}

	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	friends, err := h.service.GetFriends(r.Context(), targetUserID, limit, offset)
	if err != nil {
		logger.Printf("Failed to get friends for user %d: %v", targetUserID, err)
		http.Error(w, "failed to get friends", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(friends); err != nil {
		logger.Printf("Failed to encode friends response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully returned %d friends for user %d", len(friends.Friends), targetUserID)
}

func (h *HTTPHandler) HandleCheckFriendship(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	// Get the target user ID from the path
	targetUserIDStr := r.PathValue("user_id")
	targetUserID, err := strconv.Atoi(targetUserIDStr)
	if err != nil {
		logger.Printf("Invalid user ID: %s", targetUserIDStr)
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	// Get current user ID from context
	currentUserID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("Invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	isFriend, err := h.service.CheckFriendship(r.Context(), currentUserID, targetUserID)
	if err != nil {
		logger.Printf("Failed to check friendship status: %v", err)
		http.Error(w, "failed to check friendship status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"is_friend": isFriend})
}

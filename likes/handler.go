package likes

import (
	"CarBN/common"
	"encoding/json"
	"net/http"
	"strconv"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateFeedItemLike(w http.ResponseWriter, r *http.Request) {
	feedItemIDStr := r.PathValue("feedItemId")
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	feedItemID, err := strconv.Atoi(feedItemIDStr)
	if err != nil {
		http.Error(w, "Invalid feed item ID", http.StatusBadRequest)
		return
	}

	like, err := h.service.CreateLike(r.Context(), userID, feedItemID, TargetTypeFeedItem)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(like)
}

func (h *Handler) DeleteFeedItemLike(w http.ResponseWriter, r *http.Request) {
	feedItemIDStr := r.PathValue("feedItemId")
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	feedItemID, err := strconv.Atoi(feedItemIDStr)
	if err != nil {
		http.Error(w, "Invalid feed item ID", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteLike(r.Context(), userID, feedItemID, TargetTypeFeedItem); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateUserCarLike(w http.ResponseWriter, r *http.Request) {
	userCarIDStr := r.PathValue("userCarId")
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	userCarID, err := strconv.Atoi(userCarIDStr)
	if err != nil {
		http.Error(w, "Invalid user car ID", http.StatusBadRequest)
		return
	}

	like, err := h.service.CreateLike(r.Context(), userID, userCarID, TargetTypeUserCar)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(like)
}

func (h *Handler) DeleteUserCarLike(w http.ResponseWriter, r *http.Request) {
	userCarIDStr := r.PathValue("userCarId")
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	userCarID, err := strconv.Atoi(userCarIDStr)
	if err != nil {
		http.Error(w, "Invalid user car ID", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteLike(r.Context(), userID, userCarID, TargetTypeUserCar); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetFeedItemLikes(w http.ResponseWriter, r *http.Request) {
	feedItemIDStr := r.PathValue("feedItemId")
	feedItemID, err := strconv.Atoi(feedItemIDStr)
	if err != nil {
		http.Error(w, "Invalid feed item ID", http.StatusBadRequest)
		return
	}

	cursor := r.URL.Query().Get("cursor")
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	likes, err := h.service.GetFeedItemLikes(r.Context(), feedItemID, cursor, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(likes)
}

func (h *Handler) GetUserCarLikes(w http.ResponseWriter, r *http.Request) {
	userCarIDStr := r.PathValue("userCarId")
	userCarID, err := strconv.Atoi(userCarIDStr)
	if err != nil {
		http.Error(w, "Invalid user car ID", http.StatusBadRequest)
		return
	}

	cursor := r.URL.Query().Get("cursor")
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	likes, err := h.service.GetUserCarLikes(r.Context(), userCarID, cursor, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(likes)
}

func (h *Handler) GetUserReceivedLikes(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.PathValue("userId")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	cursor := r.URL.Query().Get("cursor")
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	likes, err := h.service.GetUserReceivedLikes(r.Context(), userID, cursor, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(likes)
}

func (h *Handler) CheckUserCarLike(w http.ResponseWriter, r *http.Request) {
	userCarIDStr := r.PathValue("userCarId")
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	userCarID, err := strconv.Atoi(userCarIDStr)
	if err != nil {
		http.Error(w, "Invalid user car ID", http.StatusBadRequest)
		return
	}

	hasLiked, err := h.service.UserHasLiked(r.Context(), userID, userCarID, TargetTypeUserCar)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"liked": hasLiked})
}

func (h *Handler) GetUserCarLikesCount(w http.ResponseWriter, r *http.Request) {
	userCarIDStr := r.PathValue("userCarId")

	userCarID, err := strconv.Atoi(userCarIDStr)
	if err != nil {
		http.Error(w, "Invalid user car ID", http.StatusBadRequest)
		return
	}

	count, err := h.service.GetLikesCount(r.Context(), userCarID, TargetTypeUserCar)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}

func (h *Handler) CheckFeedItemLike(w http.ResponseWriter, r *http.Request) {
	feedItemIDStr := r.PathValue("feedItemId")
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	feedItemID, err := strconv.Atoi(feedItemIDStr)
	if err != nil {
		http.Error(w, "Invalid feed item ID", http.StatusBadRequest)
		return
	}

	hasLiked, err := h.service.UserHasLiked(r.Context(), userID, feedItemID, TargetTypeFeedItem)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"liked": hasLiked})
}

package user

import (
	"CarBN/common"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type HTTPHandler struct {
	service *Service
}

func NewHTTPHandler(s *Service) *HTTPHandler {
	return &HTTPHandler{service: s}
}

func (h *HTTPHandler) HandleGetCarCollection(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: GET car collection - Method: %s, Path: %s, Query: %s",
		r.Method, r.URL.Path, r.URL.RawQuery)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("Invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	requestedUserIDParam := r.PathValue("user_id")
	var requestedUserID int
	var err error
	if requestedUserIDParam != "" {
		requestedUserID, err = strconv.Atoi(requestedUserIDParam)
		if err != nil {
			http.Error(w, "invalid requested user ID", http.StatusBadRequest)
			return
		}
	} else {
		requestedUserID = userID
	}

	// Handle negative userID as current user
	if requestedUserID < 0 {
		requestedUserID = userID
	}

	// Handle pagination
	var limit, offset int
	limitParam := r.URL.Query().Get("limit")
	offsetParam := r.URL.Query().Get("offset")
	limit, err = strconv.Atoi(limitParam)
	if err != nil || limit < 1 {
		limit = 10
	}
	offset, err = strconv.Atoi(offsetParam)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Parse sort parameter
	sort := r.URL.Query().Get("sort")
	// Validate sort parameter (optional)
	validSorts := map[string]bool{
		"": true, "rarity": true, "date": true, "date_asc": true,
		"date_desc": true, "name": true, "name_asc": true, "name_desc": true,
	}
	if !validSorts[sort] {
		sort = "" // Default to no specific sort if invalid
	}

	logger.Printf("Car collection request parameters - UserID: %d, RequestedUserID: %d, Limit: %d, Offset: %d, Sort: %s",
		userID, requestedUserID, limit, offset, sort)

	cars, err := h.service.GetCarCollection(r.Context(), userID, requestedUserID, limit, offset, sort)
	if err != nil {
		logger.Printf("Failed to get car collection: %v", err)
		http.Error(w, "failed to retrieve cars", http.StatusInternalServerError)
		return
	}

	// if len(cars) == 0 {
	// 	logger.Printf("No cars found for user %d", requestedUserID)
	// 	http.Error(w, "no cars found", http.StatusNotFound)
	// 	return
	// }

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(cars); err != nil {
		logger.Printf("Failed to encode cars response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully returned %d cars for user %d", len(cars), requestedUserID)
}

func (h *HTTPHandler) HandleGetSpecificUserCars(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: GET specific user cars - Method: %s, Path: %s, Query: %s",
		r.Method, r.URL.Path, r.URL.RawQuery)

	// Parse car IDs from query parameter
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		http.Error(w, "ids parameter is required", http.StatusBadRequest)
		return
	}

	idStrs := strings.Split(idsParam, ",")
	userCarIDs := make([]int, 0, len(idStrs))
	for _, idStr := range idStrs {
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil {
			http.Error(w, "invalid car ID format", http.StatusBadRequest)
			return
		}
		userCarIDs = append(userCarIDs, id)
	}

	cars, err := h.service.GetSpecificUserCars(r.Context(), userCarIDs)
	if err != nil {
		logger.Printf("Failed to get specific user cars: %v", err)
		http.Error(w, "failed to retrieve cars", http.StatusInternalServerError)
		return
	}

	if len(cars) == 0 {
		logger.Printf("No cars found for IDs: %v", userCarIDs)
		http.Error(w, "no cars found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(cars); err != nil {
		logger.Printf("Failed to encode cars response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully returned %d specific user cars", len(cars))
}

func (h *HTTPHandler) HandleGetPendingFriendRequests(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: GET pending friend requests - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("Invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	logger.Printf("Fetching pending friend requests for user %d", userID)
	requests, err := h.service.GetPendingFriendRequests(r.Context(), userID)
	if err != nil {
		logger.Printf("Failed to fetch friend requests: %v", err)
		http.Error(w, "failed to fetch requests", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(requests); err != nil {
		logger.Printf("Failed to encode friend requests response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully returned %d pending friend requests for user %d", len(requests), userID)
}

func (h *HTTPHandler) HandleUploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: POST profile picture - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		logger.Printf("Invalid user ID in context")
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	var req struct {
		Base64Image string `json:"base64_image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Failed to decode request body: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	logger.Printf("Processing profile picture upload for user %d - Payload size: %d bytes",
		userID, len(req.Base64Image))

	// Validate and decode base64 image
	if req.Base64Image == "" {
		http.Error(w, "missing base64_image", http.StatusBadRequest)
		return
	}

	// Remove data URL prefix if present
	base64Data := req.Base64Image
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		logger.Printf("Failed to decode base64 image: %v", err)
		http.Error(w, "invalid base64 image", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateProfilePicture(r.Context(), userID, imageData); err != nil {
		if err.Error() == "image must be 512x512 pixels" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		logger.Printf("Failed to update profile picture: %v", err)
		http.Error(w, "failed to update profile picture", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

	logger.Printf("Successfully updated profile picture for user %d", userID)
}

func (h *HTTPHandler) HandleGetUserProfile(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: GET user profile - Method: %s, Path: %s", r.Method, r.URL.Path)

	currentUserID := r.Context().Value(common.UserIDCtxKey).(int)
	requestedUserIDStr := r.PathValue("user_id")
	requestedUserID, err := strconv.Atoi(requestedUserIDStr)
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	logger.Printf("Profile request parameters - CurrentUserID from context: %d, RequestedUserID: %d",
		currentUserID, requestedUserID)
	if requestedUserID < 0 {
		requestedUserID = currentUserID
		logger.Printf("Negative user ID provided, using current user ID: %d", requestedUserID)
	}

	user, err := h.service.GetUserInfo(r.Context(), requestedUserID, currentUserID)
	if err != nil {
		logger.Printf("Failed to get user info: %v", err)
		http.Error(w, fmt.Sprintf("user not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(user); err != nil {
		logger.Printf("Failed to encode response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully returned profile info for user %d", requestedUserID)
}

func (h *HTTPHandler) HandleSearchUsers(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: GET search users - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID := r.Context().Value(common.UserIDCtxKey).(int)

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "search query is required", http.StatusBadRequest)
		return
	}

	users, err := h.service.SearchUsers(r.Context(), query, userID)
	if err != nil {
		logger.Printf("Failed to search users: %v", err)
		http.Error(w, "failed to search users", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(users); err != nil {
		logger.Printf("Failed to encode search results: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully returned %d users for search query: %s", len(users), query)
}

// HandleSellCar handles the request to sell a car from the user's collection
func (h *HTTPHandler) HandleSellCar(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: POST sell car - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID := r.Context().Value(common.UserIDCtxKey).(int)

	userCarID, err := strconv.Atoi(r.PathValue("user_car_id"))
	if err != nil {
		http.Error(w, "invalid car ID", http.StatusBadRequest)
		return
	}

	currencyEarned, err := h.service.SellCar(r.Context(), userID, userCarID)
	if err != nil {
		logger.Printf("Failed to sell car: %v", err)
		if err.Error() == "car not found or not owned by user" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to sell car", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":         "car sold successfully",
		"currency_earned": currencyEarned,
	})
}

// HandleUpgradeCarImage handles the request to upgrade a car's image to premium
func (h *HTTPHandler) HandleUpgradeCarImage(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: POST upgrade car image - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID := r.Context().Value(common.UserIDCtxKey).(int)
	userCarID, err := strconv.Atoi(r.PathValue("user_car_id"))
	if err != nil {
		http.Error(w, "invalid car ID", http.StatusBadRequest)
		return
	}

	result, err := h.service.UpgradeCarImage(r.Context(), userID, userCarID)
	if err != nil {
		logger.Printf("Failed to upgrade car image: %v", err)
		if err.Error() == "active subscription required" || err.Error() == "insufficient currency" {
			http.Error(w, err.Error(), http.StatusPaymentRequired)
			return
		}
		if err.Error() == "car not found or not owned by user" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to upgrade car image", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":            "car image upgraded successfully",
		"remaining_currency": result.RemainingCurrency,
		"high_res_image":     result.HighResImage,
		"low_res_image":      result.LowResImage,
	})
}

// HandleRevertCarImage handles the request to revert a car's image to original
func (h *HTTPHandler) HandleRevertCarImage(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: POST revert car image - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID := r.Context().Value(common.UserIDCtxKey).(int)
	userCarID, err := strconv.Atoi(r.PathValue("user_car_id"))
	if err != nil {
		http.Error(w, "invalid car ID", http.StatusBadRequest)
		return
	}

	result, err := h.service.RevertCarImage(r.Context(), userID, userCarID)
	if err != nil {
		logger.Printf("Failed to revert car image: %v", err)
		if err.Error() == "car not found or not owned by user" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to revert car image", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":        "car image reverted successfully",
		"high_res_image": result.HighResImage,
		"low_res_image":  result.LowResImage,
	})
}

func (h *HTTPHandler) HandleGetCarUpgrades(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(common.UserIDCtxKey).(int)
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	userCarID, err := strconv.Atoi(r.PathValue("user_car_id"))
	if err != nil {
		http.Error(w, "invalid car ID", http.StatusBadRequest)
		return
	}

	upgrades, err := h.service.GetCarUpgrades(r.Context(), userID, userCarID)
	if err != nil {
		logger.Printf("Failed to get car upgrades: %v", err)
		http.Error(w, "failed to retrieve upgrades", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(upgrades); err != nil {
		logger.Printf("Failed to encode upgrades response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleUpdateDisplayName handles requests to update a user's display name
func (h *HTTPHandler) HandleUpdateDisplayName(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: POST update display name - Method: %s, Path: %s", r.Method, r.URL.Path)

	userID := r.Context().Value(common.UserIDCtxKey).(int)

	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Failed to decode request body: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateDisplayName(r.Context(), userID, req.DisplayName); err != nil {
		if err.Error() == "display name must be between 3 and 50 characters" ||
			err.Error() == "display name contains inappropriate content" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		logger.Printf("Failed to update display name: %v", err)
		http.Error(w, "failed to update display name", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleCreateShareLink creates a shareable link for any car
func (h *HTTPHandler) HandleCreateShareLink(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: POST create share link - Method: %s, Path: %s", r.Method, r.URL.Path)

	// Get the requesting user's ID (not necessarily the car owner)
	requestingUserID := r.Context().Value(common.UserIDCtxKey).(int)

	userCarID, err := strconv.Atoi(r.PathValue("user_car_id"))
	if err != nil {
		http.Error(w, "invalid car ID", http.StatusBadRequest)
		return
	}

	// Generate a unique share token for this car (ownership check removed)
	shareToken, err := h.service.CreateShareLink(r.Context(), requestingUserID, userCarID)
	if err != nil {
		if err.Error() == "car not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		logger.Printf("Failed to create share link: %v", err)
		http.Error(w, "failed to create share link", http.StatusInternalServerError)
		return
	}

	// Update share URL to use "/share/token/" prefix
	baseURL := h.service.config.BaseURL
	if baseURL == "" {
		baseURL = "https://carbn-test-01.mzinck.com" // Default URL
	}
	shareURL := fmt.Sprintf("%s/share/token/%s", baseURL, shareToken)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"share_token": shareToken,
		"share_url":   shareURL,
	})

	logger.Printf("Successfully created share link for car %d: %s", userCarID, shareToken)
}

// HandleGetSharedCar gets the publicly available car data for a shared car
func (h *HTTPHandler) HandleGetSharedCar(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: GET shared car - Method: %s, Path: %s", r.Method, r.URL.Path)

	shareToken := r.PathValue("share_token")
	if shareToken == "" {
		http.Error(w, "share token is required", http.StatusBadRequest)
		return
	}

	// Get the shared car data
	sharedCar, err := h.service.GetSharedCarByToken(r.Context(), shareToken)
	if err != nil {
		if err.Error() == "share token not found or expired" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		logger.Printf("Failed to get shared car: %v", err)
		http.Error(w, "failed to retrieve shared car", http.StatusInternalServerError)
		return
	}

	// Increment view count for analytics using a background context
	// so it's not canceled when the request ends
	go func() {
		// Create a new background context with the logger
		bgCtx := context.WithValue(context.Background(), common.LoggerCtxKey, logger)
		if err := h.service.IncrementShareViews(bgCtx, shareToken); err != nil {
			logger.Printf("Background view count update failed: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sharedCar); err != nil {
		logger.Printf("Failed to encode shared car response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	logger.Printf("Successfully served shared car for token: %s", shareToken)
}

// HandleServeSharePage serves the HTML page for shared cars
func (h *HTTPHandler) HandleServeSharePage(w http.ResponseWriter, r *http.Request) {
	// Read the HTML file
	content, err := os.ReadFile("shared_car/index.html")
	if err != nil {
		http.Error(w, "Error loading page", http.StatusInternalServerError)
		return
	}

	// Serve the HTML content
	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

// HandleServeShareStaticFiles serves static files (CSS, JS) for the shared car page
func (h *HTTPHandler) HandleServeShareStaticFiles(w http.ResponseWriter, r *http.Request) {
	// Get the filename from the URL path
	filename := strings.TrimPrefix(r.URL.Path, "/share/static/")

	// Map the requested file to the actual file path
	var filePath string
	switch filename {
	case "styles.css":
		filePath = "shared_car/styles.css"
	case "script.js":
		filePath = "shared_car/script.js"
	default:
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Set appropriate content type
	if strings.HasSuffix(filename, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(filename, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	}

	// Serve the file
	http.ServeFile(w, r, filePath)
}

// HandleDeleteAccount handles the request to delete a user's account
func (h *HTTPHandler) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Request received: DELETE account - Method: %s, Path: %s", r.Method, r.URL.Path)

	// Get user ID from context (set by auth middleware)
	userID := r.Context().Value(common.UserIDCtxKey).(int)

	// Parse and validate request body
	var req struct {
		Confirm bool `json:"confirm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Failed to parse request body: %v", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Confirm deletion is explicitly set to true
	if !req.Confirm {
		logger.Printf("Account deletion not confirmed for user %d", userID)
		http.Error(w, "Deletion not confirmed", http.StatusBadRequest)
		return
	}

	// Call service method to delete the account
	if err := h.service.DeleteAccount(r.Context(), userID); err != nil {
		logger.Printf("Failed to delete account for user %d: %v", userID, err)
		http.Error(w, "Failed to delete account", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Account successfully deleted",
	})
}

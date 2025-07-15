package subscription

import (
	"CarBN/common"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

type SubscriptionHandler struct {
	service *SubscriptionService
}

func NewSubscriptionHandler(service *SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{service: service}
}

func (h *SubscriptionHandler) HandleGetUserSubscription(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	subscription, err := h.service.GetUserSubscription(r.Context(), userID)
	if err != nil {
		logger.Printf("Failed to get user subscription: %v", err)
		http.Error(w, "failed to get subscription info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subscription)
}

func (h *SubscriptionHandler) HandleGetUserSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	requestedUserID, err := strconv.Atoi(r.PathValue("user_id"))
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	isActive, err := h.service.HasActiveSubscription(r.Context(), requestedUserID)
	if err != nil {
		logger.Printf("Failed to get subscription status: %v", err)
		http.Error(w, "failed to get subscription status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"is_active": isActive})
}

func (h *SubscriptionHandler) HandlePurchaseSubscription(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	var request PurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logger.Printf("Failed to parse purchase request: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// logger.Printf("Received receipt data (first 20 chars): %s", request.ReceiptData[:20])

	// Validate platform
	if request.Platform != "apple" && request.Platform != "google" {
		http.Error(w, "invalid platform, must be 'apple' or 'google'", http.StatusBadRequest)
		return
	}

	// Set purchase type to subscription
	request.PurchaseType = PurchaseTypeSubscription

	// Process purchase
	response, err := h.service.ProcessPurchase(r.Context(), userID, &request)
	if err != nil {
		logger.Printf("Failed to process subscription purchase: %v", err)
		if response != nil {
			// Return structured error response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "failed to process purchase", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandlePurchaseScanPack processes a one-time scan pack purchase
func (h *SubscriptionHandler) HandlePurchaseScanPack(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	userID, ok := r.Context().Value(common.UserIDCtxKey).(int)
	if !ok {
		http.Error(w, "invalid user ID in context", http.StatusInternalServerError)
		return
	}

	var request PurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logger.Printf("Failed to parse scan pack purchase request: %v", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate platform
	if request.Platform != "apple" && request.Platform != "google" {
		http.Error(w, "invalid platform, must be 'apple' or 'google'", http.StatusBadRequest)
		return
	}

	// Set purchase type to scan pack
	request.PurchaseType = PurchaseTypeScanPack

	// Process purchase
	response, err := h.service.ProcessPurchase(r.Context(), userID, &request)
	if err != nil {
		logger.Printf("Failed to process scan pack purchase: %v", err)
		if response != nil {
			// Return structured error response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "failed to process purchase", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *SubscriptionHandler) HandleGetSubscriptionProducts(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

	// Get platform filter from query parameter (optional)
	platform := r.URL.Query().Get("platform")

	// Get product type filter from query parameter (optional)
	productType := r.URL.Query().Get("type")

	products, err := h.service.GetSubscriptionProducts(r.Context(), platform, productType)
	if err != nil {
		logger.Printf("Failed to get subscription products: %v", err)
		http.Error(w, "failed to get subscription products", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

// HandleAppStoreNotification processes App Store Server Notifications V2
func (h *SubscriptionHandler) HandleAppStoreNotification(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Received App Store server notification")

	// Parse the notification payload
	var notification AppStoreNotificationPayload
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		logger.Printf("Failed to decode notification: %v", err)
		http.Error(w, "invalid notification payload", http.StatusBadRequest)
		return
	}

	logger.Printf("Received notification type: %s, subtype: %s, UUID: %s",
		notification.NotificationType, notification.Subtype, notification.NotificationUUID)

	// Add validation for empty notifications
	if notification.NotificationType == "" || notification.NotificationUUID == "" {
		logger.Printf("Received empty notification, ignoring")
		w.WriteHeader(http.StatusOK) // Still return OK to Apple
		return
	}

	// Verify bundle ID
	expectedBundleID := "com.mzinck.CarBN"
	if notification.Data.BundleId != "" && notification.Data.BundleId != expectedBundleID {
		logger.Printf("Invalid bundle ID: %s, expected: %s",
			notification.Data.BundleId, expectedBundleID)
		http.Error(w, "invalid bundle ID", http.StatusBadRequest)
		return
	}

	// Log the notification to database and check if already processed
	alreadyProcessed, err := h.logNotificationToDatabase(r.Context(), &notification)
	if err != nil {
		logger.Printf("Failed to log notification to database: %v", err)
	}

	// Skip processing if already handled
	if alreadyProcessed {
		logger.Printf("Notification %s already processed, skipping processing",
			notification.NotificationUUID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process the notification asynchronously
	go func() {
		ctx := context.Background()
		// Add logger to context for async processing
		ctx = context.WithValue(ctx, common.LoggerCtxKey, logger)

		if err := h.service.ProcessAppStoreNotification(ctx, &notification); err != nil {
			logger.Printf("Failed to process App Store notification: %v", err)
			// Log the error to the database for later analysis
			h.logNotificationError(ctx, notification.NotificationUUID, err)
		}
	}()

	// Always respond with 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)
}

// logNotificationToDatabase records the notification in the database for audit purposes
func (h *SubscriptionHandler) logNotificationToDatabase(ctx context.Context, notification *AppStoreNotificationPayload) (bool, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	// Serialize the full payload for storage
	rawPayload, err := json.Marshal(notification)
	if err != nil {
		return false, fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Check if we've already processed this notification (idempotency)
	var exists bool
	err = h.service.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM app_store_notifications
			WHERE notification_uuid = $1
		)
	`, notification.NotificationUUID).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check for existing notification: %w", err)
	}

	if exists {
		logger.Printf("Notification %s already processed, skipping", notification.NotificationUUID)
		return true, nil
	}

	// Insert the notification record
	_, err = h.service.db.Exec(ctx, `
		INSERT INTO app_store_notifications (
			notification_uuid, notification_type, subtype, signed_date, raw_payload
		) VALUES ($1, $2, $3, $4, $5)
	`, notification.NotificationUUID,
		notification.NotificationType,
		notification.Subtype,
		notification.SignedDate,
		rawPayload)

	if err != nil {
		return false, fmt.Errorf("failed to insert notification record: %w", err)
	}

	return false, nil
}

// logNotificationError updates the notification record with error information
func (h *SubscriptionHandler) logNotificationError(ctx context.Context, notificationUUID string, err error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	_, dbErr := h.service.db.Exec(ctx, `
		UPDATE app_store_notifications 
		SET error_message = $1
		WHERE notification_uuid = $2
	`, err.Error(), notificationUUID)

	if dbErr != nil {
		logger.Printf("Failed to log notification error to database: %v", dbErr)
	}
}

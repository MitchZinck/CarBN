package subscription

import (
	"CarBN/common"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Purchase types
const (
	PurchaseTypeSubscription = "subscription"
	PurchaseTypeScanPack     = "scanpack"
)

type SubscriptionService struct {
	db                *pgxpool.Pool
	httpClient        *http.Client
	appleIssuerId     string
	appleKeyId        string
	applePrivateKey   []byte
	appleSharedSecret string
	cacheTTL          time.Duration
	transactionCache  *TransactionCache
}

// Cache for storing transaction data
type TransactionCache struct {
	cache map[string]*AppStoreTransaction
	mu    sync.RWMutex
}

// App Store Server API types
type AppStoreTransaction struct {
	TransactionId              string    `json:"transactionId"`
	OriginalTransactionId      string    `json:"originalTransactionId"`
	WebOrderLineItemId         string    `json:"webOrderLineItemId"`
	BundleId                   string    `json:"bundleId"`
	ProductId                  string    `json:"productId"`
	SubscriptionGroupId        string    `json:"subscriptionGroupId"`
	PurchaseDate               int64     `json:"purchaseDate"`
	OriginalPurchaseDate       int64     `json:"originalPurchaseDate"`
	ExpiresDate                int64     `json:"expiresDate"`
	Quantity                   int       `json:"quantity"`
	Type                       string    `json:"type"`
	InAppOwnershipType         string    `json:"inAppOwnershipType"`
	SignedDate                 int64     `json:"signedDate"`
	Environment                string    `json:"environment"`
	Status                     int       `json:"status"`
	ParsedPurchaseDate         time.Time `json:"-"`
	ParsedExpiresDate          time.Time `json:"-"`
	ParsedOriginalPurchaseDate time.Time `json:"-"`

	// JWT required fields
	jwt.RegisteredClaims
}

type AppStoreNotificationPayload struct {
	NotificationType string                   `json:"notificationType"`
	Subtype          string                   `json:"subtype"`
	NotificationUUID string                   `json:"notificationUUID"`
	Data             AppStoreNotificationData `json:"data"`
	Version          string                   `json:"version"`
	SignedDate       int64                    `json:"signedDate"`
}

type AppStoreNotificationData struct {
	AppAppleId            int64  `json:"appAppleId"`
	BundleId              string `json:"bundleId"`
	BundleVersion         string `json:"bundleVersion"`
	Environment           string `json:"environment"`
	SignedTransactionInfo string `json:"signedTransactionInfo"`
	SignedRenewalInfo     string `json:"signedRenewalInfo"`
}

type AppStoreErrorResponse struct {
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

// Error implements the error interface
func (e *AppStoreErrorResponse) Error() string {
	return fmt.Sprintf("Apple App Store error %d: %s", e.ErrorCode, e.ErrorMessage)
}

// Legacy Apple receipt verification types - kept for backward compatibility
type AppleReceipt struct {
	ReceiptData            string `json:"receipt-data"`
	Password               string `json:"password,omitempty"`
	ExcludeOldTransactions bool   `json:"exclude-old-transactions"`
}

type AppleVerifyResponse struct {
	Status             int                `json:"status"`
	Environment        string             `json:"environment"`
	LatestReceiptInfo  []AppleReceiptInfo `json:"latest_receipt_info"`
	LatestReceipt      string             `json:"latest_receipt"`
	PendingRenewalInfo []AppleRenewalInfo `json:"pending_renewal_info"`
}

type AppleReceiptInfo struct {
	TransactionID               string `json:"transaction_id"`
	OriginalTransactionID       string `json:"original_transaction_id"`
	ProductID                   string `json:"product_id"`
	PurchaseDateMs              string `json:"purchase_date_ms"`
	ExpiresDateMs               string `json:"expires_date_ms"`
	SubscriptionGroupIdentifier string `json:"subscription_group_identifier"`
}

type AppleRenewalInfo struct {
	OriginalTransactionID string `json:"original_transaction_id"`
	ProductID             string `json:"product_id"`
	AutoRenewStatus       string `json:"auto_renew_status"`
}

// PurchaseRequest contains the data needed to process a subscription purchase
type PurchaseRequest struct {
	ReceiptData   string      `json:"receipt_data,omitempty"`
	TransactionID interface{} `json:"transaction_id,omitempty"` // Can be number or string
	Platform      string      `json:"platform"`                 // "apple" or "google"
	PurchaseType  string      `json:"purchase_type,omitempty"`  // "subscription" or "scanpack" (default: "subscription")
}

// PurchaseResponse contains the result of a subscription purchase
type PurchaseResponse struct {
	Success      bool              `json:"success"`
	Message      string            `json:"message,omitempty"`
	Subscription *SubscriptionInfo `json:"subscription,omitempty"`
}

type SubscriptionInfo struct {
	IsActive             bool    `json:"is_active"`
	Tier                 string  `json:"tier"`
	SubscriptionStart    *string `json:"subscription_start,omitempty"`
	SubscriptionEnd      *string `json:"subscription_end,omitempty"`
	ScanCreditsRemaining int     `json:"scan_credits_remaining"`
}

type SubscriptionProduct struct {
	ID           int    `json:"id"`
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	Tier         string `json:"tier"`
	Platform     string `json:"platform"`
	DurationDays int    `json:"duration_days"`
	ScanCredits  int    `json:"scan_credits"`
	Type         string `json:"type"` // "subscription" or "scanpack"
}

func NewSubscriptionService(db *pgxpool.Pool) *SubscriptionService {
	var privateKey []byte
	privateKeyContent := os.Getenv("APPLE_PRIVATE_KEY")
	if privateKeyContent != "" {
		privateKey = []byte(privateKeyContent)
		log.Printf("Successfully loaded Apple private key from env, length: %d bytes", len(privateKey))
	} else {
		log.Printf("Warning: APPLE_PRIVATE_KEY environment variable is not set")
	}

	// Log important configuration details
	log.Printf("Initializing SubscriptionService")
	log.Printf("Apple issuer ID configured: %t", os.Getenv("APPLE_ISSUER_ID") != "")
	log.Printf("Apple key ID configured: %t", os.Getenv("APPLE_KEY_ID") != "")
	log.Printf("Apple private key configured: %t", len(privateKey) > 0)
	log.Printf("Apple shared secret configured: %t", os.Getenv("APPLE_SHARED_SECRET_KEY") != "")

	return &SubscriptionService{
		db: db,
		httpClient: &http.Client{
			Timeout: 240 * time.Second,
		},
		appleIssuerId:     os.Getenv("APPLE_ISSUER_ID"),
		appleKeyId:        os.Getenv("APPLE_KEY_ID"),
		applePrivateKey:   privateKey,
		appleSharedSecret: os.Getenv("APPLE_SHARED_SECRET_KEY"),
		cacheTTL:          time.Hour * 1, // Cache transactions for 1 hour
		transactionCache: &TransactionCache{
			cache: make(map[string]*AppStoreTransaction),
			mu:    sync.RWMutex{},
		},
	}
}

func (s *SubscriptionService) GetUserSubscription(ctx context.Context, userID int) (*SubscriptionInfo, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching subscription info for user %d", userID)

	var info SubscriptionInfo
	var startTime, endTime *time.Time
	var needsUpdate bool

	err := s.db.QueryRow(ctx, `
		SELECT 
			COALESCE(is_active, false),
			COALESCE(tier, 'none'),
			subscription_start,
			subscription_end,
			COALESCE(scan_credits_remaining, 0)
		FROM user_subscriptions
		WHERE user_id = $1
	`, userID).Scan(&info.IsActive, &info.Tier, &startTime, &endTime, &info.ScanCreditsRemaining)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No subscription found for user %d, returning defaults", userID)
			// If no subscription found, return default values
			return &SubscriptionInfo{
				IsActive:             false,
				Tier:                 "none",
				ScanCreditsRemaining: 6, // Default credits for new users
			}, nil
		}
		logger.Printf("Error fetching subscription for user %d: %v", userID, err)
		return nil, err
	}

	// Check if subscription has expired but is still marked as active
	if info.IsActive && endTime != nil && time.Now().After(*endTime) {
		logger.Printf("Subscription for user %d has expired at %s but is still marked as active. Updating status.",
			userID, endTime.Format(time.RFC3339))
		info.IsActive = false
		needsUpdate = true
	}

	logger.Printf("Retrieved subscription for user %d: active=%t, tier=%s, credits=%d",
		userID, info.IsActive, info.Tier, info.ScanCreditsRemaining)

	if startTime != nil {
		startStr := common.FormatTimestamp(*startTime)
		info.SubscriptionStart = &startStr
		logger.Printf("User %d subscription start: %s", userID, startStr)
	}
	if endTime != nil {
		endStr := common.FormatTimestamp(*endTime)
		info.SubscriptionEnd = &endStr
		logger.Printf("User %d subscription end: %s", userID, endStr)
	}

	// Update the database if needed
	if needsUpdate {
		logger.Printf("Updating expired subscription status for user %d", userID)
		_, err := s.db.Exec(ctx, `
			UPDATE user_subscriptions
			SET is_active = false
			WHERE user_id = $1
		`, userID)

		if err != nil {
			logger.Printf("Error updating subscription status for user %d: %v", userID, err)
			// Don't return error as we still want to return the subscription info
		}
	}

	return &info, nil
}

func (s *SubscriptionService) HasActiveSubscription(ctx context.Context, userID int) (bool, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Checking active subscription status for user %d", userID)

	var isActive bool
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(is_active, false)
		FROM user_subscriptions
		WHERE user_id = $1 AND is_active = true 
		AND (subscription_end IS NULL OR subscription_end > NOW())
	`, userID).Scan(&isActive)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No active subscription found for user %d", userID)
			return false, nil // No subscription found
		}
		logger.Printf("Error checking subscription status for user %d: %v", userID, err)
		return false, err
	}

	logger.Printf("User %d has active subscription: %t", userID, isActive)
	return isActive, nil
}

func (s *SubscriptionService) HasScanCredits(ctx context.Context, userID int) (bool, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Checking scan credits for user %d", userID)

	var credits int
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(scan_credits_remaining, 6)
		FROM user_subscriptions
		WHERE user_id = $1
	`, userID).Scan(&credits)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No subscription record found for user %d, assuming default credits", userID)
			return true, nil // No subscription found, assume default credits
		}
		logger.Printf("Error checking scan credits for user %d: %v", userID, err)
		return false, err
	}

	logger.Printf("User %d has %d scan credits remaining", userID, credits)
	return credits > 0, nil
}

func (s *SubscriptionService) DeductScanCredit(ctx context.Context, userID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	result, err := s.db.Exec(ctx, `
		UPDATE user_subscriptions
		SET scan_credits_remaining = scan_credits_remaining - 1
		WHERE user_id = $1 AND scan_credits_remaining > 0
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to deduct scan credit: %w", err)
	}

	if result.RowsAffected() == 0 {
		// If no row was updated, insert a new subscription row with default credits - 1
		_, err = s.db.Exec(ctx, `
			INSERT INTO user_subscriptions (user_id, scan_credits_remaining, tier)
			VALUES ($1, 5, 'none')
			ON CONFLICT (user_id) DO NOTHING
		`, userID)
		if err != nil {
			return fmt.Errorf("failed to create initial subscription: %w", err)
		}
	}

	logger.Printf("Successfully deducted scan credit for user %d", userID)
	return nil
}

// ProcessPurchase handles subscription purchase from Apple or Google
func (s *SubscriptionService) ProcessPurchase(ctx context.Context, userID int, request *PurchaseRequest) (*PurchaseResponse, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	// Default to subscription if purchase type not specified
	if request.PurchaseType == "" {
		request.PurchaseType = PurchaseTypeSubscription
	}

	logger.Printf("Processing %s purchase for user %d on platform %s",
		request.PurchaseType, userID, request.Platform)

	if request.Platform == "apple" {
		// Handle transaction ID provided directly
		if request.TransactionID != nil {
			var transactionID string

			// Handle different possible types for TransactionID
			switch v := request.TransactionID.(type) {
			case float64:
				transactionID = strconv.FormatInt(int64(v), 10)
			case string:
				transactionID = v
			default:
				transactionID = fmt.Sprintf("%v", v) // Fallback conversion
			}

			logger.Printf("Using transaction ID: %s", transactionID)
			return s.processApplePurchase(ctx, userID, transactionID, request.PurchaseType)
		}

		// Legacy receipt processing if TransactionID not provided
		if request.ReceiptData != "" {
			return s.processApplePurchase(ctx, userID, request.ReceiptData, request.PurchaseType)
		}

		return nil, errors.New("missing transaction_id or receipt_data for Apple purchase")
	}

	// Handle other platforms...

	return nil, fmt.Errorf("unsupported platform: %s", request.Platform)
}

// processApplePurchase verifies and processes Apple purchases (subscriptions or scan packs)
func (s *SubscriptionService) processApplePurchase(ctx context.Context, userID int, receiptData string, purchaseType string) (*PurchaseResponse, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing Apple %s purchase for user %d", purchaseType, userID)

	var transactionId string

	// First check if receiptData is already a transaction ID (all digits)
	if _, err := strconv.ParseInt(receiptData, 10, 64); err == nil {
		// This is a numeric string, likely a transaction ID
		transactionId = receiptData
		logger.Printf("Using provided transaction ID: %s", transactionId)
	} else {
		// Try to extract transaction ID from JSON if it's not a direct ID
		var jsonReceipt map[string]interface{}
		if err := json.Unmarshal([]byte(receiptData), &jsonReceipt); err == nil {
			// This appears to be JSON format
			if txId, ok := jsonReceipt["transaction_id"].(string); ok && txId != "" {
				transactionId = txId
				logger.Printf("Extracted transaction ID from JSON receipt: %s", transactionId)
			} else if txIdNum, ok := jsonReceipt["transaction_id"].(float64); ok {
				// Handle numeric transaction ID
				transactionId = strconv.FormatInt(int64(txIdNum), 10)
				logger.Printf("Extracted numeric transaction ID from JSON receipt: %s", transactionId)
			} else if txId, ok := jsonReceipt["transactionId"].(string); ok && txId != "" {
				transactionId = txId
				logger.Printf("Extracted transaction ID from JSON receipt: %s", transactionId)
			}
		}
	}

	var transaction *AppStoreTransaction
	var product SubscriptionProduct

	// If we have a transaction ID, use the App Store Server API
	if transactionId != "" {
		// Get transaction from App Store Server API
		var err error
		transaction, err = s.getTransactionById(ctx, transactionId)
		if err != nil {
			logger.Printf("Failed to get transaction from App Store API: %v", err)
			return &PurchaseResponse{
				Success: false,
				Message: "Transaction verification failed",
			}, fmt.Errorf("transaction verification failed: %w", err)
		}

		// Add debug logging for the product ID
		logger.Printf("Looking up product with ID: %s", transaction.ProductId)

		// Find the product in our database
		err = s.db.QueryRow(ctx, `
            SELECT id, product_id, name, tier, platform, duration_days, scan_credits, type
            FROM subscription_products
            WHERE product_id = $1 AND platform = 'apple'
        `, transaction.ProductId).Scan(
			&product.ID,
			&product.ProductID,
			&product.Name,
			&product.Tier,
			&product.Platform,
			&product.DurationDays,
			&product.ScanCredits,
			&product.Type,
		)

		if err != nil {
			logger.Printf("Product not found: %s", transaction.ProductId)
			// Dump the transaction details for debugging
			jsonData, _ := json.Marshal(transaction)
			logger.Printf("Transaction details: %s", string(jsonData))
			return &PurchaseResponse{
				Success: false,
				Message: "Subscription product not recognized",
			}, fmt.Errorf("product not found: %s", transaction.ProductId)
		}

		// Verify the purchase type matches
		if product.Type != purchaseType {
			logger.Printf("Product type mismatch: expected %s, got %s", purchaseType, product.Type)
			return &PurchaseResponse{
				Success: false,
				Message: fmt.Sprintf("Invalid product type: expected %s", purchaseType),
			}, fmt.Errorf("product type mismatch: %s", product.Type)
		}
	} else {
		// Fall back to legacy verification for backward compatibility
		logger.Printf("No transaction ID found, falling back to legacy receipt verification")

		// Verify the receipt with Apple (legacy method)
		verifyResp, err := s.verifyAppleReceipt(ctx, receiptData)
		if err != nil {
			logger.Printf("Apple receipt verification failed: %v", err)
			return &PurchaseResponse{
				Success: false,
				Message: "Receipt verification failed",
			}, fmt.Errorf("receipt verification failed: %w", err)
		}

		// Check status (0 = success)
		if verifyResp.Status != 0 {
			logger.Printf("Apple receipt invalid, status: %d", verifyResp.Status)
			return &PurchaseResponse{
				Success: false,
				Message: fmt.Sprintf("Invalid receipt, status: %d", verifyResp.Status),
			}, fmt.Errorf("invalid receipt, status: %d", verifyResp.Status)
		}

		// Extract latest receipt info
		if len(verifyResp.LatestReceiptInfo) == 0 {
			logger.Printf("No receipt info found in response")
			return &PurchaseResponse{
				Success: false,
				Message: "No subscription details found in receipt",
			}, errors.New("no subscription details in receipt")
		}

		// Get latest receipt info (sorted by purchase date)
		latestInfo := verifyResp.LatestReceiptInfo[len(verifyResp.LatestReceiptInfo)-1]
		transactionId = latestInfo.TransactionID

		// Find the product in our database
		err = s.db.QueryRow(ctx, `
			SELECT id, product_id, name, tier, platform, duration_days, scan_credits, type
			FROM subscription_products
			WHERE product_id = $1 AND platform = 'apple'
		`, latestInfo.ProductID).Scan(
			&product.ID,
			&product.ProductID,
			&product.Name,
			&product.Tier,
			&product.Platform,
			&product.DurationDays,
			&product.ScanCredits,
			&product.Type,
		)

		if err != nil {
			logger.Printf("Product not found: %s", latestInfo.ProductID)
			return &PurchaseResponse{
				Success: false,
				Message: "Product not recognized",
			}, fmt.Errorf("product not found: %s", latestInfo.ProductID)
		}

		// Verify the purchase type matches
		if product.Type != purchaseType {
			logger.Printf("Product type mismatch: expected %s, got %s", purchaseType, product.Type)
			return &PurchaseResponse{
				Success: false,
				Message: fmt.Sprintf("Invalid product type: expected %s", purchaseType),
			}, fmt.Errorf("product type mismatch: %s", product.Type)
		}

		// Now get the transaction using the App Store Server API for consistent processing
		transaction, err = s.getTransactionById(ctx, transactionId)
		if err != nil {
			logger.Printf("Failed to get transaction after receipt verification: %v", err)

			// If API call fails, use the data from the receipt
			purchaseTimeMs, err := strconv.ParseInt(latestInfo.PurchaseDateMs, 10, 64)
			if err != nil {
				return &PurchaseResponse{
					Success: false,
					Message: "Invalid purchase date in receipt",
				}, fmt.Errorf("invalid purchase date: %w", err)
			}

			// Create a synthetic transaction from legacy receipt data
			transaction = &AppStoreTransaction{
				TransactionId:         latestInfo.TransactionID,
				OriginalTransactionId: latestInfo.OriginalTransactionID,
				ProductId:             latestInfo.ProductID,
				PurchaseDate:          purchaseTimeMs,
				ParsedPurchaseDate:    time.Unix(purchaseTimeMs/1000, 0),
			}

			// Try to parse expiry date if available
			if latestInfo.ExpiresDateMs != "" {
				expiryTimeMs, err := strconv.ParseInt(latestInfo.ExpiresDateMs, 10, 64)
				if err == nil {
					transaction.ExpiresDate = expiryTimeMs
					transaction.ParsedExpiresDate = time.Unix(expiryTimeMs/1000, 0)
				}
			}
		}
	}

	// Process based on purchase type
	if purchaseType == PurchaseTypeSubscription {
		return s.processSubscription(ctx, userID, transaction, product, receiptData)
	} else if purchaseType == PurchaseTypeScanPack {
		return s.processScanPack(ctx, userID, transaction, product, receiptData)
	} else {
		logger.Printf("Unsupported purchase type: %s", purchaseType)
		return &PurchaseResponse{
			Success: false,
			Message: fmt.Sprintf("Unsupported purchase type: %s", purchaseType),
		}, fmt.Errorf("unsupported purchase type: %s", purchaseType)
	}
}

// processSubscription handles subscription-specific processing
func (s *SubscriptionService) processSubscription(ctx context.Context, userID int, transaction *AppStoreTransaction, product SubscriptionProduct, receiptData string) (*PurchaseResponse, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing subscription for user %d: %s", userID, product.ProductID)

	// Calculate expiry date if not already set
	purchaseTime := transaction.ParsedPurchaseDate
	var expiryTime time.Time
	if !transaction.ParsedExpiresDate.IsZero() {
		expiryTime = transaction.ParsedExpiresDate
	} else {
		expiryTime = purchaseTime.AddDate(0, 0, product.DurationDays)
	}

	// Process the subscription in a transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Database error",
		}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if this transaction has already been processed
	var existingID int
	err = tx.QueryRow(ctx, `
		SELECT id FROM user_subscriptions
		WHERE user_id = $1 AND original_transaction_id = $2
	`, userID, transaction.OriginalTransactionId).Scan(&existingID)

	// If existing subscription found, update it
	if err == nil {
		var currentCredits int
		// Get current credits
		err = tx.QueryRow(ctx, `
			SELECT scan_credits_remaining
			FROM user_subscriptions
			WHERE id = $1
		`, existingID).Scan(&currentCredits)

		if err != nil && err != pgx.ErrNoRows {
			return &PurchaseResponse{
				Success: false,
				Message: "Failed to retrieve current credits",
			}, fmt.Errorf("failed to get current credits: %w", err)
		}

		// Update existing subscription
		_, err = tx.Exec(ctx, `
			UPDATE user_subscriptions
			SET 
				is_active = true,
				tier = $1,
				subscription_start = $2,
				subscription_end = $3,
				scan_credits_remaining = $4,
				receipt_data = $5,
				transaction_id = $6,
				original_transaction_id = $7,
				environment = $8,
				platform = 'apple'
			WHERE id = $9
		`, product.Tier, purchaseTime, expiryTime, currentCredits+product.ScanCredits,
			receiptData, transaction.TransactionId, transaction.OriginalTransactionId,
			transaction.Environment, existingID)

		if err != nil {
			return &PurchaseResponse{
				Success: false,
				Message: "Failed to update subscription",
			}, fmt.Errorf("failed to update subscription: %w", err)
		}
	} else if err == pgx.ErrNoRows {
		// Insert new subscription
		_, err = tx.Exec(ctx, `
			INSERT INTO user_subscriptions
			(user_id, is_active, tier, subscription_start, subscription_end, 
			scan_credits_remaining, receipt_data, transaction_id, original_transaction_id, 
			environment, platform)
			VALUES
			($1, true, $2, $3, $4, $5, $6, $7, $8, $9, 'apple')
			ON CONFLICT (user_id) 
			DO UPDATE SET
				is_active = true,
				tier = EXCLUDED.tier,
				subscription_start = EXCLUDED.subscription_start,
				subscription_end = EXCLUDED.subscription_end,
				scan_credits_remaining = user_subscriptions.scan_credits_remaining + $5,
				receipt_data = EXCLUDED.receipt_data,
				transaction_id = EXCLUDED.transaction_id,
				original_transaction_id = EXCLUDED.original_transaction_id,
				environment = EXCLUDED.environment,
				platform = 'apple'
		`, userID, product.Tier, purchaseTime, expiryTime, product.ScanCredits,
			receiptData, transaction.TransactionId, transaction.OriginalTransactionId,
			transaction.Environment)

		if err != nil {
			return &PurchaseResponse{
				Success: false,
				Message: "Failed to create subscription",
			}, fmt.Errorf("failed to create subscription: %w", err)
		}
	} else {
		return &PurchaseResponse{
			Success: false,
			Message: "Database error",
		}, fmt.Errorf("database error: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Failed to save subscription",
		}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get updated subscription info
	subscription, err := s.GetUserSubscription(ctx, userID)
	if err != nil {
		logger.Printf("Warning: Failed to get updated subscription: %v", err)
	}

	return &PurchaseResponse{
		Success:      true,
		Message:      fmt.Sprintf("Successfully processed %s subscription", product.Tier),
		Subscription: subscription,
	}, nil
}

// processScanPack handles one-time scan pack purchases
func (s *SubscriptionService) processScanPack(ctx context.Context, userID int, transaction *AppStoreTransaction, product SubscriptionProduct, receiptData string) (*PurchaseResponse, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing scan pack for user %d: %s", userID, product.ProductID)

	// Start a transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Database error",
		}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if this transaction has already been processed
	var transactionExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM scan_pack_purchases
			WHERE transaction_id = $1
		)
	`, transaction.TransactionId).Scan(&transactionExists)

	if err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Database error",
		}, fmt.Errorf("failed to check for existing purchase: %w", err)
	}

	if transactionExists {
		return &PurchaseResponse{
			Success: false,
			Message: "This purchase has already been processed",
		}, fmt.Errorf("duplicate transaction: %s", transaction.TransactionId)
	}

	// Record the scan pack purchase
	_, err = tx.Exec(ctx, `
		INSERT INTO scan_pack_purchases (
			user_id, product_id, transaction_id, original_transaction_id,
			purchase_date, environment, platform, credits_amount
		) VALUES ($1, $2, $3, $4, $5, $6, 'apple', $7)
	`, userID, product.ProductID, transaction.TransactionId,
		transaction.OriginalTransactionId, transaction.ParsedPurchaseDate,
		transaction.Environment, product.ScanCredits)

	if err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Failed to record scan pack purchase",
		}, fmt.Errorf("failed to record purchase: %w", err)
	}

	// Add scan credits to the user's account
	_, err = tx.Exec(ctx, `
		INSERT INTO user_subscriptions (
			user_id, scan_credits_remaining, tier
		) VALUES (
			$1, $2, 'none'
		)
		ON CONFLICT (user_id) DO UPDATE
		SET scan_credits_remaining = user_subscriptions.scan_credits_remaining + $2
	`, userID, product.ScanCredits)

	if err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Failed to add scan credits",
		}, fmt.Errorf("failed to add scan credits: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return &PurchaseResponse{
			Success: false,
			Message: "Failed to complete purchase",
		}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get updated subscription info
	subscription, err := s.GetUserSubscription(ctx, userID)
	if err != nil {
		logger.Printf("Warning: Failed to get updated subscription info: %v", err)
	}

	return &PurchaseResponse{
		Success:      true,
		Message:      fmt.Sprintf("Successfully purchased %s with %d scan credits", product.Name, product.ScanCredits),
		Subscription: subscription,
	}, nil
}

// verifyAppleReceipt sends the receipt to Apple for verification
func (s *SubscriptionService) verifyAppleReceipt(ctx context.Context, receiptData string) (*AppleVerifyResponse, error) {
	receipt := &AppleReceipt{
		ReceiptData:            receiptData,
		Password:               s.appleSharedSecret,
		ExcludeOldTransactions: false,
	}

	jsonData, err := json.Marshal(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal receipt data: %w", err)
	}

	// Try production first
	verifyURL := "https://buy.itunes.apple.com/verifyReceipt"
	resp, err := s.sendVerifyRequest(ctx, verifyURL, jsonData)

	// Check for status 21007 or 21008 which indicate wrong environment
	if err == nil && (resp.Status == 21007 || resp.Status == 21008) {
		// Try sandbox
		sandboxURL := "https://sandbox.itunes.apple.com/verifyReceipt"
		sandboxResp, sandboxErr := s.sendVerifyRequest(ctx, sandboxURL, jsonData)
		if sandboxErr == nil {
			return sandboxResp, nil
		}
	}

	return resp, err
}

// Helper for sending verification requests
func (s *SubscriptionService) sendVerifyRequest(ctx context.Context, verifyURL string, jsonData []byte) (*AppleVerifyResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send verification request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple server returned error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var verifyResponse AppleVerifyResponse
	if err := json.Unmarshal(body, &verifyResponse); err != nil {
		return nil, fmt.Errorf("failed to parse verification response: %w", err)
	}

	return &verifyResponse, nil
}

// GetSubscriptionProducts returns all available subscription products
func (s *SubscriptionService) GetSubscriptionProducts(ctx context.Context, platform string, productType string) ([]SubscriptionProduct, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Getting subscription products for platform: %s, type: %s", platform, productType)

	query := `
		SELECT id, product_id, name, tier, platform, duration_days, scan_credits, type
		FROM subscription_products
		WHERE 
			($1 = '' OR platform = $1)
			AND ($2 = '' OR type = $2)
		ORDER BY 
			CASE 
				WHEN tier = 'basic' THEN 1
				WHEN tier = 'standard' THEN 2
				WHEN tier = 'premium' THEN 3
				ELSE 4
			END
	`

	rows, err := s.db.Query(ctx, query, platform, productType)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscription products: %w", err)
	}
	defer rows.Close()

	var products []SubscriptionProduct
	for rows.Next() {
		var product SubscriptionProduct
		err := rows.Scan(
			&product.ID,
			&product.ProductID,
			&product.Name,
			&product.Tier,
			&product.Platform,
			&product.DurationDays,
			&product.ScanCredits,
			&product.Type,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan product row: %w", err)
		}
		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating product rows: %w", err)
	}

	return products, nil
}

// generateAppStoreToken generates a JWT token for the Apple App Store Server API
func (s *SubscriptionService) generateAppStoreToken() (string, error) {
	log.Printf("Generating App Store JWT token")

	if len(s.applePrivateKey) == 0 {
		log.Printf("ERROR: Apple private key is empty! Check APPLE_PRIVATE_KEY environment variable")
		return "", fmt.Errorf("apple private key is empty")
	}

	log.Printf("Apple private key length: %d bytes", len(s.applePrivateKey))
	log.Printf("Apple issuer ID: %s", maskSecretValue(s.appleIssuerId))
	log.Printf("Apple key ID: %s", maskSecretValue(s.appleKeyId))

	// Parse the private key from PEM format
	block, _ := pem.Decode(s.applePrivateKey)
	if block == nil {
		log.Printf("ERROR: Failed to parse PEM block, invalid private key format")
		return "", fmt.Errorf("failed to parse PEM block")
	}

	log.Printf("Successfully decoded PEM block, type: %s", block.Type)

	var privateKey interface{}
	var err error

	// Try to parse as PKCS8 first (which can contain either RSA or EC keys)
	privateKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		log.Printf("Failed to parse as PKCS8: %v, trying EC format", err)
		// If PKCS8 fails, try EC directly
		privateKey, err = x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			log.Printf("Failed to parse as EC: %v, trying RSA format", err)
			// If EC fails, try PKCS1 (RSA)
			privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				log.Printf("ERROR: All key parsing methods failed: %v", err)
				return "", fmt.Errorf("failed to parse private key: %w", err)
			} else {
				log.Printf("Successfully parsed key as RSA (PKCS1)")
			}
		} else {
			log.Printf("Successfully parsed key as EC")
		}
	} else {
		log.Printf("Successfully parsed key as PKCS8, key type: %T", privateKey)
	}

	// Create JWT claims
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": s.appleIssuerId,           // Issuer ID from App Store Connect
		"iat": now.Unix(),                // Issued at
		"exp": now.Add(time.Hour).Unix(), // Expiration time (1 hour)
		"aud": "appstoreconnect-v1",      // Audience
		"bid": "com.mzinck.CarBN",        // Bundle ID
	}

	// Create a new token
	var token *jwt.Token

	// Check key type and use appropriate signing method
	switch privateKey.(type) {
	case *rsa.PrivateKey:
		log.Printf("Using RSA-256 signing method")
		token = jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	case *ecdsa.PrivateKey:
		log.Printf("Using ECDSA-256 signing method")
		token = jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	default:
		log.Printf("ERROR: Unsupported private key type: %T", privateKey)
		return "", fmt.Errorf("unsupported private key type: %T", privateKey)
	}

	token.Header["kid"] = s.appleKeyId // Key ID from App Store Connect
	log.Printf("Set token header kid: %s", maskSecretValue(s.appleKeyId))

	// Sign and get the encoded token as a string
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		log.Printf("ERROR: Failed to sign JWT: %v", err)
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	log.Printf("Successfully generated JWT token, length: %d", len(tokenString))
	return tokenString, nil
}

// getAppStoreEndpoint returns the proper App Store API URL based on environment
func (s *SubscriptionService) getAppStoreEndpoint(path string, environment string) string {
	baseURL := "https://api.storekit.itunes.apple.com" // Production
	if environment == "Sandbox" {
		baseURL = "https://api.storekit-sandbox.itunes.apple.com"
	}

	return baseURL + path
}

// getTransactionById retrieves a transaction from the App Store Server API
func (s *SubscriptionService) getTransactionById(ctx context.Context, transactionId string) (*AppStoreTransaction, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Getting transaction by ID: %s", transactionId)

	// Check cache with proper locking
	s.transactionCache.mu.RLock()
	transaction, found := s.transactionCache.cache[transactionId]
	s.transactionCache.mu.RUnlock()

	if found {
		logger.Printf("Transaction %s found in cache, expires at: %s",
			transactionId, transaction.ParsedExpiresDate.Format(time.RFC3339))
		return transaction, nil
	}

	logger.Printf("Transaction %s not found in cache, fetching from App Store API", transactionId)

	// Not in cache, fetch from App Store API
	token, err := s.generateAppStoreToken()
	if err != nil {
		logger.Printf("Failed to generate App Store token: %v", err)
		return nil, fmt.Errorf("failed to generate App Store token: %w", err)
	}

	logger.Printf("Successfully generated App Store token, length: %d", len(token))

	// Try production environment first with 3 retries
	logger.Printf("Attempting to fetch transaction %s from Production environment", transactionId)
	transaction, err = s.fetchTransactionFromAPI(ctx, transactionId, token, "Production")

	// If production fails, try sandbox environment with 3 retries
	if err != nil {
		logger.Printf("Failed to fetch from Production: %v. Trying Sandbox environment", err)
		transaction, err = s.fetchTransactionFromAPI(ctx, transactionId, token, "Sandbox")
		if err != nil {
			logger.Printf("Failed to fetch from Sandbox: %v", err)
			return nil, fmt.Errorf("transaction retrieval failed in both environments: %w", err)
		}
	}

	logger.Printf("Successfully retrieved transaction %s from App Store API", transactionId)
	return transaction, nil
}

// fetchTransactionFromAPI makes the actual HTTP request to the App Store Server API
func (s *SubscriptionService) fetchTransactionFromAPI(
	ctx context.Context,
	transactionId string,
	token string,
	environment string,
) (*AppStoreTransaction, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	endpoint := s.getAppStoreEndpoint("/inApps/v1/transactions/"+transactionId, environment)
	logger.Printf("Fetching transaction %s from endpoint: %s", transactionId, endpoint)

	// Implement exponential backoff for retries
	maxRetries := 3
	baseDelay := 500 * time.Millisecond

	var transaction *AppStoreTransaction
	var err error
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			logger.Printf("Retrying App Store API request in %s environment (attempt %d/%d) after %v delay",
				environment, attempt+1, maxRetries, delay)

			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				logger.Printf("Context cancelled during retry: %v", ctx.Err())
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
		}

		// Make the request
		transaction, err = s.doTransactionAPIRequest(ctx, endpoint, token, transactionId)

		// If successful or not a rate limit error, break the retry loop
		if err == nil {
			return transaction, nil
		}

		lastErr = err

		if !s.isRateLimitError(err) {
			// If it's not a rate limit error, no need to retry
			break
		}

		logger.Printf("Rate limit error from App Store API in %s environment: %v", environment, err)
	}

	logger.Printf("All %d attempts failed in %s environment. Last error: %v", maxRetries, environment, lastErr)
	return nil, lastErr
}

// doTransactionAPIRequest performs the actual HTTP request to the App Store API
func (s *SubscriptionService) doTransactionAPIRequest(
	ctx context.Context,
	endpoint string,
	token string,
	transactionId string,
) (*AppStoreTransaction, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Sending API request to endpoint: %s", endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		logger.Printf("Failed to create request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	logger.Printf("Set Authorization header with bearer token length: %d", len(token))

	startTime := time.Now()
	resp, err := s.httpClient.Do(req)
	requestDuration := time.Since(startTime)

	logger.Printf("API request took %v", requestDuration)

	if err != nil {
		logger.Printf("Failed to send transaction request: %v", err)
		return nil, fmt.Errorf("failed to send transaction request: %w", err)
	}
	defer resp.Body.Close()

	logger.Printf("Received response with status code: %d", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("Failed to read response body: %v", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Printf("Response body length: %d bytes", len(body))

	// Log a small sample of the response for debugging
	bodySample := string(body)
	if len(bodySample) > 100 {
		bodySample = bodySample[:100] + "..."
	}
	logger.Printf("Response sample: %s", bodySample)

	if resp.StatusCode != http.StatusOK {
		var errorResp AppStoreErrorResponse
		if err := json.Unmarshal(body, &errorResp); err != nil {
			logger.Printf("Failed to parse error response: %v", err)
			return nil, fmt.Errorf("apple server returned error code %d", resp.StatusCode)
		}
		logger.Printf("Apple server error: %d - %s", errorResp.ErrorCode, errorResp.ErrorMessage)
		return nil, fmt.Errorf("apple server error: %d - %s", errorResp.ErrorCode, errorResp.ErrorMessage)
	}

	// First unmarshal the raw response that contains the signedTransactionInfo
	var rawResponse struct {
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	}

	if err := json.Unmarshal(body, &rawResponse); err != nil {
		logger.Printf("Failed to parse initial response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Now decode the JWT to get the actual transaction details
	if rawResponse.SignedTransactionInfo != "" {
		transaction, err := s.decodeJWTTransaction(rawResponse.SignedTransactionInfo)
		if err != nil {
			logger.Printf("Failed to decode transaction JWT: %v", err)
			return nil, fmt.Errorf("failed to decode transaction JWT: %w", err)
		}

		// Store JWT transaction in cache
		s.transactionCache.mu.Lock()
		s.transactionCache.cache[transactionId] = transaction
		s.transactionCache.mu.Unlock()
		logger.Printf("Stored JWT transaction %s in cache", transactionId)

		// Return the decoded transaction
		return transaction, nil
	} else {
		// Fall back to direct unmarshaling if no JWT is present
		var transaction AppStoreTransaction
		if err := json.Unmarshal(body, &transaction); err != nil {
			logger.Printf("Failed to parse transaction response: %v", err)
			return nil, fmt.Errorf("failed to parse transaction response: %w", err)
		}

		// Parse timestamps into Go time.Time
		transaction.ParsedPurchaseDate = time.Unix(transaction.PurchaseDate/1000, 0)
		transaction.ParsedExpiresDate = time.Unix(transaction.ExpiresDate/1000, 0)
		transaction.ParsedOriginalPurchaseDate = time.Unix(transaction.OriginalPurchaseDate/1000, 0)

		logger.Printf("Transaction %s details - Product: %s, Purchase date: %s, Expires: %s",
			transaction.TransactionId,
			transaction.ProductId,
			transaction.ParsedPurchaseDate.Format(time.RFC3339),
			transaction.ParsedExpiresDate.Format(time.RFC3339))

		// Store in cache
		s.transactionCache.mu.Lock()
		s.transactionCache.cache[transactionId] = &transaction
		s.transactionCache.mu.Unlock()
		logger.Printf("Stored transaction %s in cache", transactionId)

		logger.Printf("Successfully retrieved transaction %s, expires: %s",
			transactionId, transaction.ParsedExpiresDate.Format(time.RFC3339))

		return &transaction, nil
	}
}

// isRateLimitError determines if the error is due to rate limiting
func (s *SubscriptionService) isRateLimitError(err error) bool {
	// Check for rate limit error codes
	// Apple's rate limit error is typically 429 Too Many Requests
	return strings.Contains(err.Error(), "429") ||
		strings.Contains(err.Error(), "too many requests")
}

// decodeJWTTransaction decodes a signed transaction from a notification
func (s *SubscriptionService) decodeJWTTransaction(signedTransaction string) (*AppStoreTransaction, error) {
	// Parse without verification since Apple already signed it
	token, err := jwt.ParseWithClaims(signedTransaction, &AppStoreTransaction{}, func(token *jwt.Token) (interface{}, error) {
		return nil, nil // We're not verifying the signature here
	})

	// Check if token is nil before accessing its properties
	if token == nil {
		return nil, fmt.Errorf("failed to parse transaction JWT: token is nil")
	}

	if err != nil && !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		// Only return error if it's not just the signature validation
		return nil, fmt.Errorf("failed to parse transaction JWT: %w", err)
	}

	claims, ok := token.Claims.(*AppStoreTransaction)
	if !ok {
		return nil, fmt.Errorf("failed to parse transaction claims")
	}

	// Parse timestamps into Go time.Time
	claims.ParsedPurchaseDate = time.Unix(claims.PurchaseDate/1000, 0)
	claims.ParsedExpiresDate = time.Unix(claims.ExpiresDate/1000, 0)
	claims.ParsedOriginalPurchaseDate = time.Unix(claims.OriginalPurchaseDate/1000, 0)

	return claims, nil
}

// ProcessAppStoreNotification processes notifications from the App Store Server Notifications V2 API
func (s *SubscriptionService) ProcessAppStoreNotification(ctx context.Context, notification *AppStoreNotificationPayload) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing notification type: %s, subtype: %s", notification.NotificationType, notification.Subtype)

	// Decode transaction info from the notification
	if notification.Data.SignedTransactionInfo == "" {
		return fmt.Errorf("no transaction info in notification")
	}

	transaction, err := s.decodeJWTTransaction(notification.Data.SignedTransactionInfo)
	if err != nil {
		return fmt.Errorf("failed to decode transaction info: %w", err)
	}

	logger.Printf("Processing transaction %s for product %s",
		transaction.TransactionId, transaction.ProductId)

	// Handle different notification types
	switch notification.NotificationType {
	case "SUBSCRIBED":
		return s.processSubscriptionStart(ctx, transaction)

	case "DID_RENEW":
		return s.processSubscriptionRenewal(ctx, transaction)

	case "DID_FAIL_TO_RENEW":
		return s.processFailedRenewal(ctx, transaction)

	case "EXPIRED":
		return s.processSubscriptionExpired(ctx, transaction)

	case "REFUND":
		return s.processRefund(ctx, transaction)

	case "REVOKE":
		return s.processRevoke(ctx, transaction)

	case "CONSUMPTION_REQUEST":
		// Handle consumption request if needed
		return nil

	default:
		logger.Printf("Unhandled notification type: %s", notification.NotificationType)
		return nil
	}
}

// processSubscriptionStart handles a new subscription or resubscription
func (s *SubscriptionService) processSubscriptionStart(ctx context.Context, transaction *AppStoreTransaction) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing subscription start for transaction %s", transaction.TransactionId)

	// Find the user associated with this subscription
	var userID int
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM user_subscriptions 
		WHERE original_transaction_id = $1
	`, transaction.OriginalTransactionId).Scan(&userID)

	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("failed to find user for transaction: %w", err)
	}

	if err == pgx.ErrNoRows {
		logger.Printf("No user found for transaction %s, subscription may be from another system",
			transaction.OriginalTransactionId)
		return nil
	}

	// Find the product in our database
	var product SubscriptionProduct
	err = s.db.QueryRow(ctx, `
		SELECT id, product_id, name, tier, platform, duration_days, scan_credits
		FROM subscription_products
		WHERE product_id = $1 AND platform = 'apple'
	`, transaction.ProductId).Scan(&product.ID, &product.ProductID, &product.Name,
		&product.Tier, &product.Platform, &product.DurationDays, &product.ScanCredits)

	if err != nil {
		logger.Printf("Product not found for %s", transaction.ProductId)
		return fmt.Errorf("product not found: %w", err)
	}

	// Update subscription record
	_, err = s.db.Exec(ctx, `
		UPDATE user_subscriptions
		SET 
			is_active = true,
			tier = $1,
			subscription_start = $2,
			subscription_end = $3,
			transaction_id = $4,
			environment = $5
		WHERE user_id = $6
	`, product.Tier,
		transaction.ParsedPurchaseDate,
		transaction.ParsedExpiresDate,
		transaction.TransactionId,
		transaction.Environment,
		userID)

	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	logger.Printf("Successfully updated subscription for user %d", userID)
	return nil
}

// processSubscriptionRenewal handles subscription renewal
func (s *SubscriptionService) processSubscriptionRenewal(ctx context.Context, transaction *AppStoreTransaction) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing subscription renewal for transaction %s", transaction.TransactionId)

	// Find the user associated with this subscription
	var userID int
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM user_subscriptions 
		WHERE original_transaction_id = $1
	`, transaction.OriginalTransactionId).Scan(&userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No user found for transaction %s, ignoring renewal",
				transaction.OriginalTransactionId)
			return nil
		}
		return fmt.Errorf("failed to find user for transaction: %w", err)
	}

	// Find the product to get scan credits
	var product SubscriptionProduct
	err = s.db.QueryRow(ctx, `
		SELECT id, product_id, name, tier, platform, duration_days, scan_credits
		FROM subscription_products
		WHERE product_id = $1 AND platform = 'apple'
	`, transaction.ProductId).Scan(&product.ID, &product.ProductID, &product.Name,
		&product.Tier, &product.Platform, &product.DurationDays, &product.ScanCredits)

	if err != nil {
		logger.Printf("Product not found for %s, using existing tier", transaction.ProductId)
		// Update with just the new expiry date
		_, err = s.db.Exec(ctx, `
			UPDATE user_subscriptions
			SET 
				is_active = true,
				subscription_end = $1,
				transaction_id = $2
			WHERE user_id = $3
		`, transaction.ParsedExpiresDate, transaction.TransactionId, userID)
	} else {
		// Update with new product details and add scan credits
		_, err = s.db.Exec(ctx, `
			UPDATE user_subscriptions
			SET 
				is_active = true,
				tier = $1,
				subscription_end = $2,
				scan_credits_remaining = scan_credits_remaining + $3,
				transaction_id = $4
			WHERE user_id = $5
		`, product.Tier,
			transaction.ParsedExpiresDate,
			product.ScanCredits,
			transaction.TransactionId,
			userID)
	}

	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	logger.Printf("Successfully renewed subscription for user %d until %s",
		userID, transaction.ParsedExpiresDate.Format(time.RFC3339))
	return nil
}

// processFailedRenewal handles failed subscription renewal
func (s *SubscriptionService) processFailedRenewal(ctx context.Context, transaction *AppStoreTransaction) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing failed renewal for transaction %s", transaction.OriginalTransactionId)

	// Find the user associated with this subscription
	var userID int
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM user_subscriptions 
		WHERE original_transaction_id = $1
	`, transaction.OriginalTransactionId).Scan(&userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No user found for transaction %s, ignoring failed renewal",
				transaction.OriginalTransactionId)
			return nil
		}
		return fmt.Errorf("failed to find user for transaction: %w", err)
	}

	// Mark subscription as inactive if current date is past expiration date
	now := time.Now()
	if now.After(transaction.ParsedExpiresDate) {
		_, err = s.db.Exec(ctx, `
			UPDATE user_subscriptions
			SET 
				is_active = false
			WHERE user_id = $1
		`, userID)

		if err != nil {
			return fmt.Errorf("failed to mark subscription as inactive: %w", err)
		}

		logger.Printf("Marked subscription as inactive for user %d", userID)
	} else {
		logger.Printf("Subscription for user %d is still active until %s",
			userID, transaction.ParsedExpiresDate.Format(time.RFC3339))
	}

	return nil
}

// processSubscriptionExpired handles subscription expiration
func (s *SubscriptionService) processSubscriptionExpired(ctx context.Context, transaction *AppStoreTransaction) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing subscription expiration for transaction %s", transaction.OriginalTransactionId)

	// Find the user associated with this subscription
	var userID int
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM user_subscriptions 
		WHERE original_transaction_id = $1
	`, transaction.OriginalTransactionId).Scan(&userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No user found for transaction %s, ignoring expiration",
				transaction.OriginalTransactionId)
			return nil
		}
		return fmt.Errorf("failed to find user for transaction: %w", err)
	}

	// Mark subscription as inactive
	_, err = s.db.Exec(ctx, `
		UPDATE user_subscriptions
		SET 
			is_active = false,
			subscription_end = $1
		WHERE user_id = $2
	`, transaction.ParsedExpiresDate, userID)

	if err != nil {
		return fmt.Errorf("failed to expire subscription: %w", err)
	}

	logger.Printf("Successfully expired subscription for user %d", userID)
	return nil
}

// processRefund handles subscription refunds
func (s *SubscriptionService) processRefund(ctx context.Context, transaction *AppStoreTransaction) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing refund for transaction %s", transaction.TransactionId)

	// Find the user associated with this subscription
	var userID int
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM user_subscriptions 
		WHERE transaction_id = $1 OR original_transaction_id = $2
	`, transaction.TransactionId, transaction.OriginalTransactionId).Scan(&userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No user found for transaction %s, ignoring refund",
				transaction.TransactionId)
			return nil
		}
		return fmt.Errorf("failed to find user for transaction: %w", err)
	}

	// Mark subscription as inactive and remove scan credits
	_, err = s.db.Exec(ctx, `
		UPDATE user_subscriptions
		SET 
			is_active = false,
			subscription_end = NOW(),
			scan_credits_remaining = 6
		WHERE user_id = $1
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to process refund: %w", err)
	}

	logger.Printf("Successfully processed refund for user %d", userID)
	return nil
}

// processRevoke handles revoked subscriptions
func (s *SubscriptionService) processRevoke(ctx context.Context, transaction *AppStoreTransaction) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Processing revoke for transaction %s", transaction.OriginalTransactionId)

	// Find the user associated with this subscription
	var userID int
	err := s.db.QueryRow(ctx, `
		SELECT user_id FROM user_subscriptions 
		WHERE original_transaction_id = $1
	`, transaction.OriginalTransactionId).Scan(&userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			logger.Printf("No user found for transaction %s, ignoring revoke",
				transaction.OriginalTransactionId)
			return nil
		}
		return fmt.Errorf("failed to find user for transaction: %w", err)
	}

	// Mark subscription as inactive
	_, err = s.db.Exec(ctx, `
		UPDATE user_subscriptions
		SET 
			is_active = false,
			subscription_end = NOW(),
			scan_credits_remaining = 6
		WHERE user_id = $1
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to revoke subscription: %w", err)
	}

	logger.Printf("Successfully revoked subscription for user %d", userID)
	return nil
}

// Helper function to mask sensitive information for logging
func maskSecretValue(value string) string {
	if value == "" {
		return "[empty]"
	}

	if len(value) <= 8 {
		return "****" + value[len(value)-2:]
	}

	return value[:4] + "****" + value[len(value)-4:]
}

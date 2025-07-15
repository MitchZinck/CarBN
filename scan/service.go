package scan

import (
	"CarBN/common"
	"CarBN/feed"
	"CarBN/subscription"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type CarImagePaths struct {
	LowRes  string
	HighRes string
}

type Service struct {
	db                  *pgxpool.Pool
	feedService         *feed.Service
	subscriptionService *subscription.SubscriptionService
	defaultAPIKey       string
	defaultBaseURL      string
	fallbackAPIKey      string
	fallbackBaseURL     string
	defaultChatModel    string
	defaultVisionModel  string
	fallbackChatModel   string
	fallbackVisionModel string
	client              *http.Client
	scanSaveDir         string
	generatedSaveDir    string
}

type car struct {
	ID             int     `json:"id"`
	UserCarID      int     `json:"user_car_id"`
	UserID         int     `json:"user_id"`
	Make           string  `json:"make"`
	Model          string  `json:"model"`
	Year           string  `json:"year"`
	Color          string  `json:"color"`
	Trim           string  `json:"trim,omitempty"`
	Horsepower     int     `json:"horsepower"`
	Torque         int     `json:"torque"`
	TopSpeed       int     `json:"top_speed"`
	Acceleration   float64 `json:"acceleration"`
	EngineType     string  `json:"engine_type"`
	DrivetrainType string  `json:"drivetrain_type"`
	CurbWeight     float64 `json:"curb_weight"`
	Price          int     `json:"price"`
	Description    string  `json:"description"`
	Rarity         int     `json:"rarity"`
	LowResImage    string  `json:"low_res_image"`
	HighResImage   string  `json:"high_res_image"`
	DateCollected  string  `json:"date_collected"`
}

type AIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type CarDetails struct {
	Make           string  `json:"make"`
	Model          string  `json:"model"`
	Year           string  `json:"year"`
	Color          string  `json:"color"`
	Trim           string  `json:"trim"`
	Horsepower     string  `json:"horsepower"`
	Torque         string  `json:"torque"`
	TopSpeed       string  `json:"top_speed"`
	Acceleration   string  `json:"acceleration"`
	EngineType     string  `json:"engine_type"`
	DrivetrainType string  `json:"drivetrain_type"`
	CurbWeight     string  `json:"curb_weight"`
	Price          string  `json:"price"`
	Description    string  `json:"description"`
	Rarity         *string `json:"rarity"`
}

type ScanImageResponse struct {
	Make   string `json:"make"`
	Model  string `json:"model"`
	Trim   string `json:"trim"`
	Year   string `json:"year"`
	Color  string `json:"color"`
	Reject bool   `json:"reject"`
}
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatPayload struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

func NewService(db *pgxpool.Pool, feedService *feed.Service, subscriptionService *subscription.SubscriptionService) *Service {
	defaultAPIKey := os.Getenv("DEFAULT_API_KEY")
	defaultBaseURL := os.Getenv("DEFAULT_BASE_URL")
	fallbackAPIKey := os.Getenv("FALLBACK_API_KEY")
	fallbackBaseURL := os.Getenv("FALLBACK_BASE_URL")
	defaultChatModel := os.Getenv("DEFAULT_CHAT_MODEL")
	defaultVisionModel := os.Getenv("DEFAULT_VISION_MODEL")
	fallbackChatModel := os.Getenv("FALLBACK_CHAT_MODEL")
	fallbackVisionModel := os.Getenv("FALLBACK_VISION_MODEL")
	scanSaveDir := os.Getenv("SCAN_SAVE_DIR")
	generatedSaveDir := os.Getenv("GENERATED_SAVE_DIR")

	// Add logging to verify environment variables
	log.Printf("Environment variables loaded:")
	log.Printf("SCAN_SAVE_DIR: %s", scanSaveDir)
	log.Printf("GENERATED_SAVE_DIR: %s", generatedSaveDir)

	// Verify directories exist and are accessible
	if generatedSaveDir != "" {
		if err := os.MkdirAll(generatedSaveDir, 0750); err != nil {
			log.Printf("Warning: Failed to create GENERATED_SAVE_DIR: %v", err)
		}
	} else {
		log.Printf("Warning: GENERATED_SAVE_DIR is empty")
	}

	if scanSaveDir != "" {
		if err := os.MkdirAll(scanSaveDir, 0750); err != nil {
			log.Printf("Warning: Failed to create SCAN_SAVE_DIR: %v", err)
		}
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	return &Service{
		db:                  db,
		feedService:         feedService,
		subscriptionService: subscriptionService,
		defaultAPIKey:       defaultAPIKey,
		defaultBaseURL:      defaultBaseURL,
		fallbackAPIKey:      fallbackAPIKey,
		fallbackBaseURL:     fallbackBaseURL,
		defaultChatModel:    defaultChatModel,
		defaultVisionModel:  defaultVisionModel,
		fallbackChatModel:   fallbackChatModel,
		fallbackVisionModel: fallbackVisionModel,
		client:              client,
		scanSaveDir:         scanSaveDir,
		generatedSaveDir:    generatedSaveDir,
	}
}

func (s *Service) ScanImage(ctx context.Context, userID int, base64Image string) (*car, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	hasCredits, err := s.subscriptionService.HasScanCredits(ctx, userID)
	if err != nil {
		logger.Printf("Failed to check scan credits: %v", err)
		return nil, fmt.Errorf("failed to check scan credits: %w", err)
	}
	if !hasCredits {
		return nil, fmt.Errorf("no scan credits remaining")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("Failed to begin transaction: %v", err)
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			logger.Printf("Warning: transaction rollback error: %v", err)
		}
	}()

	carDetails, err := s.identifyCar(ctx, base64Image)
	if err != nil {
		// Record failed scan in history if it's a rejection
		if err.Error() == common.RejectedScanError {
			scanPath := fmt.Sprintf("user_%d/rejected_scan_%d.jpg", userID, time.Now().Unix())
			if s.scanSaveDir != "" {
				if saveErr := s.saveImage(base64Image, s.scanSaveDir, scanPath); saveErr != nil {
					logger.Printf("Warning: failed to save rejected scan: %v", saveErr)
				}
			}
			// if recordErr := s.recordScanHistory(ctx, tx, userID, 0, "", scanPath, false, err.Error()); recordErr != nil {
			// 	logger.Printf("Warning: failed to record scan history: %v", recordErr)
			// }
			// if commitErr := tx.Commit(ctx); commitErr != nil {
			// 	logger.Printf("Warning: failed to commit rejected scan history: %v", commitErr)
			// }
		}
		logger.Printf("Failed to identify car: %v", err)
		return nil, err
	}

	// Check for duplicate scans
	if err := s.checkForDuplicateScan(ctx, tx, userID, carDetails.Make, carDetails.Model, carDetails.Trim, carDetails.Year); err != nil {
		logger.Printf("Duplicate scan check failed: %v", err)
		return nil, err
	}

	// Continue with the existing scan logic
	carID, err := s.fetchOrCreateCar(ctx, tx, carDetails)
	if err != nil {
		logger.Printf("Failed to fetch or create car: %v", err)
		return nil, fmt.Errorf("fetchOrCreateCar failed: %w", err)
	}

	imagePaths, err := s.getCarImagePaths(ctx, tx, carID, carDetails.Color)
	if err != nil {
		logger.Printf("Failed to get car image paths for car ID %d: %v", carID, err)
		return nil, fmt.Errorf("failed to get car image paths: %w", err)
	}

	var highResPath, lowResPath string
	if imagePaths == nil {
		logger.Printf("Generating new images for car ID %d", carID)
		generatedImage, err := common.GenerateCarImage(ctx, carDetails.Year, carDetails.Make, carDetails.Model, carDetails.Trim, carDetails.Color, false, "")
		if err != nil {
			logger.Printf("Failed to generate car image: %v", err)
			return nil, fmt.Errorf("failed to generate car image: %w", err)
		}

		// Create a timestamp for unique filenames
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)

		// Define relative paths for database storage - now including 'generated' in the path and timestamp
		colorDir := fmt.Sprintf("car_%d/%s", carID, strings.ToLower(carDetails.Color))
		highResFileName := fmt.Sprintf("high_res_%d.jpg", timestamp)
		lowResFileName := fmt.Sprintf("low_res_%d.jpg", timestamp)
		highResPath = filepath.Join("generated", colorDir, highResFileName)
		lowResPath = filepath.Join("generated", colorDir, lowResFileName)

		// Use full paths for actual file saving
		// fullHighResPath := filepath.Join(s.generatedSaveDir, colorDir, highResFileName)
		fullLowResPath := filepath.Join(s.generatedSaveDir, colorDir, lowResFileName)

		// Save using full paths
		if err := s.saveImage(generatedImage, s.generatedSaveDir, filepath.Join(colorDir, highResFileName)); err != nil {
			logger.Printf("Failed to save high res image: %v", err)
			return nil, fmt.Errorf("failed to save high res image: %w", err)
		}

		if err := s.saveLowResImage(generatedImage, fullLowResPath); err != nil {
			logger.Printf("Failed to save low res image: %v", err)
			return nil, fmt.Errorf("failed to save low res image: %w", err)
		}

		colorImages := make(map[string]map[string]string)
		colorImages[strings.ToLower(carDetails.Color)] = map[string]string{
			"high_res": highResPath,
			"low_res":  lowResPath,
		}

		if _, err := tx.Exec(ctx, "UPDATE cars SET color_images = $1 WHERE id = $2",
			colorImages, carID); err != nil {
			logger.Printf("Failed to update car color images: %v", err)
			return nil, fmt.Errorf("failed to update car color images: %w", err)
		}
	} else {
		logger.Printf("Using existing images for car ID %d", carID)
		highResPath = imagePaths.HighRes
		lowResPath = imagePaths.LowRes
	}

	var userCarID int
	err = tx.QueryRow(ctx,
		`INSERT INTO user_cars (user_id, car_id, color, high_res_image, low_res_image)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		userID, carID, carDetails.Color, highResPath, lowResPath,
	).Scan(&userCarID)
	if err != nil {
		logger.Printf("Failed to create user_cars entry: %v", err)
		return nil, fmt.Errorf("failed to create user_cars entry: %w", err)
	}

	if s.scanSaveDir != "" {
		scanPath := fmt.Sprintf("user_%d/scan_%d.jpg", userID, userCarID)
		if err := s.saveImage(base64Image, s.scanSaveDir, scanPath); err != nil {
			logger.Printf("Warning: failed to save original scan: %v", err)
		}
	}

	// Record successful scan in history
	scanPath := fmt.Sprintf("user_%d/scan_%d.jpg", userID, userCarID)
	if err := s.recordScanHistory(ctx, tx, userID, carID, carDetails.Color, scanPath, true, ""); err != nil {
		logger.Printf("Failed to record scan history: %v", err)
		return nil, fmt.Errorf("failed to record scan history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Printf("Failed to commit transaction: %v", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch the complete car details using the main database connection
	var result car
	var dateCollected time.Time
	err = s.db.QueryRow(ctx, `
		SELECT c.id, uc.id, c.make, c.model, c.year, uc.color, c.trim,
			c.horsepower, c.torque, c.top_speed, c.acceleration,
			c.engine_type, c.drivetrain_type, c.curb_weight,
			c.price, c.description, c.rarity,
			uc.low_res_image, uc.high_res_image, uc.date_collected
		FROM cars c
		JOIN user_cars uc ON c.id = uc.car_id
		WHERE uc.id = $1`, userCarID).Scan(
		&result.ID, &result.UserCarID, &result.Make, &result.Model,
		&result.Year, &result.Color, &result.Trim, &result.Horsepower,
		&result.Torque, &result.TopSpeed, &result.Acceleration,
		&result.EngineType, &result.DrivetrainType, &result.CurbWeight,
		&result.Price, &result.Description, &result.Rarity,
		&result.LowResImage, &result.HighResImage, &dateCollected)
	if err != nil {
		logger.Printf("Failed to fetch complete car details: %v", err)
		return nil, fmt.Errorf("failed to fetch complete car details: %w", err)
	}

	// Set the UserID in the result
	result.UserID = userID

	// Format the date using common.FormatTimestamp
	result.DateCollected = common.FormatTimestamp(dateCollected)

	if result.Rarity >= 4 {
		// Create feed entry after successful transaction
		err = s.feedService.CreateFeed(ctx, userID, "car_scanned", userCarID, 0)
		if err != nil {
			logger.Printf("Failed to create feed for user %d and car %d: %v", userID, userCarID, err)
		}
	}

	// Deduct scan credit after successful car creation
	if err := s.subscriptionService.DeductScanCredit(ctx, userID); err != nil {
		logger.Printf("Failed to deduct scan credit: %v", err)
		return nil, fmt.Errorf("failed to deduct scan credit: %w", err)
	}

	logger.Printf("Successfully created scan entry for user %d, car ID %d", userID, carID)
	return &result, nil
}

func (s *Service) identifyCar(ctx context.Context, base64Image string) (*CarDetails, error) {
	carDetails, err := s.callVisionAPI(ctx, s.defaultBaseURL, s.defaultAPIKey, s.defaultVisionModel, base64Image)
	if err == nil {
		return carDetails, nil
	}
	// If scan rejected, return error immediately.
	if err.Error() == common.RejectedScanError {
		return nil, err
	}
	log.Printf("Default AI request failed: %v, falling back to Fallback AI", err)
	carDetails, err = s.callVisionAPI(ctx, s.fallbackBaseURL, s.fallbackAPIKey, s.fallbackVisionModel, base64Image)
	if err != nil {
		// Ensure error is properly formatted as JSON
		if err.Error() == common.RejectedScanError {
			return nil, errors.New(common.RejectedScanError)
		}
		log.Printf("Failed to identify car: %v", err)
		return nil, fmt.Errorf(`{"error": "Failed to identify car: %s"}`, err.Error())
	}
	return carDetails, nil
}

func (s *Service) identifyCarDetails(ctx context.Context, make string, model string, trim string, year string) (*CarDetails, error) {
	carDetails, err := s.callChatAPI(ctx, s.defaultBaseURL, s.defaultAPIKey, s.defaultChatModel, make, model, trim, year)
	if err == nil {
		return carDetails, nil
	}

	log.Printf("OpenAI request failed (details): %v, falling back to XAI", err)
	return s.callChatAPI(ctx, s.fallbackBaseURL, s.fallbackAPIKey, s.fallbackChatModel, make, model, trim, year)
}

func (s *Service) callVisionAPI(ctx context.Context, baseURL, apiKey, model, base64Image string) (*CarDetails, error) {
	payload := struct {
		Model    string        `json:"model"`
		Messages []interface{} `json:"messages"`
	}{
		Model: model,
		Messages: []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": common.SCAN_IMAGE,
					},
					map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]string{
							"url":    fmt.Sprintf("data:image/jpeg;base64,%s", base64Image),
							"detail": "high",
						},
					},
				},
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	var scanResp ScanImageResponse
	if err := s.makeAIRequest(ctx, baseURL, apiKey, payloadBytes, &scanResp); err != nil {
		return nil, err
	}

	if scanResp.Reject {
		// Modified error message on scan rejection.
		return nil, errors.New(common.RejectedScanError)
	}

	return &CarDetails{
		Make:  scanResp.Make,
		Model: scanResp.Model,
		Trim:  scanResp.Trim,
		Year:  scanResp.Year,
		Color: scanResp.Color,
	}, nil
}

func (s *Service) callChatAPI(ctx context.Context, baseURL string, apiKey string, aiModel string, make string, model string, trim string, year string) (*CarDetails, error) {
	prompt := fmt.Sprintf(common.IDENTIFY_CAR_DETAILS, year, make, model, trim)
	payload := ChatPayload{
		Model: aiModel,
		Messages: []ChatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat payload: %w", err)
	}

	var carDetails CarDetails
	if err := s.makeAIRequest(ctx, baseURL, apiKey, payloadBytes, &carDetails); err != nil {
		return nil, err
	}
	return &carDetails, nil
}

func (s *Service) makeAIRequest(ctx context.Context, baseURL, apiKey string, payload []byte, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", baseURL), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create AI request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("AI request error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
		logger.Printf("AI API error response (status %d): %s", resp.StatusCode, string(body))
		return fmt.Errorf("AI API returned status %d: %s", resp.StatusCode, string(body))
	}

	var aiResp AIResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		return fmt.Errorf("failed to decode AI response: %w", err)
	}

	if len(aiResp.Choices) == 0 {
		return fmt.Errorf("no response from AI")
	}

	content := aiResp.Choices[0].Message.Content

	// Extract JSON content between curly braces
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	return json.Unmarshal([]byte(content), out)
}

// Add this helper function before fetchOrCreateCar
func calculateRarity(price int) int {
	switch {
	case price >= 500000:
		return 5
	case price >= 150000:
		return 4
	case price >= 50000:
		return 3
	case price >= 25000:
		return 2
	default:
		return 1
	}
}

func (s *Service) fetchOrCreateCar(ctx context.Context, tx pgx.Tx, c *CarDetails) (int, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Attempting to fetch or create car: %s %s %s %s",
		c.Year, c.Make, c.Model, c.Trim)

	var carID int
	baseQuery := "SELECT id FROM cars WHERE LOWER(make)=LOWER($1) AND LOWER(model)=LOWER($2)"
	args := []interface{}{c.Make, c.Model}
	paramCount := 2

	// Add year to query if present
	if c.Year != "" {
		paramCount++
		baseQuery += fmt.Sprintf(" AND year=$%d", paramCount)
		args = append(args, c.Year)
	}

	// Add trim to query if present
	if c.Trim != "" {
		paramCount++
		baseQuery += fmt.Sprintf(" AND LOWER(trim)=LOWER($%d)", paramCount)
		args = append(args, c.Trim)
	}

	logger.Printf("Executing query to find existing car: %s", baseQuery)
	err := tx.QueryRow(ctx, baseQuery, args...).Scan(&carID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			logger.Printf("Error querying for existing car: %v", err)
			return 0, fmt.Errorf("failed to query car: %w", err)
		}

		logger.Printf("Car not found, fetching additional details from AI")
		additionalDetails, aiErr := s.identifyCarDetails(ctx, c.Make, c.Model, c.Trim, c.Year)
		if aiErr != nil {
			logger.Printf("Failed to get additional car details from AI: %v", aiErr)
			return 0, fmt.Errorf("failed to get additional car details: %w", aiErr)
		}

		price := convertToInt(additionalDetails.Price)
		rarity := calculateRarity(price)

		logger.Printf("Creating new car entry with details: %s %s (Year: %s)", c.Make, c.Model, c.Year)
		err = tx.QueryRow(ctx,
			`INSERT INTO cars (
				make, model, year, trim,
				horsepower, torque, top_speed, acceleration,
				engine_type, drivetrain_type, curb_weight,
				price, description, rarity
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING id`,
			c.Make, c.Model, c.Year, c.Trim,
			convertToInt(additionalDetails.Horsepower),
			convertToInt(additionalDetails.Torque),
			convertToInt(additionalDetails.TopSpeed),
			convertToFloat64(additionalDetails.Acceleration),
			additionalDetails.EngineType,
			additionalDetails.DrivetrainType,
			convertToFloat64(additionalDetails.CurbWeight),
			price,
			additionalDetails.Description,
			rarity,
		).Scan(&carID)
		if err != nil {
			logger.Printf("Failed to create new car entry: %v", err)
			return 0, fmt.Errorf("failed to create car entry: %w", err)
		}
		logger.Printf("Successfully created new car with ID: %d", carID)
	} else {
		logger.Printf("Found existing car with ID: %d", carID)
	}
	return carID, nil
}

// Add these helper functions for type conversion
func convertToInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		// Remove any non-numeric characters except decimal point and negative sign
		clean := strings.TrimFunc(t, func(r rune) bool {
			return !unicode.IsDigit(r) && r != '.' && r != '-'
		})
		if i, err := strconv.ParseInt(clean, 10, 64); err == nil {
			return int(i)
		}
		if f, err := strconv.ParseFloat(clean, 64); err == nil {
			return int(f)
		}
	}
	return 0
}

func convertToFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		// Remove any non-numeric characters except decimal point and negative sign
		clean := strings.TrimFunc(t, func(r rune) bool {
			return !unicode.IsDigit(r) && r != '.' && r != '-'
		})
		if f, err := strconv.ParseFloat(clean, 64); err == nil {
			return f
		}
	}
	return 0
}

// saveImage wrapper to maintain compatibility
func (s *Service) saveImage(base64Image string, baseDir string, relativePath string) error {
	return common.SaveImage(base64Image, baseDir, relativePath, 0755)
}

// saveLowResImage wrapper to maintain compatibility
func (s *Service) saveLowResImage(originalImage string, savePath string) error {
	return common.SaveLowResVersion(originalImage, savePath, 95, 0755)
}

func (s *Service) getCarImagePaths(ctx context.Context, tx pgx.Tx, carID int, color string) (*CarImagePaths, error) {
	var colorImages map[string]map[string]string
	err := tx.QueryRow(ctx, "SELECT color_images FROM cars WHERE id = $1", carID).Scan(&colorImages)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query car images: %w", err)
	}

	if colorImages == nil {
		return nil, nil
	}

	colorKey := strings.ToLower(color)
	if colorData, exists := colorImages[colorKey]; exists {
		if highRes, ok := colorData["high_res"]; ok {
			if lowRes, ok := colorData["low_res"]; ok {
				return &CarImagePaths{
					HighRes: highRes,
					LowRes:  lowRes,
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *Service) checkForDuplicateScan(ctx context.Context, tx pgx.Tx, userID int, make, model, trim string, year string) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	// Build query to find car ID
	baseQuery := "SELECT c.id FROM cars c INNER JOIN scan_history sh ON c.id = sh.car_id WHERE sh.user_id = $1 AND LOWER(c.make) = LOWER($2) AND LOWER(c.model) = LOWER($3)"
	args := []interface{}{userID, make, model}
	paramCount := 3

	if year != "" {
		paramCount++
		baseQuery += fmt.Sprintf(" AND c.year = $%d", paramCount)
		args = append(args, year)
	}

	if trim != "" {
		paramCount++
		baseQuery += fmt.Sprintf(" AND LOWER(c.trim) = LOWER($%d)", paramCount)
		args = append(args, trim)
	}

	baseQuery += " AND sh.scanned_at > NOW() - INTERVAL '24 hours'"

	var carID int
	err := tx.QueryRow(ctx, baseQuery, args...).Scan(&carID)
	if err == nil {
		logger.Printf("Found duplicate scan for user %d, car %d within last 24 hours", userID, carID)
		return errors.New(common.DuplicateScanError)
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("error checking for duplicate scan: %w", err)
	}
	return nil
}

func (s *Service) recordScanHistory(ctx context.Context, tx pgx.Tx, userID, carID int, color, imagePath string, success bool, rejectionReason string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO scan_history (user_id, car_id, color, image_path, success, rejection_reason)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, carID, color, imagePath, success, rejectionReason)
	return err
}

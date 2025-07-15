package user

import (
	"CarBN/common"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Service configuration
type ServiceConfig struct {
	BaseURL string
}

func NewService(db *pgxpool.Pool, generatedSaveDir string) *Service {
	return &Service{
		db:               db,
		generatedSaveDir: generatedSaveDir,
		config: ServiceConfig{
			BaseURL: os.Getenv("BASE_URL"),
		},
	}
}

type Service struct {
	db               *pgxpool.Pool
	generatedSaveDir string
	config           ServiceConfig
}

type carUpgrade struct {
	ID          int    `json:"id"`
	UpgradeType string `json:"upgrade_type"`
	Active      bool   `json:"active"`
	Metadata    any    `json:"metadata,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type car struct {
	ID             int          `json:"id"`
	UserCarID      int          `json:"user_car_id,omitempty"`
	UserID         int          `json:"user_id,omitempty"`
	Make           string       `json:"make"`
	Model          string       `json:"model"`
	Year           string       `json:"year"`
	Color          string       `json:"color"`
	Trim           string       `json:"trim,omitempty"`
	Horsepower     *int         `json:"horsepower,omitempty"`
	Torque         *int         `json:"torque,omitempty"`
	TopSpeed       *int         `json:"top_speed,omitempty"`
	Acceleration   *float64     `json:"acceleration,omitempty"`
	EngineType     *string      `json:"engine_type,omitempty"`
	DrivetrainType *string      `json:"drivetrain_type,omitempty"`
	CurbWeight     *float64     `json:"curb_weight,omitempty"`
	Price          *int         `json:"price,omitempty"`
	Description    *string      `json:"description,omitempty"`
	Rarity         *int         `json:"rarity,omitempty"`
	LowResImage    *string      `json:"low_res_image,omitempty"`
	HighResImage   *string      `json:"high_res_image,omitempty"`
	DateCollected  *string      `json:"date_collected,omitempty"`
	LikesCount     int          `json:"likes_count"`
	Upgrades       []carUpgrade `json:"upgrades"`
}

type User struct {
	ID             int     `json:"id"`
	Email          *string `json:"email,omitempty"`
	DisplayName    *string `json:"display_name,omitempty"`
	FollowerCount  int     `json:"followers_count"`
	FriendCount    int     `json:"friend_count"`
	ProfilePicture *string `json:"profile_picture,omitempty"`
	IsFriend       bool    `json:"is_friend,omitempty"`
	IsPrivate      bool    `json:"is_private,omitempty"`
	CarCount       int     `json:"car_count,omitempty"`
	Currency       int     `json:"currency"`
	CarScore       int     `json:"car_score"` // Added car_score field
}

// SharedCar represents the public data for a shared car
type SharedCar struct {
	Make           string       `json:"make"`
	Model          string       `json:"model"`
	Year           string       `json:"year"`
	Color          string       `json:"color"`
	Trim           string       `json:"trim,omitempty"`
	Horsepower     *int         `json:"horsepower,omitempty"`
	Torque         *int         `json:"torque,omitempty"`
	TopSpeed       *int         `json:"top_speed,omitempty"`
	Acceleration   *float64     `json:"acceleration,omitempty"`
	EngineType     *string      `json:"engine_type,omitempty"`
	DrivetrainType *string      `json:"drivetrain_type,omitempty"`
	CurbWeight     *float64     `json:"curb_weight,omitempty"`
	Price          *int         `json:"price,omitempty"`
	Description    *string      `json:"description,omitempty"`
	Rarity         *int         `json:"rarity,omitempty"`
	LowResImage    string       `json:"low_res_image"`
	HighResImage   string       `json:"high_res_image"`
	DateCollected  string       `json:"date_collected"`
	LikesCount     int          `json:"likes_count"`
	ViewCount      int          `json:"view_count"`
	OwnerName      string       `json:"owner_name"`
	Upgrades       []carUpgrade `json:"upgrades"`
}

func (s *Service) GetCarCollection(ctx context.Context, userID, requestedUserID int, limit, offset int, sortBy string) ([]car, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching car collection - UserID: %d, RequestedUserID: %d, Limit: %d, Offset: %d, SortBy: %s",
		userID, requestedUserID, limit, offset, sortBy)

	var isPrivate bool
	if err := s.db.QueryRow(ctx, "SELECT is_private FROM users WHERE id = $1", requestedUserID).Scan(&isPrivate); err != nil {
		logger.Printf("Failed to check privacy settings for user %d: %v", requestedUserID, err)
		return nil, err
	}

	// Check friendship status for private collections
	if isPrivate && userID != requestedUserID {
		logger.Printf("Checking friendship status for private collection access (user: %d, requested: %d)", userID, requestedUserID)
		var count int
		checkQuery := `
			SELECT COUNT(*) FROM friends
			WHERE ((user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1))
			AND status = 'accepted'
		`
		if err := s.db.QueryRow(ctx, checkQuery, userID, requestedUserID).Scan(&count); err != nil {
			logger.Printf("Failed to check friendship status: %v", err)
			return nil, err
		}
		if count == 0 {
			logger.Printf("Access denied: users %d and %d are not friends", userID, requestedUserID)
			return []car{}, nil
		}
	}

	// Determine the ORDER BY clause based on the sortBy parameter
	orderByClause := "uc.id ASC" // default sort order
	switch sortBy {
	case "rarity":
		orderByClause = "c.rarity DESC"
	case "date_asc":
		orderByClause = "uc.date_collected ASC"
	case "date", "date_desc":
		orderByClause = "uc.date_collected DESC"
	case "name", "name_asc":
		orderByClause = "c.make ASC, c.model ASC"
	case "name_desc":
		orderByClause = "c.make DESC, c.model DESC"
	}

	// Base query
	query := `
		SELECT c.id, uc.id as user_car_id, uc.user_id, c.make, c.model, c.year, uc.color, c.trim,
		c.horsepower, c.torque, c.top_speed, c.acceleration, c.engine_type,
		c.drivetrain_type, c.curb_weight, c.price, c.description, c.rarity,
		uc.low_res_image, uc.high_res_image, uc.date_collected, uc.likes_count,
		COALESCE(
			jsonb_agg(
				jsonb_build_object(
					'id', cu.id,
					'upgrade_type', cu.upgrade_type,
					'active', cu.active,
					'metadata', cu.metadata,
					'created_at', TO_CHAR(cu.created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
					'updated_at', TO_CHAR(cu.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
				)
			) FILTER (WHERE cu.id IS NOT NULL AND cu.active = true),
			'[]'::jsonb
		) as upgrades
		FROM user_cars uc
		JOIN cars c ON uc.car_id = c.id
		LEFT JOIN car_upgrades cu ON uc.id = cu.user_car_id
		WHERE uc.user_id = $1
		GROUP BY c.id, uc.id, c.make, c.model, c.year, uc.color, c.trim,
		c.horsepower, c.torque, c.top_speed, c.acceleration, c.engine_type,
		c.drivetrain_type, c.curb_weight, c.price, c.description, c.rarity,
		uc.low_res_image, uc.high_res_image, uc.date_collected, uc.likes_count
		ORDER BY ` + orderByClause + ` LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(ctx, query, requestedUserID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cars []car
	for rows.Next() {
		var car car
		var upgradesJson []byte
		var dateCollected time.Time
		if err := rows.Scan(&car.ID, &car.UserCarID, &car.UserID, &car.Make, &car.Model, &car.Year,
			&car.Color, &car.Trim, &car.Horsepower, &car.Torque, &car.TopSpeed,
			&car.Acceleration, &car.EngineType, &car.DrivetrainType, &car.CurbWeight,
			&car.Price, &car.Description, &car.Rarity, &car.LowResImage,
			&car.HighResImage, &dateCollected, &car.LikesCount, &upgradesJson); err != nil {
			return nil, err
		}

		// Format the date using common.FormatTimestamp
		formattedDate := common.FormatTimestamp(dateCollected)
		car.DateCollected = &formattedDate

		if err := json.Unmarshal(upgradesJson, &car.Upgrades); err != nil {
			return nil, fmt.Errorf("failed to parse upgrades: %w", err)
		}
		cars = append(cars, car)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	logger.Printf("Successfully retrieved %d cars for user %d", len(cars), requestedUserID)
	return cars, nil
}

func (s *Service) GetSpecificUserCars(ctx context.Context, userCarIDs []int) ([]car, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching specific user cars - UserCarIDs: %v", userCarIDs)

	// Base query
	query := `
		SELECT c.id, uc.id as user_car_id, uc.user_id, c.make, c.model, c.year, uc.color, c.trim,
		c.horsepower, c.torque, c.top_speed, c.acceleration, c.engine_type,
		c.drivetrain_type, c.curb_weight, c.price, c.description, c.rarity,
		uc.low_res_image, uc.high_res_image, uc.date_collected, uc.likes_count,
		COALESCE(
			jsonb_agg(
				jsonb_build_object(
					'id', cu.id,
					'upgrade_type', cu.upgrade_type,
					'active', cu.active,
					'metadata', cu.metadata,
					'created_at', TO_CHAR(cu.created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
					'updated_at', TO_CHAR(cu.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
				)
			) FILTER (WHERE cu.id IS NOT NULL AND cu.active = true),
			'[]'::jsonb
		) as upgrades
		FROM user_cars uc
		JOIN cars c ON uc.car_id = c.id
		LEFT JOIN car_upgrades cu ON uc.id = cu.user_car_id
		WHERE uc.id = ANY($1)
		GROUP BY c.id, uc.id, uc.user_id, c.make, c.model, c.year, uc.color, c.trim,
		c.horsepower, c.torque, c.top_speed, c.acceleration, c.engine_type,
		c.drivetrain_type, c.curb_weight, c.price, c.description, c.rarity,
		uc.low_res_image, uc.high_res_image, uc.date_collected, uc.likes_count
		ORDER BY uc.id ASC
	`

	rows, err := s.db.Query(ctx, query, userCarIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cars []car
	for rows.Next() {
		var car car
		var upgradesJson []byte
		var dateCollected time.Time
		if err := rows.Scan(&car.ID, &car.UserCarID, &car.UserID, &car.Make, &car.Model, &car.Year,
			&car.Color, &car.Trim, &car.Horsepower, &car.Torque, &car.TopSpeed,
			&car.Acceleration, &car.EngineType, &car.DrivetrainType, &car.CurbWeight,
			&car.Price, &car.Description, &car.Rarity, &car.LowResImage,
			&car.HighResImage, &dateCollected, &car.LikesCount, &upgradesJson); err != nil {
			return nil, err
		}

		// Format the date using common.FormatTimestamp
		formattedDate := common.FormatTimestamp(dateCollected)
		car.DateCollected = &formattedDate

		if err := json.Unmarshal(upgradesJson, &car.Upgrades); err != nil {
			return nil, fmt.Errorf("failed to parse upgrades: %w", err)
		}
		cars = append(cars, car)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	logger.Printf("Successfully retrieved %d specific user cars", len(cars))
	return cars, nil
}

type FriendRequest struct {
	ID                   int     `json:"id"`
	UserID               int     `json:"user_id"`
	UserDisplayName      *string `json:"user_display_name"`
	UserProfilePicture   *string `json:"user_profile_picture"`
	FriendID             int     `json:"friend_id"`
	FriendDisplayName    *string `json:"friend_display_name"`
	FriendProfilePicture *string `json:"friend_profile_picture"`
}

func (s *Service) GetPendingFriendRequests(ctx context.Context, userID int) ([]FriendRequest, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching pending friend requests for user %d", userID)

	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.user_id, f.friend_id, u.profile_picture, u.display_name, uf.display_name, uf.profile_picture
		FROM friends f
		JOIN users u ON u.id = f.user_id
		JOIN users uf ON uf.id = f.friend_id
		WHERE (f.friend_id = $1 OR f.user_id = $1) AND f.status = 'pending'
	`, userID)
	if err != nil {
		logger.Printf("Failed to query pending friend requests: %v", err)
		return nil, err
	}
	defer rows.Close()

	var requests []FriendRequest
	for rows.Next() {
		var r FriendRequest
		if err := rows.Scan(&r.ID, &r.UserID, &r.FriendID, &r.UserProfilePicture, &r.UserDisplayName, &r.FriendDisplayName, &r.FriendProfilePicture); err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}
	logger.Printf("Found %d pending friend requests for user %d", len(requests), userID)
	return requests, rows.Err()
}

func (s *Service) GetUserInfo(ctx context.Context, requestedUserID, currentUserID int) (User, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching user info for user %d (requested by %d)", requestedUserID, currentUserID)

	var user User
	err := s.db.QueryRow(ctx, `
		WITH friend_status AS (
			SELECT EXISTS (
				SELECT 1 FROM friends 
				WHERE ((user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1))
				AND status = 'accepted'
			) as is_friend
		),
		car_count AS (
			SELECT COUNT(*) as count 
			FROM user_cars 
			WHERE user_id = $2
		),
		friend_count AS (
			SELECT COUNT(*) as count
			FROM friends
			WHERE (user_id = $2 OR friend_id = $2)
			AND status = 'accepted'
		),
		car_score_calc AS (
			SELECT 
				COALESCE(SUM(
					CASE 
						WHEN c.rarity = 1 THEN 10
						WHEN c.rarity = 2 THEN 25
						WHEN c.rarity = 3 THEN 50
						WHEN c.rarity = 4 THEN 100
						WHEN c.rarity = 5 THEN 200
						ELSE 0
					END
				), 0) as score
			FROM user_cars uc
			JOIN cars c ON uc.car_id = c.id
			WHERE uc.user_id = $2
		)
		SELECT u.id, u.display_name, u.followers_count, u.profile_picture, 
			   u.is_private, u.currency, f.is_friend, c.count, fc.count, cs.score
		FROM users u
		CROSS JOIN friend_status f
		CROSS JOIN car_count c
		CROSS JOIN friend_count fc
		CROSS JOIN car_score_calc cs
		WHERE u.id = $2
	`, currentUserID, requestedUserID).Scan(
		&user.ID, &user.DisplayName, &user.FollowerCount, &user.ProfilePicture,
		&user.IsPrivate, &user.Currency, &user.IsFriend, &user.CarCount, &user.FriendCount, &user.CarScore,
	)

	if err != nil {
		logger.Printf("Failed to fetch user info: %v", err)
		return User{}, fmt.Errorf("failed to get user info: %w", err)
	}

	// Only include email if user is requesting their own details
	if currentUserID == requestedUserID {
		var email string
		err := s.db.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", currentUserID).Scan(&email)
		if err == nil {
			user.Email = &email
		}
	}

	logger.Printf("Successfully retrieved info for user %d with car score %d", user.ID, user.CarScore)
	return user, nil
}

func (s *Service) UpdateProfilePicture(ctx context.Context, userID int, imageData []byte) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Updating profile picture for user %d - Image size: %d bytes", userID, len(imageData))

	// Decode and validate image dimensions
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		logger.Printf("Image decode failed for user %d: %v", userID, err)
		return fmt.Errorf("invalid image format")
	}

	bounds := img.Bounds()
	logger.Printf("Image dimensions for user %d - Width: %d, Height: %d",
		userID, bounds.Dx(), bounds.Dy())

	if bounds.Dx() != 512 || bounds.Dy() != 512 {
		return fmt.Errorf("image must be 512x512 pixels")
	}

	// Create directory if it doesn't exist
	dirPath := filepath.Join("_resources", "images", "profile_pictures")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		logger.Printf("Failed to create directory: %v", err)
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate a unique timestamp (milliseconds since epoch)
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	// Create filename with user ID and timestamp to break cache
	fileName := fmt.Sprintf("user_%d_%d.jpg", userID, timestamp)
	filePath := filepath.Join(dirPath, fileName)
	f, err := os.Create(filePath)
	if err != nil {
		logger.Printf("Failed to create file: %v", err)
		return fmt.Errorf("failed to save image: %w", err)
	}
	defer f.Close()

	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		logger.Printf("Failed to encode image: %v", err)
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Update database with the path including timestamp
	relativePath := fmt.Sprintf("images/profile_pictures/user_%d_%d.jpg", userID, timestamp)
	_, err = s.db.Exec(ctx, `
		UPDATE users 
		SET profile_picture = $1, 
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = $2`, relativePath, userID)
	if err != nil {
		logger.Printf("Failed to update profile picture path: %v", err)
		return fmt.Errorf("failed to update profile picture: %w", err)
	}

	logger.Printf("Successfully updated profile picture for user %d at path: %s", userID, relativePath)
	return nil
}

func (s *Service) SearchUsers(ctx context.Context, query string, currentUserID int) ([]User, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Searching users with query: %s", query)

	rows, err := s.db.Query(ctx, `
		WITH friend_status AS (
			SELECT friend_id, true as is_friend
			FROM friends 
			WHERE (user_id = $1 OR friend_id = $1)
			AND status = 'accepted'
		),
		car_counts AS (
			SELECT user_id, COUNT(*) as count
			FROM user_cars
			GROUP BY user_id
		)
		SELECT u.id, u.display_name, u.followers_count, u.profile_picture,
			   u.is_private, COALESCE(f.is_friend, false), COALESCE(c.count, 0)
		FROM users u
		LEFT JOIN friend_status f ON (u.id = f.friend_id)
		LEFT JOIN car_counts c ON (u.id = c.user_id)
		WHERE LOWER(u.display_name) LIKE LOWER($2)
		AND u.id != $1
		LIMIT 10
	`, currentUserID, "%"+query+"%")

	if err != nil {
		logger.Printf("Failed to search users: %v", err)
		return nil, fmt.Errorf("failed to search users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(
			&user.ID, &user.DisplayName, &user.FollowerCount, &user.ProfilePicture,
			&user.IsPrivate, &user.IsFriend, &user.CarCount,
		); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	logger.Printf("Found %d users matching query: %s", len(users), query)
	return users, rows.Err()
}

// SellCar removes a car from the user's collection and gives them currency based on rarity
func (s *Service) SellCar(ctx context.Context, userID int, userCarID int) (int, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Attempting to sell car - UserID: %d, UserCarID: %d", userID, userCarID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify car ownership and get rarity
	var rarity int
	err = tx.QueryRow(ctx, `
        SELECT c.rarity
        FROM user_cars uc
        JOIN cars c ON uc.car_id = c.id
        WHERE uc.id = $1 AND uc.user_id = $2
    `, userCarID, userID).Scan(&rarity)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("car not found or not owned by user")
		}
		return 0, fmt.Errorf("failed to verify car ownership: %w", err)
	}

	// Calculate currency to award
	currencyToAdd := common.GetCurrencyForRarity(rarity)

	// Update user's currency and transfer car ownership in a transaction
	_, err = tx.Exec(ctx, `
        UPDATE users 
        SET currency = currency + $1
        WHERE id = $2
    `, currencyToAdd, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to update user currency: %w", err)
	}

	// Transfer the car to system ownership (user_id = 0) instead of deleting
	_, err = tx.Exec(ctx, `
        UPDATE user_cars
        SET user_id = 0
        WHERE id = $1 AND user_id = $2
    `, userCarID, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to transfer car ownership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Printf("Successfully sold car. UserID: %d, UserCarID: %d, Currency earned: %d",
		userID, userCarID, currencyToAdd)
	return currencyToAdd, nil
}

// ImageUpgradeResult represents the result of upgrading a car's image
type ImageUpgradeResult struct {
	RemainingCurrency int
	HighResImage      string
	LowResImage       string
}

// UpgradeCarImage upgrades a car's image to a premium version
func (s *Service) UpgradeCarImage(ctx context.Context, userID int, userCarID int) (*ImageUpgradeResult, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Attempting to upgrade car image - UserID: %d, UserCarID: %d", userID, userCarID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get car details, verify ownership, and check subscription status
	var carID int
	var color string
	var year string
	var make, model, trim string
	var currentCurrency int
	var hasActiveSubscription bool
	var currentHighRes, currentLowRes string

	err = tx.QueryRow(ctx, `
        SELECT c.id, uc.color, c.year, c.make, c.model, c.trim, u.currency,
               COALESCE(us.is_active, false) as has_subscription, uc.high_res_image, uc.low_res_image
        FROM user_cars uc
        JOIN cars c ON uc.car_id = c.id
        JOIN users u ON uc.user_id = u.id
        LEFT JOIN user_subscriptions us ON u.id = us.user_id
        WHERE uc.id = $1 AND uc.user_id = $2
        AND (us.is_active = true AND (us.subscription_end IS NULL OR us.subscription_end > NOW()))
    `, userCarID, userID).Scan(&carID, &color, &year, &make, &model, &trim, &currentCurrency, &hasActiveSubscription, &currentHighRes, &currentLowRes)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Check if it's because of no subscription or no car
			var exists bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM user_cars 
					WHERE id = $1 AND user_id = $2
				)
			`, userCarID, userID).Scan(&exists); err != nil {
				return nil, fmt.Errorf("failed to check car existence: %w", err)
			}
			if exists {
				return nil, fmt.Errorf("active subscription required")
			}
			return nil, fmt.Errorf("car not found or not owned by user")
		}
		return nil, fmt.Errorf("failed to get car details: %w", err)
	}

	if !hasActiveSubscription {
		return nil, fmt.Errorf("active subscription required")
	}

	if currentCurrency < common.UpgradeCost {
		return nil, fmt.Errorf("insufficient currency")
	}

	// Generate background index
	backgroundIndex := rand.Intn(len(common.PREMIUM_BACKGROUNDS))
	selectedBackground := common.PREMIUM_BACKGROUNDS[backgroundIndex]

	// Create a timestamp for unique filenames
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	// Check if we already have a premium image for this car/user/background combination
	premiumDir := fmt.Sprintf("car_%d/premium/%d", carID, userID)
	premiumImageName := fmt.Sprintf("premium_%d_%d.jpg", backgroundIndex, timestamp)
	lowResPremiumImageName := fmt.Sprintf("low_res_%d_%d.jpg", backgroundIndex, timestamp)
	relativeHighResPath := filepath.Join("generated", premiumDir, premiumImageName)
	relativeLowResPath := filepath.Join("generated", premiumDir, lowResPremiumImageName)

	// Always generate a new premium image with the timestamp
	base64Image, err := common.GenerateCarImage(ctx, year, make, model, trim, color, true, selectedBackground)
	if err != nil {
		return nil, fmt.Errorf("failed to generate premium image: %w", err)
	}

	// Save high-res premium image
	if err := s.saveImage(base64Image, s.generatedSaveDir, filepath.Join(premiumDir, premiumImageName)); err != nil {
		return nil, fmt.Errorf("failed to save premium image: %w", err)
	}

	// Create and save low-res version
	absolutePremiumLowResPath := filepath.Join(s.generatedSaveDir, premiumDir, lowResPremiumImageName)
	if err := s.saveLowResImage(base64Image, absolutePremiumLowResPath); err != nil {
		return nil, fmt.Errorf("failed to create low res version: %w", err)
	}

	// Update user's currency
	_, err = tx.Exec(ctx, `
        UPDATE users 
        SET currency = currency - $1
        WHERE id = $2
    `, common.UpgradeCost, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to update user currency: %w", err)
	}

	// Update car images with relative paths for database
	_, err = tx.Exec(ctx, `
        UPDATE user_cars
        SET high_res_image = $1, low_res_image = $2
        WHERE id = $3 AND user_id = $4
    `, relativeHighResPath, relativeLowResPath, userCarID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to update car images: %w", err)
	}

	// Add upgrade record with background index and original image paths
	_, err = tx.Exec(ctx, `
        INSERT INTO car_upgrades (user_car_id, upgrade_type, metadata)
        VALUES ($1, 'premium_image', $2)
    `, userCarID, map[string]interface{}{
		"premium_low_res":   relativeLowResPath,
		"premium_high_res":  relativeHighResPath,
		"original_low_res":  currentLowRes,
		"original_high_res": currentHighRes,
		"background_index":  backgroundIndex,
		"timestamp":         timestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to record upgrade: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &ImageUpgradeResult{
		RemainingCurrency: currentCurrency - common.UpgradeCost,
		HighResImage:      relativeHighResPath,
		LowResImage:       relativeLowResPath,
	}, nil
}

// RevertCarImage reverts a car's image back to its original version
func (s *Service) RevertCarImage(ctx context.Context, userID int, userCarID int) (*ImageUpgradeResult, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Attempting to revert car image - UserID: %d, UserCarID: %d", userID, userCarID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// First, check if there's an active premium image upgrade and get its metadata
	var upgradeMetadata map[string]interface{}
	var originalHighRes, originalLowRes string
	var premiumHighRes, premiumLowRes string

	err = tx.QueryRow(ctx, `
        SELECT metadata
        FROM car_upgrades
        WHERE user_car_id = $1 AND upgrade_type = 'premium_image' AND active = true
        ORDER BY created_at DESC
        LIMIT 1
    `, userCarID).Scan(&upgradeMetadata)

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to get upgrade metadata: %w", err)
		}
		// No active upgrade found, check if the car belongs to the user
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM user_cars 
				WHERE id = $1 AND user_id = $2
			)
		`, userCarID, userID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("failed to verify car ownership: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("car not found or not owned by user")
		}
		// No upgrade to revert
		return nil, fmt.Errorf("no active image upgrade found")
	}

	// Extract original image paths from metadata
	if originalHigh, ok := upgradeMetadata["original_high_res"].(string); ok {
		originalHighRes = originalHigh
	}

	if originalLow, ok := upgradeMetadata["original_low_res"].(string); ok {
		originalLowRes = originalLow
	}

	if premiumHigh, ok := upgradeMetadata["premium_high_res"].(string); ok {
		premiumHighRes = premiumHigh
	}

	if premiumLow, ok := upgradeMetadata["premium_low_res"].(string); ok {
		premiumLowRes = premiumLow
	}

	if originalHighRes == "" || originalLowRes == "" {
		return nil, fmt.Errorf("failed to retrieve original image paths")
	}

	// Extract car ID for logging
	var carID int
	if err := tx.QueryRow(ctx, `
		SELECT car_id
		FROM user_cars
		WHERE id = $1 AND user_id = $2
	`, userCarID, userID).Scan(&carID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("car not found or not owned by user")
		}
		return nil, fmt.Errorf("failed to get car details: %w", err)
	}

	logger.Printf("Reverting car image for car ID %d - from premium paths %s and %s to original paths %s and %s",
		carID, premiumHighRes, premiumLowRes, originalHighRes, originalLowRes)

	// Update car images to original using relative paths from metadata
	_, err = tx.Exec(ctx, `
        UPDATE user_cars
        SET high_res_image = $1, low_res_image = $2
        WHERE id = $3 AND user_id = $4
    `, originalHighRes, originalLowRes, userCarID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to update car images: %w", err)
	}

	// Deactivate the upgrade record
	_, err = tx.Exec(ctx, `
        UPDATE car_upgrades 
        SET active = false, updated_at = CURRENT_TIMESTAMP
        WHERE user_car_id = $1 AND upgrade_type = 'premium_image' AND active = true
    `, userCarID)
	if err != nil {
		return nil, fmt.Errorf("failed to update upgrade status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &ImageUpgradeResult{
		HighResImage: originalHighRes,
		LowResImage:  originalLowRes,
	}, nil
}

// GetCarUpgrades returns all active upgrades for a user's car
func (s *Service) GetCarUpgrades(ctx context.Context, userID int, userCarID int) ([]carUpgrade, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching upgrades for car - UserID: %d, UserCarID: %d", userID, userCarID)

	// Verify car ownership
	var exists bool
	if err := s.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM user_cars 
            WHERE id = $1 AND user_id = $2
        )
    `, userCarID, userID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("failed to verify car ownership: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("car not found or not owned by user")
	}

	// Get active upgrades
	rows, err := s.db.Query(ctx, `
        SELECT id, upgrade_type, active, metadata, created_at, updated_at
        FROM car_upgrades
        WHERE user_car_id = $1 AND active = true
        ORDER BY created_at DESC
    `, userCarID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch upgrades: %w", err)
	}
	defer rows.Close()

	var upgrades []carUpgrade
	for rows.Next() {
		var upgrade carUpgrade
		var metadata []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&upgrade.ID, &upgrade.UpgradeType, &upgrade.Active, &metadata, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan upgrade: %w", err)
		}
		upgrade.CreatedAt = common.FormatTimestamp(createdAt)
		upgrade.UpdatedAt = common.FormatTimestamp(updatedAt)
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &upgrade.Metadata); err != nil {
				return nil, fmt.Errorf("failed to parse upgrade metadata: %w", err)
			}
		}
		upgrades = append(upgrades, upgrade)
	}
	return upgrades, nil
}

// saveImage wrapper to maintain compatibility
func (s *Service) saveImage(base64Image string, baseDir string, relativePath string) error {
	return common.SaveImage(base64Image, baseDir, relativePath, 0755)
}

// saveLowResImage wrapper to maintain compatibility
func (s *Service) saveLowResImage(originalImage string, savePath string) error {
	return common.SaveLowResVersion(originalImage, savePath, 95, 0755)
}

func (s *Service) UpdateDisplayName(ctx context.Context, userID int, newDisplayName string) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Updating display name for user %d", userID)

	// Validate display name length
	if len(newDisplayName) < 3 || len(newDisplayName) > 50 {
		return fmt.Errorf("display name must be between 3 and 50 characters")
	}

	// Get regex patterns from environment
	patterns := strings.Split(os.Getenv("BANNED_USERNAME_PATTERNS"), ",")
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		regex, err := regexp.Compile("(?i)" + strings.TrimSpace(pattern))
		if err != nil {
			logger.Printf("Invalid regex pattern %q: %v", pattern, err)
			continue
		}

		if regex.MatchString(newDisplayName) {
			return fmt.Errorf("display name contains inappropriate content")
		}
	}

	// Update the display name
	_, err := s.db.Exec(ctx, `
		UPDATE users 
		SET display_name = $1, 
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = $2`,
		newDisplayName, userID)

	if err != nil {
		logger.Printf("Failed to update display name: %v", err)
		return fmt.Errorf("failed to update display name: %w", err)
	}

	logger.Printf("Successfully updated display name for user %d", userID)
	return nil
}

// CreateShareLink generates a shareable link for any car
func (s *Service) CreateShareLink(ctx context.Context, requestingUserID int, userCarID int) (string, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Creating share link - RequestingUserID: %d, UserCarID: %d", requestingUserID, userCarID)

	// Get the car and its actual owner ID (don't verify ownership)
	var ownerID int
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT user_id, TRUE
		FROM user_cars 
		WHERE id = $1
	`, userCarID).Scan(&ownerID, &exists)

	if err != nil || !exists {
		logger.Printf("Failed to find car: %v", err)
		return "", fmt.Errorf("car not found")
	}

	logger.Printf("Found car %d owned by user %d", userCarID, ownerID)

	// Check if a share link already exists for this car
	var existingToken string
	err = s.db.QueryRow(ctx, `
		SELECT token FROM shared_cars 
		WHERE user_car_id = $1 AND expires_at > NOW()
	`, userCarID).Scan(&existingToken)

	if err == nil {
		// Return existing token
		logger.Printf("Found existing share token for car %d: %s", userCarID, existingToken)
		return existingToken, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		logger.Printf("Error checking for existing share token: %v", err)
		return "", fmt.Errorf("database error: %w", err)
	}

	// Generate a secure random token
	token := generateSecureToken(32)

	// Create a new share link that expires in 30 days
	expiryDate := time.Now().AddDate(0, 0, 30) // 30 days

	// Store who created the share link, but use the actual owner's ID
	_, err = s.db.Exec(ctx, `
		INSERT INTO shared_cars (user_id, user_car_id, token, expires_at)
		VALUES ($1, $2, $3, $4)
	`, ownerID, userCarID, token, expiryDate)

	if err != nil {
		logger.Printf("Failed to create share link: %v", err)
		return "", fmt.Errorf("failed to create share link: %w", err)
	}

	logger.Printf("Successfully created share token for car %d: %s", userCarID, token)
	return token, nil
}

// GetSharedCarByToken retrieves car data for a shared link
func (s *Service) GetSharedCarByToken(ctx context.Context, shareToken string) (*SharedCar, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Fetching shared car - Token: %s", shareToken)

	query := `
		SELECT c.make, c.model, c.year, uc.color, c.trim,
		    c.horsepower, c.torque, c.top_speed, c.acceleration, c.engine_type,
		    c.drivetrain_type, c.curb_weight, c.price, c.description, c.rarity,
		    uc.low_res_image, uc.high_res_image, uc.date_collected, 
		    uc.likes_count, sc.view_count, u.display_name,
		    COALESCE(
		        jsonb_agg(
		            jsonb_build_object(
		                'id', cu.id,
		                'upgrade_type', cu.upgrade_type,
		                'active', cu.active,
		                'metadata', cu.metadata,
		                'created_at', TO_CHAR(cu.created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		                'updated_at', TO_CHAR(cu.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		            )
		        ) FILTER (WHERE cu.id IS NOT NULL AND cu.active = true),
		        '[]'::jsonb
		    ) as upgrades
		FROM shared_cars sc
		JOIN user_cars uc ON sc.user_car_id = uc.id
		JOIN cars c ON uc.car_id = c.id
		JOIN users u ON uc.user_id = u.id
		LEFT JOIN car_upgrades cu ON uc.id = cu.user_car_id
		WHERE sc.token = $1 AND sc.expires_at > NOW()
		GROUP BY c.make, c.model, c.year, uc.color, c.trim,
		    c.horsepower, c.torque, c.top_speed, c.acceleration, c.engine_type,
		    c.drivetrain_type, c.curb_weight, c.price, c.description, c.rarity,
		    uc.low_res_image, uc.high_res_image, uc.date_collected, 
		    uc.likes_count, sc.view_count, u.display_name
	`

	var car SharedCar
	var upgradesJson []byte
	var dateCollected time.Time
	var lowResImage, highResImage string

	err := s.db.QueryRow(ctx, query, shareToken).Scan(
		&car.Make, &car.Model, &car.Year, &car.Color, &car.Trim,
		&car.Horsepower, &car.Torque, &car.TopSpeed, &car.Acceleration, &car.EngineType,
		&car.DrivetrainType, &car.CurbWeight, &car.Price, &car.Description, &car.Rarity,
		&lowResImage, &highResImage, &dateCollected,
		&car.LikesCount, &car.ViewCount, &car.OwnerName, &upgradesJson,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Printf("Share token not found or expired: %s", shareToken)
			return nil, fmt.Errorf("share token not found or expired")
		}
		logger.Printf("Failed to get shared car: %v", err)
		return nil, fmt.Errorf("failed to get shared car: %w", err)
	}

	// Format the date using common.FormatTimestamp
	car.DateCollected = common.FormatTimestamp(dateCollected)

	// Set image paths
	car.LowResImage = lowResImage
	if car.LowResImage == "" {
		car.LowResImage = "images/placeholder.jpg"
	}

	car.HighResImage = highResImage
	if car.HighResImage == "" {
		car.HighResImage = "images/placeholder.jpg"
	}

	if err := json.Unmarshal(upgradesJson, &car.Upgrades); err != nil {
		return nil, fmt.Errorf("failed to parse upgrades: %w", err)
	}

	logger.Printf("Successfully retrieved shared car for token: %s", shareToken)
	return &car, nil
}

// IncrementShareViews increases the view count for a shared car
func (s *Service) IncrementShareViews(ctx context.Context, shareToken string) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Incrementing view count for shared car - Token: %s", shareToken)

	_, err := s.db.Exec(ctx, `
		UPDATE shared_cars 
		SET view_count = view_count + 1 
		WHERE token = $1
	`, shareToken)

	if err != nil {
		logger.Printf("Failed to increment view count: %v", err)
		return fmt.Errorf("failed to update view count: %w", err)
	}

	return nil
}

// generateSecureToken creates a secure random token for sharing
func generateSecureToken(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// DeleteAccount permanently deletes a user's account and all associated data
func (s *Service) DeleteAccount(ctx context.Context, userID int) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("Anonymizing account for user %d", userID)

	// Generate unique placeholders based on timestamp
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	newEmail := fmt.Sprintf("deleted_%d@example.com", timestamp)
	newAuthID := fmt.Sprintf("deleted_%d", timestamp)

	// Anonymize user record
	_, err := s.db.Exec(ctx, `
		UPDATE users
		SET display_name = 'Deleted User',
		    full_name = 'Deleted User',
		    email = $1,
		    auth_provider_id = $2,
		    profile_picture = '',
		    is_private = true,
		    currency = 0,
		    is_private_email = true,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, newEmail, newAuthID, userID)
	if err != nil {
		logger.Printf("Failed to anonymize user %d: %v", userID, err)
		return fmt.Errorf("failed to anonymize user: %w", err)
	}

	logger.Printf("Successfully anonymized account for user %d", userID)
	return nil
}

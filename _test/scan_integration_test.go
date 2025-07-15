package test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	GOOD_IMAGE = filepath.Join("testdata", "test_car_good.jpeg")
	BAD_IMAGE  = filepath.Join("testdata", "test_car_bad.jpeg")
)

// Helper to load an image from given path.
func loadImage(t *testing.T, path string) string {
	imgBytes, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read test image")
	return base64.StdEncoding.EncodeToString(imgBytes)
}

func TestScanIntegration_GoodImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	// Set image generator URL to local server
	origBaseURL := os.Getenv("IMAGE_GEN_BASE_URL")
	os.Setenv("IMAGE_GEN_BASE_URL", "http://127.0.0.1:5000")
	defer os.Setenv("IMAGE_GEN_BASE_URL", origBaseURL)

	// Create a test user
	user := createTestUser(t)
	userId := createTestUserInDB(t, user)

	// Login to get access token
	resp, body := makeRequest(t, http.MethodPost, "/login",
		map[string]string{"email": user.Email, "password": user.Password}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var authResp struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(body, &authResp))

	// Load and encode good test image
	base64Image := loadImage(t, GOOD_IMAGE)

	// Make scan request with good image
	scanPayload := map[string]interface{}{
		"base64_image": base64Image,
	}
	resp, body = makeRequest(t, http.MethodPost, "/scan", scanPayload, authResp.AccessToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)

	// Update the scan response struct to include all new fields
	var scanResp struct {
		ID             int      `json:"id"`
		UserCarID      int      `json:"user_car_id"`
		Make           string   `json:"make"`
		Model          string   `json:"model"`
		Year           int      `json:"year"`
		Color          string   `json:"color"`
		Trim           string   `json:"trim,omitempty"`
		Horsepower     *int     `json:"horsepower,omitempty"`
		Torque         *int     `json:"torque,omitempty"`
		TopSpeed       *int     `json:"top_speed,omitempty"`
		Acceleration   *float64 `json:"acceleration,omitempty"`
		EngineType     *string  `json:"engine_type,omitempty"`
		DrivetrainType *string  `json:"drivetrain_type,omitempty"`
		CurbWeight     *float64 `json:"curb_weight,omitempty"`
		Price          *int     `json:"price,omitempty"`
		Description    *string  `json:"description,omitempty"`
		Rarity         *int     `json:"rarity,omitempty"`
		LowResImage    *string  `json:"low_res_image,omitempty"`
		HighResImage   *string  `json:"high_res_image,omitempty"`
	}
	require.NoError(t, json.Unmarshal(body, &scanResp))

	// Verify basic response fields
	require.NotZero(t, scanResp.ID)
	require.NotZero(t, scanResp.UserCarID)
	assert.NotEmpty(t, scanResp.Make)
	assert.NotEmpty(t, scanResp.Model)
	assert.NotZero(t, scanResp.Year)
	assert.NotEmpty(t, scanResp.Color)

	// Verify optional fields are present and have values
	assert.NotNil(t, scanResp.Horsepower)
	assert.NotNil(t, scanResp.Torque)
	assert.NotNil(t, scanResp.TopSpeed)
	assert.NotNil(t, scanResp.Acceleration)
	assert.NotNil(t, scanResp.EngineType)
	assert.NotNil(t, scanResp.DrivetrainType)
	assert.NotNil(t, scanResp.CurbWeight)
	assert.NotNil(t, scanResp.Price)
	assert.NotNil(t, scanResp.Description)
	assert.NotNil(t, scanResp.Rarity)
	assert.NotNil(t, scanResp.LowResImage)
	assert.NotNil(t, scanResp.HighResImage)

	// Verify car was added to database
	var carExists bool
	err := testDB.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM cars WHERE id = $1)",
		scanResp.ID).Scan(&carExists)
	require.NoError(t, err)
	require.True(t, carExists, "Car should exist in database")

	// Verify user_car was created
	var userCarExists bool
	err = testDB.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM user_cars WHERE id = $1 AND user_id = $2 AND car_id = $3)",
		scanResp.UserCarID, userId, scanResp.ID).Scan(&userCarExists)
	require.NoError(t, err)
	require.True(t, userCarExists, "User car association should exist in database")

	// Verify car images were saved
	var colorImages map[string]map[string]string
	err = testDB.QueryRow(ctx,
		"SELECT color_images FROM cars WHERE id = $1",
		scanResp.ID).Scan(&colorImages)
	require.NoError(t, err)
	require.NotNil(t, colorImages)

	colorKey := strings.ToLower(scanResp.Color)
	require.Contains(t, colorImages, colorKey)
	require.Contains(t, colorImages[colorKey], "high_res")
	require.Contains(t, colorImages[colorKey], "low_res")

	// Verify the image files exist
	highResPath := colorImages[colorKey]["high_res"]
	lowResPath := colorImages[colorKey]["low_res"]
	assert.FileExists(t, highResPath)
	assert.FileExists(t, lowResPath)
}

func TestScanIntegration_BadImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Set image generator URL to local server
	origBaseURL := os.Getenv("IMAGE_GEN_BASE_URL")
	os.Setenv("IMAGE_GEN_BASE_URL", "http://127.0.0.1:5000")
	defer os.Setenv("IMAGE_GEN_BASE_URL", origBaseURL)

	// Create a test user
	user := createTestUser(t)
	createTestUserInDB(t, user)

	// Login to get access token
	resp, body := makeRequest(t, http.MethodPost, "/login",
		map[string]string{"email": user.Email, "password": user.Password}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var authResp struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(body, &authResp))

	// Load and encode bad test image
	base64Image := loadImage(t, BAD_IMAGE)

	// Make scan request with bad image
	scanPayload := map[string]interface{}{
		"base64_image": base64Image,
	}
	resp, body = makeRequest(t, http.MethodPost, "/scan", scanPayload, authResp.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)

	// Verify no car was created in database
	var carCount int
	err := testDB.QueryRow(ctx,
		"SELECT COUNT(*) FROM cars").Scan(&carCount)
	require.NoError(t, err)
	assert.Zero(t, carCount, "No cars should be created for rejected images")
}

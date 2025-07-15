package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"CarBN/common"
	"CarBN/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const (
	baseURLEnv     = "TEST_BASE_URL"
	defaultURL     = "http://localhost:8080"
	truncateAllSQL = `
TRUNCATE TABLE 
    users,
    user_cars,
    friends,
    trades,
    posts,
    refresh_tokens
CASCADE;
`
)

var (
	testDB     *pgxpool.Pool
	httpClient = &http.Client{Timeout: 240 * time.Second}
	logger     *log.Logger
)

type testUser struct {
	ID          int
	Email       string
	Password    string
	DisplayName string
}

// cleanDatabase truncates all tables with proper context handling
func cleanDatabase(t testing.TB) {
	if testDB == nil {
		t.Fatal("Database connection is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, truncateAllSQL)
	require.NoError(t, err, "Failed to clean database")
}

// TestMain with improved context handling for DB setup
func TestMain(m *testing.M) {
	// Make sure we're not using any existing connection
	if postgres.DB != nil {
		postgres.DB.Close()
		postgres.DB = nil
	}
	logger = log.New(os.Stdout, "TEST: ", log.LstdFlags)
	// Verify connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	ctx = context.WithValue(ctx, common.LoggerCtxKey, logger)
	// Initialize DB connection with test-specific config
	postgres.InitDB(ctx)
	testDB = postgres.DB

	if testDB == nil {
		fmt.Printf("Failed to initialize database connection\n")
		os.Exit(1)
	}

	err := testDB.Ping(ctx)
	cancel()

	if err != nil {
		fmt.Printf("Failed to ping database: %v\n", err)
		os.Exit(1)
	}

	// Clean once before all tests
	cleanDatabase(nil)

	code := m.Run()

	// Cleanup database connection
	postgres.DB.Close()

	os.Exit(code)
}

func getBaseURL() string {
	if url := os.Getenv(baseURLEnv); url != "" {
		return url
	}
	return defaultURL
}

func createTestUser(t *testing.T) testUser {
	return testUser{
		Email:       fmt.Sprintf("user-%s@test.com", uuid.New().String()),
		Password:    "SecurePass123!",
		DisplayName: "Test User",
	}
}

// makeRequest sends JSON payload to an endpoint with context timeout
func makeRequest(t *testing.T, method, path string, payload interface{}, token string) (*http.Response, []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	// Ensure logger is set in context
	ctx = context.WithValue(ctx, common.LoggerCtxKey, logger)

	bodyBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, method, getBaseURL()+path, bytes.NewReader(bodyBytes))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, respBody
}

// createTestUserInDB creates a test user with proper context management
func createTestUserInDB(t *testing.T, user testUser) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(user.Password), 12)
	require.NoError(t, err)

	var userID int
	err = testDB.QueryRow(ctx, `
		INSERT INTO users (email, password, display_name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, user.Email, string(hashedBytes), user.DisplayName).Scan(&userID)
	require.NoError(t, err)

	return userID
}

// loginUser logs in a test user and returns the access token
func loginUser(t *testing.T, email, password string) string {
	resp, body := makeRequest(t, http.MethodPost, "/login",
		map[string]string{"email": email, "password": password}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var authResp struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(body, &authResp))
	return authResp.AccessToken
}

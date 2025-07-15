package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthIntegration(t *testing.T) {
	// Parent context for all subtests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("RegisterLoginFlow", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create test user data
		user := createTestUser(t)

		// Test registration
		regPayload := map[string]string{
			"email":        user.Email,
			"password":     user.Password,
			"display_name": user.DisplayName,
		}
		resp, body := makeRequest(t, http.MethodPost, "/register", regPayload, "")
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)

		// Verify user exists in database
		var exists bool
		err := testDB.QueryRow(subCtx,
			"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)",
			user.Email).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists)

		// Test login
		loginPayload := map[string]string{
			"email":    user.Email,
			"password": user.Password,
		}
		resp, body = makeRequest(t, http.MethodPost, "/login", loginPayload, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var loginResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		require.NoError(t, json.Unmarshal(body, &loginResp))
		assert.NotEmpty(t, loginResp.AccessToken)
		assert.NotEmpty(t, loginResp.RefreshToken)

		// Verify refresh token in database
		err = testDB.QueryRow(subCtx,
			"SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token = $1)",
			loginResp.RefreshToken).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("RefreshTokenFlow", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create and login user
		user := createTestUser(t)
		createTestUserInDB(t, user)

		resp, body := makeRequest(t, http.MethodPost, "/login",
			map[string]string{"email": user.Email, "password": user.Password}, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var loginResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		require.NoError(t, json.Unmarshal(body, &loginResp))

		// Test token refresh
		resp, body = makeRequest(t, http.MethodPost, "/refresh",
			map[string]string{"refresh_token": loginResp.RefreshToken}, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var refreshResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		require.NoError(t, json.Unmarshal(body, &refreshResp))
		assert.NotEmpty(t, refreshResp.AccessToken)
		assert.NotEmpty(t, refreshResp.RefreshToken)
		assert.NotEqual(t, loginResp.RefreshToken, refreshResp.RefreshToken)

		// Verify old refresh token is invalidated
		var exists bool
		err := testDB.QueryRow(subCtx,
			"SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token = $1)",
			loginResp.RefreshToken).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists)

		// Verify new refresh token exists
		err = testDB.QueryRow(subCtx,
			"SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token = $1)",
			refreshResp.RefreshToken).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("LogoutFlow", func(t *testing.T) {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		defer subCancel()

		// Create and login user
		user := createTestUser(t)
		createTestUserInDB(t, user)

		resp, body := makeRequest(t, http.MethodPost, "/login",
			map[string]string{"email": user.Email, "password": user.Password}, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var loginResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		require.NoError(t, json.Unmarshal(body, &loginResp))

		// Test logout
		resp, _ = makeRequest(t, http.MethodPost, "/logout",
			map[string]string{"refresh_token": loginResp.RefreshToken},
			loginResp.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify refresh token is removed
		var exists bool
		err := testDB.QueryRow(subCtx,
			"SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token = $1)",
			loginResp.RefreshToken).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists)

		// // Verify protected endpoint access is denied
		// resp, _ = makeRequest(t, http.MethodGet, "/user/details", nil, loginResp.AccessToken)
		// require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

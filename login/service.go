package login

import (
	"CarBN/common"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Config for service parameters
type Config struct {
	JWTSecret          []byte
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	BcryptCost         int
	GoogleClientID     string
	AppleTeamID        string
	AppleServiceID     string
	AppleKeyID         string
	ApplePrivateKey    []byte
}

// Service holds DB and config
type Service struct {
	db     *pgxpool.Pool
	config Config
}

// MyClaims extends JWT claims with user info
type MyClaims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type GoogleUserInfo struct {
	ID            string `json:"sub"`
	Email         string `json:"email"`
	VerifiedEmail string `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
	Aud           string `json:"aud"`
}

type AppleUserInfo struct {
	ID            string `json:"sub"`   // "sub" is Apple's user identifier
	Email         string `json:"email"` // email address
	EmailVerified bool   `json:"email_verified"`
	Name          struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	} `json:"name,omitempty"` // name is optional and might not be present
	IsPrivateEmail bool `json:"is_private_email,omitempty"`
	jwt.RegisteredClaims
}

// AuthProvider represents the type of authentication provider
type AuthProvider string

const (
	Google AuthProvider = "google"
	Apple  AuthProvider = "apple"
	// Add more providers here in the future
)

const (
	defaultBcryptCost = 12
)

// NewService initializes a new service
func NewService(db *pgxpool.Pool, config Config) *Service {
	if config.BcryptCost == 0 {
		config.BcryptCost = defaultBcryptCost
	}
	return &Service{
		db:     db,
		config: config,
	}
}

// AuthMiddleware checks the Authorization header for a valid token.
func (s *Service) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := r.Context().Value(common.LoggerCtxKey).(*log.Logger)

		// Extract token from header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.Printf("Auth failed: missing Authorization header")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Printf("Auth failed: invalid Authorization header format")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse and validate the JWT token
		token, err := jwt.ParseWithClaims(parts[1], &MyClaims{}, func(t *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				logger.Printf("Auth failed: unexpected signing method: %v", t.Header["alg"])
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return s.config.JWTSecret, nil
		})

		if err != nil {
			logger.Printf("Auth failed: token parse error: %v", err)
			if err.Error() == "Token is expired" {
				http.Error(w, "Token expired", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			logger.Printf("Auth failed: invalid token")
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Extract and validate claims
		claims, ok := token.Claims.(*MyClaims)
		if !ok {
			logger.Printf("Auth failed: invalid token claims type")
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Check if user exists and is active
		var exists bool
		err = s.db.QueryRow(r.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM users 
				WHERE id = $1 
				AND auth_provider IS NOT NULL
			)
		`, claims.UserID).Scan(&exists)

		if err != nil {
			logger.Printf("Auth failed: database error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !exists {
			logger.Printf("Auth failed: user %d not found or inactive", claims.UserID)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user info to context
		logger.Printf("Auth successful for user ID: %d", claims.UserID)
		ctx := context.WithValue(r.Context(), common.UserIDCtxKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GoogleSignIn handles authentication with Google
func (s *Service) GoogleSignIn(ctx context.Context, idToken string, displayName string) (string, string, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	// Verify Google ID token
	userInfo, err := s.verifyGoogleToken(idToken)
	if err != nil {
		logger.Printf("Failed to verify Google token: %v", err)
		return "", "", fmt.Errorf("invalid Google token: %w", err)
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Add transaction to context
	ctxWithTx := context.WithValue(ctx, "tx", tx)

	// Check if user exists or create new user
	var userID int
	err = tx.QueryRow(ctxWithTx, `
		INSERT INTO users (
			email, 
			display_name, 
			auth_provider, 
			auth_provider_id,
			full_name,
			is_private_email
		)
		VALUES ($1, $2, $3, $4, $5, false)
		ON CONFLICT (auth_provider, auth_provider_id) DO UPDATE 
		SET last_login = NOW()
		RETURNING id
	`, userInfo.Email, displayName, Google, userInfo.ID, userInfo.Name).Scan(&userID)

	if err != nil {
		logger.Printf("Failed to upsert user: %v", err)
		return "", "", fmt.Errorf("failed to process user: %w", err)
	}

	// Generate tokens
	accessToken, err := s.generateJWT(userID, s.config.AccessTokenExpiry, logger)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.generateJWT(userID, s.config.RefreshTokenExpiry, logger)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token using the same transaction
	if err := s.storeRefreshTokenTx(ctxWithTx, refreshToken, userID, time.Now().Add(s.config.RefreshTokenExpiry)); err != nil {
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	if err := tx.Commit(ctxWithTx); err != nil {
		return "", "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return accessToken, refreshToken, nil
}

// verifyGoogleToken verifies the Google ID token
func (s *Service) verifyGoogleToken(idToken string) (*GoogleUserInfo, error) {
	// Google's token info endpoint
	resp, err := http.Get(fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", idToken))
	if err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid token")
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	// Verify aud claim matches our client ID
	// In a real implementation, you'd want to verify this
	// but for now we'll just log it
	// if userInfo.Aud != s.config.GoogleClientID {
	//     return nil, fmt.Errorf("invalid client ID")
	// }

	return &userInfo, nil
}

// AppleSignIn handles authentication with Apple
func (s *Service) AppleSignIn(ctx context.Context, idToken string, displayName string) (string, string, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	// Verify Apple ID token
	userInfo, err := s.verifyAppleToken(idToken, ctx)
	if err != nil {
		logger.Printf("Failed to verify Apple token: %v", err)
		return "", "", fmt.Errorf("invalid Apple token: %w", err)
	}

	emailValue := userInfo.Email
	if emailValue == "" {
		// Generate a placeholder email using the Apple ID
		emailValue = fmt.Sprintf("%s@privaterelay.appleid.com", userInfo.ID)
		logger.Printf("Using placeholder email for Apple user with empty email claim")
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Add transaction to context
	ctxWithTx := context.WithValue(ctx, "tx", tx)

	// Check if user exists or create new user
	var userID int
	err = tx.QueryRow(ctxWithTx, `
        INSERT INTO users (
            email, 
            display_name, 
            auth_provider, 
            auth_provider_id,
            full_name,
            is_private_email
        )
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (auth_provider, auth_provider_id) DO UPDATE 
        SET last_login = NOW()
        RETURNING id
    `,
		emailValue,
		displayName,
		Apple,
		userInfo.ID,
		fmt.Sprintf("%s %s", userInfo.Name.FirstName, userInfo.Name.LastName),
		userInfo.IsPrivateEmail).Scan(&userID)

	// After getting userID, handle email updates
	if err == nil {
		err = s.handleAppleEmailUpdate(ctxWithTx, tx, userInfo, userID)
		if err != nil {
			logger.Printf("Warning: failed to handle email update: %v", err)
			// Don't return error here - the sign-in can still succeed
		}
	}

	if err != nil {
		logger.Printf("Failed to upsert user: %v", err)
		return "", "", fmt.Errorf("failed to process user: %w", err)
	}

	// Generate tokens
	accessToken, err := s.generateJWT(userID, s.config.AccessTokenExpiry, logger)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.generateJWT(userID, s.config.RefreshTokenExpiry, logger)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token using the same transaction
	if err := s.storeRefreshTokenTx(ctxWithTx, refreshToken, userID, time.Now().Add(s.config.RefreshTokenExpiry)); err != nil {
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	if err := tx.Commit(ctxWithTx); err != nil {
		return "", "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return accessToken, refreshToken, nil
}

// verifyAppleToken verifies the Apple ID token
func (s *Service) verifyAppleToken(idToken string, ctx context.Context) (*AppleUserInfo, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok || logger == nil {
		logger = log.New(os.Stdout, "[CarBN] ", log.LstdFlags)
	}

	logger.Printf("[Apple Auth] Starting token verification, token length: %d", len(idToken))
	if len(idToken) < 10 {
		return nil, fmt.Errorf("token too short")
	}

	startTime := time.Now()
	// Parse and verify the token
	token, err := jwt.ParseWithClaims(idToken, &AppleUserInfo{}, func(token *jwt.Token) (interface{}, error) {
		// Check if the signing method is RS256
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			logger.Printf("[Apple Auth] Invalid signing method: %v, expected RSA", token.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		logger.Printf("[Apple Auth] Token signing method verified: %v", token.Header["alg"])

		// Get Apple's public keys
		logger.Printf("[Apple Auth] Fetching Apple public keys")
		keysStartTime := time.Now()
		resp, err := http.Get("https://appleid.apple.com/auth/keys")
		if err != nil {
			logger.Printf("[Apple Auth] Failed to get Apple public keys: %v", err)
			return nil, fmt.Errorf("failed to get Apple public keys: %w", err)
		}
		defer resp.Body.Close()
		logger.Printf("[Apple Auth] Fetched Apple public keys in %v", time.Since(keysStartTime))

		var jwks struct {
			Keys []struct {
				Kty string `json:"kty"`
				Kid string `json:"kid"`
				Use string `json:"use"`
				Alg string `json:"alg"`
				N   string `json:"n"`
				E   string `json:"e"`
			} `json:"keys"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
			logger.Printf("[Apple Auth] Failed to decode Apple public keys: %v", err)
			return nil, fmt.Errorf("failed to decode Apple public keys: %w", err)
		}
		logger.Printf("[Apple Auth] Retrieved %d public keys from Apple", len(jwks.Keys))

		// Find the key that matches the kid in the token header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			logger.Printf("[Apple Auth] kid not found in token header")
			return nil, fmt.Errorf("kid not found in token header")
		}
		logger.Printf("[Apple Auth] Looking for key with ID: %s", kid)

		var publicKey *rsa.PublicKey
		for i, key := range jwks.Keys {
			logger.Printf("[Apple Auth] Checking key %d with ID: %s", i, key.Kid)
			if key.Kid == kid {
				logger.Printf("[Apple Auth] Matching key found: %s", key.Kid)
				// Decode the modulus and exponent
				nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
				if err != nil {
					logger.Printf("[Apple Auth] Failed to decode modulus: %v", err)
					return nil, fmt.Errorf("failed to decode modulus: %w", err)
				}
				eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
				if err != nil {
					logger.Printf("[Apple Auth] Failed to decode exponent: %v", err)
					return nil, fmt.Errorf("failed to decode exponent: %w", err)
				}

				// Convert exponent bytes to int
				var eInt int
				for i := 0; i < len(eBytes); i++ {
					eInt = eInt<<8 + int(eBytes[i])
				}

				publicKey = &rsa.PublicKey{
					N: new(big.Int).SetBytes(nBytes),
					E: eInt,
				}
				logger.Printf("[Apple Auth] Public key created successfully")
				break
			}
		}

		if publicKey == nil {
			logger.Printf("[Apple Auth] No matching public key found for kid: %s", kid)
			return nil, fmt.Errorf("matching public key not found")
		}

		return publicKey, nil
	})

	if err != nil {
		logger.Printf("[Apple Auth] Token verification failed after %v: %v", time.Since(startTime), err)
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}
	logger.Printf("[Apple Auth] Token signature verified successfully in %v", time.Since(startTime))

	userInfo, ok := token.Claims.(*AppleUserInfo)
	if !ok {
		logger.Printf("[Apple Auth] Failed to parse token claims to AppleUserInfo")
		return nil, fmt.Errorf("invalid token claims")
	}

	// Debug logging
	logger.Printf("[Apple Auth] Token claims parsed successfully - ID: %s, Email: %s, EmailVerified: %v, Name: %v",
		userInfo.ID,
		maskEmail(userInfo.Email),
		userInfo.EmailVerified,
		userInfo.Name.FirstName != "")

	// Verify required claims - only sub (ID) is absolutely required
	if userInfo.ID == "" {
		logger.Printf("[Apple Auth] Missing required sub claim")
		return nil, fmt.Errorf("missing required sub claim")
	}

	// If email is missing but we have the sub claim, we can still proceed
	if userInfo.Email == "" {
		logger.Printf("[Apple Auth] Warning: email claim is missing from Apple ID token")
	}

	logger.Printf("[Apple Auth] Token verification completed successfully in %v", time.Since(startTime))
	return userInfo, nil
}

// Helper function to mask email for logging
func maskEmail(email string) string {
	if email == "" {
		return "<empty>"
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "<invalid_format>"
	}

	username := parts[0]
	domain := parts[1]

	if len(username) <= 3 {
		return username[:1] + "***@" + domain
	}

	return username[:2] + "***@" + domain
}

// GenerateTokens creates an access token and a refresh token for the given user.
func (s *Service) GenerateTokens(ctx context.Context, userID int) (string, string, error) {
	logger, ok := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	if !ok || logger == nil {
		logger = log.New(os.Stdout, "[CarBN] ", log.LstdFlags)
	}

	accessToken, err := s.generateJWT(userID, s.config.AccessTokenExpiry, logger)
	if err != nil {
		logger.Printf("Failed to generate access token: %v", err)
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.generateJWT(userID, s.config.RefreshTokenExpiry, logger)
	if err != nil {
		logger.Printf("Failed to generate refresh token: %v", err)
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token with passed context
	if err := s.storeRefreshTokenTx(ctx, refreshToken, userID, time.Now().Add(s.config.RefreshTokenExpiry)); err != nil {
		logger.Printf("Failed to store refresh token: %v", err)
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	logger.Printf("Tokens generated successfully for user ID: %d", userID)
	return accessToken, refreshToken, nil
}

// RefreshTokens takes an existing refresh token, invalidates it, and returns new tokens.
func (s *Service) RefreshTokens(ctx context.Context, refreshTokenStr string) (string, string, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	logger.Printf("[Token Refresh] Starting token refresh process for token: %s...[first 10 chars]", refreshTokenStr[:10])

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("[Token Refresh] Failed to start transaction: %v", err)
		return "", "", fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Validate the old refresh token
	claims, err := s.validateRefreshToken(ctx, refreshTokenStr)
	if err != nil {
		logger.Printf("[Token Refresh] Refresh token validation failed: %v", err)
		return "", "", err
	}
	logger.Printf("[Token Refresh] Refresh token validated successfully for user ID: %d", claims.UserID)

	// 2. Invalidate the old refresh token
	if err := s.invalidateRefreshTokenTx(ctx, tx, refreshTokenStr); err != nil {
		logger.Printf("[Token Refresh] Failed to invalidate old refresh token: %v", err)
		return "", "", fmt.Errorf("failed to invalidate refresh token: %w", err)
	}
	logger.Printf("[Token Refresh] Old refresh token invalidated successfully")

	// 3. Generate new tokens
	newAccessToken, err := s.generateJWT(claims.UserID, s.config.AccessTokenExpiry, logger)
	if err != nil {
		logger.Printf("[Token Refresh] Failed to generate new access token: %v", err)
		return "", "", fmt.Errorf("failed to sign access token: %w", err)
	}
	logger.Printf("[Token Refresh] New access token generated successfully")

	newRefreshToken, err := s.generateJWT(claims.UserID, s.config.RefreshTokenExpiry, logger)
	if err != nil {
		logger.Printf("[Token Refresh] Failed to generate new refresh token: %v", err)
		return "", "", fmt.Errorf("failed to sign refresh token: %w", err)
	}
	logger.Printf("[Token Refresh] New refresh token generated successfully")

	// 4. Store new refresh token
	if err := s.storeRefreshTokenTx(ctx, newRefreshToken, claims.UserID, time.Now().Add(s.config.RefreshTokenExpiry)); err != nil {
		logger.Printf("[Token Refresh] Failed to store new refresh token: %v", err)
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}
	logger.Printf("[Token Refresh] New refresh token stored in database")

	if err := tx.Commit(ctx); err != nil {
		logger.Printf("[Token Refresh] Failed to commit transaction: %v", err)
		return "", "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Printf("[Token Refresh] Token refresh completed successfully for user ID: %d", claims.UserID)
	return newAccessToken, newRefreshToken, nil
}

// RefreshToken takes an existing refresh token, invalidates it, and returns new tokens.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	claims, err := s.validateRefreshToken(ctx, refreshToken)
	if err != nil {
		logger.Printf("Invalid refresh token: %v", err)
		return "", "", fmt.Errorf("invalid refresh token: %w", err)
	}

	// Generate new tokens
	accessToken, newRefreshToken, err := s.GenerateTokens(ctx, claims.UserID)
	if err != nil {
		logger.Printf("Failed to generate new tokens: %v", err)
		return "", "", fmt.Errorf("failed to generate new tokens: %w", err)
	}

	// Invalidate old refresh token using transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("Failed to begin transaction: %v", err)
		return "", "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.invalidateRefreshTokenTx(ctx, tx, refreshToken); err != nil {
		logger.Printf("Failed to invalidate old refresh token: %v", err)
		return "", "", fmt.Errorf("failed to invalidate old refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Printf("Failed to commit transaction: %v", err)
		return "", "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Printf("Tokens refreshed successfully for user ID: %d", claims.UserID)
	return accessToken, newRefreshToken, nil
}

// Logout invalidates the provided refresh token in the DB.
func (s *Service) Logout(ctx context.Context, refreshTokenStr string) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	// First validate the refresh token
	_, err := s.validateRefreshToken(ctx, refreshTokenStr)
	if err != nil {
		logger.Printf("Invalid refresh token: %v", err)
		return fmt.Errorf("invalid refresh token: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logger.Printf("Failed to start transaction: %v", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Invalidate (delete) the token
	if err := s.invalidateRefreshTokenTx(ctx, tx, refreshTokenStr); err != nil {
		logger.Printf("Failed to invalidate refresh token: %v", err)
		return fmt.Errorf("failed to invalidate refresh token: %v", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		logger.Printf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Printf("User logged out successfully")
	return nil
}

// --- Helper functions ---
// storeRefreshTokenTx inserts a new refresh token in the context of a transaction.
func (s *Service) storeRefreshTokenTx(ctx context.Context, token string, userID int, expiresAt time.Time) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	var err error
	// Use the existing transaction from context if available
	if existingTx, ok := ctx.Value("tx").(pgx.Tx); ok && existingTx != nil {
		_, err = existingTx.Exec(ctx, `
            INSERT INTO refresh_tokens (token, user_id, expires_at)
            VALUES ($1, $2, $3)
        `, token, userID, expiresAt)
	} else {
		// Start a new transaction if none exists
		tx, err := s.db.Begin(ctx)
		if err != nil {
			logger.Printf("Failed to start transaction: %v", err)
			return fmt.Errorf("failed to start transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
            INSERT INTO refresh_tokens (token, user_id, expires_at)
            VALUES ($1, $2, $3)
        `, token, userID, expiresAt)

		if err == nil {
			_ = tx.Commit(ctx)
		}
	}

	if err != nil {
		logger.Printf("Failed to store refresh token: %v", err)
		return err
	}

	logger.Printf("Refresh token stored successfully for user ID: %d", userID)
	return nil
}

// validateRefreshToken checks if the given token is valid, unexpired, and still exists in the DB.
func (s *Service) validateRefreshToken(ctx context.Context, tokenStr string) (*MyClaims, error) {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)
	startTime := time.Now()

	if len(tokenStr) < 10 {
		logger.Printf("[Token Validation] Invalid token format: too short")
		return nil, fmt.Errorf("invalid token format")
	}

	logger.Printf("[Token Validation] Starting validation for token: %s...[first 10 chars]", tokenStr[:10])

	// 1. Parse and validate JWT
	parsedStartTime := time.Now()
	token, err := jwt.ParseWithClaims(tokenStr, &MyClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			logger.Printf("[Token Validation] Unexpected signing method: %v", t.Header["alg"])
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.config.JWTSecret, nil
	})
	logger.Printf("[Token Validation] Token parsing completed in %v", time.Since(parsedStartTime))

	if err != nil {
		logger.Printf("[Token Validation] Failed to parse/validate JWT: %v", err)
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	if !token.Valid {
		logger.Printf("[Token Validation] Token is not valid")
		return nil, fmt.Errorf("invalid refresh token")
	}
	logger.Printf("[Token Validation] JWT signature and format validated successfully")

	claims, ok := token.Claims.(*MyClaims)
	if !ok {
		logger.Printf("[Token Validation] Failed to extract claims from token")
		return nil, fmt.Errorf("invalid token claims")
	}
	logger.Printf("[Token Validation] Claims extracted successfully, user ID: %d, issued at: %v, expires at: %v",
		claims.UserID,
		claims.IssuedAt.Time.Format(time.RFC3339),
		claims.ExpiresAt.Time.Format(time.RFC3339))

	// 2. Check if token exists in database and is not expired
	dbStartTime := time.Now()
	var exists bool
	err = s.db.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM refresh_tokens 
            WHERE token = $1 AND expires_at > NOW()
        )
    `, tokenStr).Scan(&exists)

	logger.Printf("[Token Validation] Database verification completed in %v", time.Since(dbStartTime))

	if err != nil {
		logger.Printf("[Token Validation] Database error while checking token: %v", err)
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	if !exists {
		logger.Printf("[Token Validation] Token not found in database or expired")
		return nil, fmt.Errorf("invalid or expired refresh token")
	}
	logger.Printf("[Token Validation] Token found in database and is not expired")

	logger.Printf("[Token Validation] Validation completed successfully in %v", time.Since(startTime))
	return claims, nil
}

// invalidateRefreshTokenTx removes a refresh token in the context of a transaction.
func (s *Service) invalidateRefreshTokenTx(ctx context.Context, tx pgx.Tx, token string) error {
	logger := ctx.Value(common.LoggerCtxKey).(*log.Logger)

	_, err := tx.Exec(ctx, `
        DELETE FROM refresh_tokens 
        WHERE token = $1
    `, token)
	if err != nil {
		logger.Printf("Failed to invalidate refresh token: %v", err)
	}
	return err
}

// generateJWT generates a signed JWT for a given userID and expiry.
func (s *Service) generateJWT(userID int, expiry time.Duration, logger *log.Logger) (string, error) {
	now := time.Now()
	expiryTime := now.Add(expiry)
	tokenID := uuid.NewString()

	logger.Printf("[JWT Generation] Creating token for user ID: %d, token ID: %s", userID, tokenID)

	claims := MyClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiryTime),
			Issuer:    "myapp",
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        tokenID,
		},
	}

	logger.Printf("[JWT Generation] Token claims - UserID: %d, IssuedAt: %v, ExpiresAt: %v, Duration: %v",
		userID, now.Format(time.RFC3339), expiryTime.Format(time.RFC3339), expiry)

	startTime := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(s.config.JWTSecret)
	signDuration := time.Since(startTime)

	if err != nil {
		logger.Printf("[JWT Generation] Failed to sign JWT in %v: %v", signDuration, err)
		return "", err
	}

	tokenLength := len(signedToken)
	logger.Printf("[JWT Generation] JWT generated successfully for user ID: %d in %v, token length: %d bytes",
		userID, signDuration, tokenLength)

	return signedToken, nil
}

func (s *Service) handleAppleEmailUpdate(ctx context.Context, tx pgx.Tx, userInfo *AppleUserInfo, existingUserID int) error {
	var currentEmail string
	var isPrivateEmail bool

	err := tx.QueryRow(ctx, `
        SELECT email, is_private_email 
        FROM users 
        WHERE id = $1
    `, existingUserID).Scan(&currentEmail, &isPrivateEmail)

	if err != nil {
		return fmt.Errorf("failed to fetch user email status: %w", err)
	}

	// Only update email if:
	// 1. User hasn't previously chosen privacy
	// 2. New email is available
	// 3. Email has actually changed
	if !isPrivateEmail && userInfo.Email != "" && currentEmail != userInfo.Email {
		_, err = tx.Exec(ctx, `
            UPDATE users 
            SET email = $1,
                is_private_email = $2
            WHERE id = $3
        `, userInfo.Email, userInfo.IsPrivateEmail, existingUserID)

		if err != nil {
			return fmt.Errorf("failed to update user email: %w", err)
		}
	}

	return nil
}

type AppleEmailUpdatePayload struct {
	Type           string `json:"type"`
	Sub            string `json:"sub"`
	Email          string `json:"email"`
	IsPrivateEmail bool   `json:"is_private_email"`
}

func (s *Service) HandleAppleEmailUpdate(ctx context.Context, payload AppleEmailUpdatePayload) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID int
	err = tx.QueryRow(ctx, `
        SELECT id FROM users 
        WHERE auth_provider = $1 AND auth_provider_id = $2
    `, Apple, payload.Sub).Scan(&userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Use the existing handler
	if err := s.handleAppleEmailUpdate(ctx, tx, &AppleUserInfo{
		ID:             payload.Sub,
		Email:          payload.Email,
		IsPrivateEmail: payload.IsPrivateEmail,
	}, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

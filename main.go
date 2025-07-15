package main

import (
	"CarBN/common"
	"CarBN/feed"
	"CarBN/friends"
	"CarBN/likes"
	"CarBN/login"
	"CarBN/postgres"
	"CarBN/scan"
	"CarBN/subscription"
	"CarBN/trade"
	"CarBN/user"
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type userContextKey string

const UserContextKey userContextKey = "UserID"

// LoggerMiddleware injects the logger into the context
func LoggerMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always ensure we have a valid logger
			contextLogger := logger
			if contextLogger == nil {
				contextLogger = log.New(os.Stdout, "[CarBN] ", log.LstdFlags)
			}
			ctx := context.WithValue(r.Context(), common.LoggerCtxKey, contextLogger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Chain(handler http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}

// Simplify the imageFileHandler since we're now using consistent paths
func imageFileHandler(fs http.FileSystem) http.HandlerFunc {
	fileServer := http.FileServer(fs)
	return func(w http.ResponseWriter, r *http.Request) {
		// Save original path and strip /images/ prefix
		path := strings.TrimPrefix(r.URL.Path, "/images/")

		// URL decode the path to handle spaces and special characters
		decodedPath, err := url.QueryUnescape(path)
		if err != nil {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Update the request path without the /images/ prefix
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = decodedPath
		fileServer.ServeHTTP(w, r2)
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Initialize logger
	logger := log.New(os.Stdout, "[CarBN] ", log.LstdFlags)

	// Create root context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add logger to context
	ctx = context.WithValue(ctx, common.LoggerCtxKey, logger)

	// Initialize database
	if err := postgres.InitDB(ctx); err != nil {
		logger.Fatalf("Database initialization failed: %v", err)
	}
	defer postgres.CloseDB(ctx)
	logger.Println("Database connection established")

	// Initialize services
	loginSvc := login.NewService(postgres.DB, login.Config{
		JWTSecret:          []byte(os.Getenv("JWT_SECRET")),
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		AppleTeamID:        os.Getenv("APPLE_TEAM_ID"),
		AppleServiceID:     os.Getenv("APPLE_SERVICE_ID"),
		AppleKeyID:         os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKey:    []byte(os.Getenv("APPLE_PRIVATE_KEY")),
	})
	userSvc := user.NewService(postgres.DB, os.Getenv("GENERATED_SAVE_DIR"))
	subscriptionSvc := subscription.NewSubscriptionService(postgres.DB) // Add subscription service
	feedSvc := feed.NewService(postgres.DB)
	friendsSvc := friends.NewService(postgres.DB, feedSvc)
	tradeSvc := trade.NewService(postgres.DB, feedSvc, subscriptionSvc) // Add subscription service to trade
	likesSvc := likes.NewService(postgres.DB)
	scanSvc := scan.NewService(postgres.DB, feedSvc, subscriptionSvc) // Add subscription service to scan

	// Initialize handlers
	loginHandler := login.NewHTTPHandler(loginSvc)
	userHandler := user.NewHTTPHandler(userSvc)
	subscriptionHandler := subscription.NewSubscriptionHandler(subscriptionSvc) // Add subscription handler
	friendsHandler := friends.NewHTTPHandler(friendsSvc)
	feedHandler := feed.NewHTTPHandler(feedSvc)
	tradeHandler := trade.NewHTTPHandler(tradeSvc)
	scanHandler := scan.NewHTTPHandler(scanSvc)
	likesHandler := likes.NewHandler(likesSvc)

	// Setup router
	mux := http.NewServeMux()

	// Only serve images in development mode
	if os.Getenv("CARBN_DEV") == "development" {
		// Serve image files from root images directory
		imageDir := http.Dir("_resources/images")
		mux.HandleFunc("/images/", imageFileHandler(imageDir))
	}

	// Auth routes
	mux.HandleFunc("POST /auth/google", loginHandler.HandleGoogleSignIn)
	mux.HandleFunc("POST /auth/apple", loginHandler.HandleAppleSignIn)
	mux.HandleFunc("POST /auth/apple/updates", loginHandler.HandleAppleEmailUpdateNotification)
	mux.HandleFunc("POST /auth/refresh", loginHandler.HandleRefresh)
	mux.HandleFunc("POST /auth/logout", loginHandler.HandleLogout)

	// Protected routes
	mux.HandleFunc("GET /user/{user_id}/cars", loginSvc.AuthMiddleware(userHandler.HandleGetCarCollection))
	mux.HandleFunc("GET /user/cars", loginSvc.AuthMiddleware(userHandler.HandleGetSpecificUserCars))
	mux.HandleFunc("GET /user/friend-requests", loginSvc.AuthMiddleware(userHandler.HandleGetPendingFriendRequests))
	mux.HandleFunc("POST /user/profile/picture", loginSvc.AuthMiddleware(userHandler.HandleUploadProfilePicture))
	mux.HandleFunc("POST /user/profile/display-name", loginSvc.AuthMiddleware(userHandler.HandleUpdateDisplayName))
	mux.HandleFunc("GET /user/{user_id}/details", loginSvc.AuthMiddleware(userHandler.HandleGetUserProfile))
	mux.HandleFunc("GET /user/search", loginSvc.AuthMiddleware(userHandler.HandleSearchUsers))
	mux.HandleFunc("GET /user/{user_id}/friends", loginSvc.AuthMiddleware(friendsHandler.HandleGetFriends))
	mux.HandleFunc("GET /user/{user_id}/is-friend", loginSvc.AuthMiddleware(friendsHandler.HandleCheckFriendship))
	mux.HandleFunc("POST /user/cars/{user_car_id}/sell", loginSvc.AuthMiddleware(userHandler.HandleSellCar))

	// Car sharing endpoints
	mux.HandleFunc("POST /user/cars/{user_car_id}/share", loginSvc.AuthMiddleware(userHandler.HandleCreateShareLink))

	// Reorder and fix route patterns:
	// 1. First handle /share/static/ routes with a more specific pattern to avoid conflicts
	mux.HandleFunc("GET /share/static/{filename}", func(w http.ResponseWriter, r *http.Request) {
		// Extract filename from path
		filename := r.PathValue("filename")
		if filename == "" {
			http.NotFound(w, r)
			return
		}

		// Set proper MIME types
		switch {
		case strings.HasSuffix(filename, ".css"):
			w.Header().Set("Content-Type", "text/css")
		case strings.HasSuffix(filename, ".js"):
			w.Header().Set("Content-Type", "application/javascript")
		}

		// Create request with path pointing to the file
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = "/" + filename

		logger.Printf("Serving static file: %s", filename)
		http.FileServer(http.Dir("./shared_car")).ServeHTTP(w, r2)
	})

	// 2. Then register the remaining share routes with updated paths to avoid conflicts
	mux.HandleFunc("GET /share/token/{share_token}/data", userHandler.HandleGetSharedCar) // No auth required
	mux.HandleFunc("GET /share/token/{share_token}", userHandler.HandleServeSharePage)    // No auth required

	mux.HandleFunc("GET /user/cars/{user_car_id}/upgrades", loginSvc.AuthMiddleware(userHandler.HandleGetCarUpgrades))
	mux.HandleFunc("POST /user/cars/{user_car_id}/upgrade-image", loginSvc.AuthMiddleware(userHandler.HandleUpgradeCarImage))
	mux.HandleFunc("POST /user/cars/{user_car_id}/revert-image", loginSvc.AuthMiddleware(userHandler.HandleRevertCarImage))

	mux.HandleFunc("POST /friends/request", loginSvc.AuthMiddleware(friendsHandler.HandleSendFriendRequest))
	mux.HandleFunc("POST /friends/respond", loginSvc.AuthMiddleware(friendsHandler.HandleFriendRequestResponse))

	mux.HandleFunc("GET /feed", loginSvc.AuthMiddleware(feedHandler.HandleGetFeed))
	mux.HandleFunc("GET /feed/{feed_item_id}", loginSvc.AuthMiddleware(feedHandler.HandleGetFeedItem))

	mux.HandleFunc("POST /trade/request", loginSvc.AuthMiddleware(tradeHandler.HandleCreateTrade))
	mux.HandleFunc("POST /trade/respond", loginSvc.AuthMiddleware(tradeHandler.HandleTradeRequestResponse))
	mux.HandleFunc("GET /trade/history", loginSvc.AuthMiddleware(tradeHandler.HandleGetUserTrades))
	mux.HandleFunc("GET /trade/{trade_id}", loginSvc.AuthMiddleware(tradeHandler.HandleGetTrade))

	mux.HandleFunc("POST /scan", loginSvc.AuthMiddleware(scanHandler.HandleScanPost))

	// Likes routes
	mux.HandleFunc("POST /likes/{feedItemId}", loginSvc.AuthMiddleware(likesHandler.CreateFeedItemLike))
	mux.HandleFunc("DELETE /likes/{feedItemId}", loginSvc.AuthMiddleware(likesHandler.DeleteFeedItemLike))
	mux.HandleFunc("GET /likes/feed-item/{feedItemId}", loginSvc.AuthMiddleware(likesHandler.GetFeedItemLikes))
	mux.HandleFunc("GET /likes/feed-item/{feedItemId}/check", loginSvc.AuthMiddleware(likesHandler.CheckFeedItemLike))
	mux.HandleFunc("GET /likes/user/{userId}", loginSvc.AuthMiddleware(likesHandler.GetUserReceivedLikes))

	// Car likes routes
	mux.HandleFunc("POST /likes/car/{userCarId}", loginSvc.AuthMiddleware(likesHandler.CreateUserCarLike))
	mux.HandleFunc("DELETE /likes/car/{userCarId}", loginSvc.AuthMiddleware(likesHandler.DeleteUserCarLike))
	mux.HandleFunc("GET /likes/car/{userCarId}", loginSvc.AuthMiddleware(likesHandler.GetUserCarLikes))
	mux.HandleFunc("GET /likes/car/{userCarId}/count", loginSvc.AuthMiddleware(likesHandler.GetUserCarLikesCount))
	mux.HandleFunc("GET /likes/car/{userCarId}/check", loginSvc.AuthMiddleware(likesHandler.CheckUserCarLike))

	// Account management
	mux.HandleFunc("DELETE /user/account", loginSvc.AuthMiddleware(userHandler.HandleDeleteAccount))

	// Add subscription routes
	mux.HandleFunc("GET /user/subscription", loginSvc.AuthMiddleware(subscriptionHandler.HandleGetUserSubscription))
	mux.HandleFunc("GET /user/{user_id}/subscription/status", loginSvc.AuthMiddleware(subscriptionHandler.HandleGetUserSubscriptionStatus))
	mux.HandleFunc("POST /subscription/purchase", loginSvc.AuthMiddleware(subscriptionHandler.HandlePurchaseSubscription))
	mux.HandleFunc("POST /scanpack/purchase", loginSvc.AuthMiddleware(subscriptionHandler.HandlePurchaseScanPack))
	mux.HandleFunc("GET /subscription/products", loginSvc.AuthMiddleware(subscriptionHandler.HandleGetSubscriptionProducts))

	// App Store Server Notifications V2 webhook - not protected by auth middleware
	mux.HandleFunc("POST /webhook/apple/subscription", subscriptionHandler.HandleAppStoreNotification)

	// Initialize server
	server := &http.Server{
		Addr: ":8080",
		Handler: LoggerMiddleware(logger)(
			common.RequestIDMiddleware(
				common.RecoveryMiddleware(
					common.TimeoutMiddleware(240 * time.Second)(mux),
				),
			),
		),
		ReadTimeout:  240 * time.Second,
		WriteTimeout: 240 * time.Second,
		IdleTimeout:  240 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Printf("Server starting on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	// Shutdown gracefully
	logger.Println("Initiating server shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("Error during server shutdown: %v", err)
	}

	logger.Println("Server shutdown complete")
}

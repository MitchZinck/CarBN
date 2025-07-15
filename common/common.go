package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// Context keys
type contextKey string

// AI prompt constants
const (
	UserIDCtxKey       contextKey = "user_id"
	RequestIDCtxKey    contextKey = "request_id"
	LoggerCtxKey       contextKey = "logger"
	RejectedScanError  string     = "your scan was rejected"
	LowResImageSize    int        = 512
	UpgradeCost        int        = 5000
	HighResFileName    string     = "high_res.jpg"
	LowResFileName     string     = "low_res.jpg"
	DuplicateScanError string     = "cannot scan the same car more than once per day"

	SCAN_IMAGE = `You will be scanning an image to identify a car and return a JSON response with the **closest exact** make, model, trim, year, and color of the car. The image must be a real-life photograph of a car taken directly by the user, as it is being received from an app where users "collect" cars by photographing them in the real world.

#### **Cheating Detection:**
To prevent cheating, you **must** check for any signs that the image is a digital reproduction, including but not limited to:
- **Moire patterns or pixelation** (indicating it was taken from a screen).
- **Glare, reflections, or distortion** typical of photos taken of monitors or printed images.
- **UI elements, watermarks, or framing artifacts** that suggest the image is from another digital source.
- **Unnatural sharpness, blocky compression artifacts, or color banding** that indicate a low-quality screen capture.

If **any** of these conditions suggest that the image is not an original real-life photograph, **immediately reject it** by setting all fields to blank and '"reject": true.

#### **Strict Output Requirement:** ####
- Respond ONLY with a valid JSON object. No extra text, explanations, or formatting.
- Do not include any preamble, postscript, or additional commentary.

#### **JSON Response Format:**
{
  "make": "<string as Identified manufacturer>",
  "model": "<string as Verified model>",
  "trim": "<string as Specific variant>",
  "year": "<string as estimated Year (range) of manufacture, YYYY-YYYY format>",
  "color": "<string as Identified color>",
  "reject": false/true
}

If it is **suspected to be fake**, return:
{
  "make": "",
  "model": "",
  "trim": "",
  "year": "",
  "color": "",
  "reject": true
}

This ensures that only real-world photographs of cars are accepted. Be strict in rejecting any potential fakes.`

	IDENTIFY_CAR_DETAILS = `You are a car expert. Based on the %s %s %s %s, provide detailed specifications in this exact JSON format:
	{
		"horsepower": "<number as string>",
		"torque": "<number as string>",
		"top_speed": "<number as string>",
		"acceleration": "<number as string>",
		"engine_type": "<string: one of [V8, V6, I4, Electric, Hybrid]>",
		"drivetrain_type": "<string: one of [FWD, RWD, AWD, 4WD]>",
		"curb_weight": "<number as string>",
		"price": "<number as string, private sale value in USD>",
		"description": "<string: 2-3 sentence description>",
	}
	
	Ensure all numeric values are provided as strings. Do not include units in the numbers.`

	CREATE_CAR_IMAGE_PROMPT  = `Capture a hyperrealistic, intensely detailed image of a {year} {make} {model} {trim} presented as a prized collectible in a high-stakes environment. The car is painted {color}. The car is captured as if it we're on a shiny, new showroom floor. Wheels are slightly turned, displaying the rims. The LED headlights blaze intensely, casting a soft ambient glow that subtly illuminates the surroundings and creates dynamic lens flares. Subtle rim lighting around the car's silhouette to make it stand out. The floor is a highly polished, light gray, reflective surface that clearly mirrors the car's undercarriage and vibrant {color}. The background is a blurred, out-of-focus car showroom, with hints of other high-end vehicles and soft, ambient lighting. The overall color palette of the background and showroom is a rich, dramatic silver, such that it allows the {color} car to be the primary focal point. Use a low camera angle, looking slightly upwards at the car.`
	PREMIUM_CAR_IMAGE_PROMPT = `Capture an ultra-premium, hyperrealistic, and intensely detailed action shot of a **limited-edition {year} {make} {model} {trim}**, dynamically speeding through an **exclusive, high-stakes environment**. The car is painted {color}, and its **custom, limited-edition rims** spin dynamically, kicking up a fine mist of water, dust, or subtle sparks, depending on the terrain.

The **LED headlights blaze intensely**, casting **volumetric god rays** and **artistic lens flares**. **Streetlights, neon reflections, or sunset backlighting dance across the glossy body**, ensuring that each image captures a **unique cinematic look**.

The setting is **{selected_background}**, adding an element of prestige and luxury while ensuring uniqueness in every image. The **environment subtly reflects off the car's glossy finish**, reinforcing its collector's edition status.

Despite the variation in background, the **car remains the absolute focal point**, taking up most of the frame in a **dominant, commanding stance**. The **camera angle is always low and slightly tilted**, tracking the car's motion with precision.

**Motion blur in the background**, reflections on the pavement, and amplified rim lighting ensure a **dynamic, unique, yet consistent premium experience** in every shot.
`
)

// Standard format for all timestamps in the API
const TimestampFormat = time.RFC3339

var PREMIUM_BACKGROUNDS = [40]string{
	"Neon-lit futuristic city street with rain-slicked reflections",
	"Exclusive penthouse garage overlooking a dazzling city skyline",
	"Luxury hotel entrance driveway with golden lighting",
	"High-tech tunnel with streaking light trails",
	"Private high-rise rooftop parking deck with a panoramic cityscape",
	"Mountain pass at sunset with golden-hour lighting",
	"Coastal highway with ocean views and dramatic cliffs",
	"Secluded forest road with mist and soft golden light",
	"Desert highway at twilight with heat distortion",
	"Snow-covered mountain road with a crisp blue sky",
	"Cyberpunk-style cityscape with holographic billboards",
	"Luminous underground racing tunnel with neon accents",
	"Sleek floating highway over a futuristic city",
	"Space station showroom with Earth visible outside",
	"Private airstrip at sunset with jet-fueled luxury vibes",
	"Formula 1 racetrack at night under intense floodlights",
	"Exclusive automotive showcase event with VIP branding",
	"Secret billionaire's underground car vault with marble floors",
	"Opulent casino entrance with grand lighting",
	"Italian countryside road near a vineyard",
	// 20 new luxurious backgrounds:
	"Glass-walled luxury showroom with a glowing infinity pool backdrop",
	"Private island pier at dusk with yacht silhouettes",
	"Golden desert oasis with palm trees and mirrored water",
	"Baroque-style palace courtyard with marble fountains",
	"High-altitude sky lounge with clouds below and starry skies above",
	"Velvet-lined luxury car museum with crystal chandeliers",
	"Sleek metropolitan bridge at night with shimmering river reflections",
	"Glistening arctic ice cave with aurora borealis overhead",
	"Monaco harbor at twilight with superyachts and soft pastel skies",
	"Rooftop helipad with a neon-lit metropolis sprawling below",
	"Polished obsidian garage with ambient purple accent lighting",
	"Tropical rainforest retreat with a waterfall cascading nearby",
	"Historic European castle driveway lined with torchlit statues",
	"Futuristic orbital platform with a view of distant galaxies",
	"Midnight urban plaza with glowing sculptures and mist",
	"Exotic safari lodge with savanna sunset hues",
	"Glass-domed atrium with rare orchids and soft skylight",
	"Private racetrack in the Alps with snow-dusted peaks",
	"Underwater luxury showroom with bioluminescent coral accents",
	"Regal opera house parking court with golden arches and velvet ropes",
}

// SaveImage saves a base64 encoded image to a file with proper path handling and security checks
func SaveImage(base64Image string, baseDir string, relativePath string, fileMode os.FileMode) error {
	if baseDir == "" {
		return fmt.Errorf("base directory is not configured")
	}

	// Remove data URL prefix if present
	if idx := strings.Index(base64Image, ","); idx != -1 {
		base64Image = base64Image[idx+1:]
	}

	imageData, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		return fmt.Errorf("failed to decode base64 image: %v", err)
	}

	// Ensure the path is clean and absolute
	fullPath := filepath.Clean(filepath.Join(baseDir, relativePath))
	if !strings.HasPrefix(fullPath, baseDir) {
		return fmt.Errorf("invalid path: attempted path traversal")
	}

	// Create all necessary parent directories
	dirPath := filepath.Dir(fullPath)
	if err := os.MkdirAll(dirPath, fileMode); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dirPath, err)
	}

	if err := os.WriteFile(fullPath, imageData, fileMode); err != nil {
		return fmt.Errorf("failed to save image to %s: %v", fullPath, err)
	}

	return nil
}

// SaveLowResVersion creates and saves a low resolution version of a base64 encoded image
func SaveLowResVersion(base64Image string, savePath string, quality int, fileMode os.FileMode) error {
	// Remove data URL prefix if present
	if idx := strings.Index(base64Image, ","); idx != -1 {
		base64Image = base64Image[idx+1:]
	}

	imageData, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		return fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Create reader from image data
	reader := bytes.NewReader(imageData)

	// Decode the image
	img, _, err := image.Decode(reader)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Get original bounds
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	// Calculate new dimensions maintaining aspect ratio
	var newWidth, newHeight int
	if originalWidth > originalHeight {
		newWidth = LowResImageSize
		newHeight = int(float64(originalHeight) * (float64(LowResImageSize) / float64(originalWidth)))
	} else {
		newHeight = LowResImageSize
		newWidth = int(float64(originalWidth) * (float64(LowResImageSize) / float64(originalHeight)))
	}

	// Create a new RGBA image
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Use high-quality image scaling
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Over, nil)

	// Create all necessary parent directories
	if err := os.MkdirAll(filepath.Dir(savePath), fileMode); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create the output file
	out, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	// Encode and save as JPEG with specified quality
	if err := jpeg.Encode(out, resized, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("failed to encode JPEG: %w", err)
	}

	return nil
}

// SaveGeneratedImage saves a base64 encoded image to a file
func SaveGeneratedImage(base64Image string, filePath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Remove data URL prefix if present
	if idx := strings.Index(base64Image, ","); idx != -1 {
		base64Image = base64Image[idx+1:]
	}

	// Decode base64 image
	imageData, err := base64.StdEncoding.DecodeString(base64Image)
	if err != nil {
		return fmt.Errorf("failed to decode base64 image: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, imageData, 0755); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	return nil
}

// CreateLowResVersion creates a low resolution version of the image
func CreateLowResVersion(highResPath string, lowResPath string) error {
	// Read the original image
	file, err := os.Open(highResPath)
	if err != nil {
		return fmt.Errorf("failed to open high res image: %w", err)
	}
	defer file.Close()

	// Decode the image
	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode high res image: %w", err)
	}

	// Create a low res version (512x512)
	lowRes := image.NewRGBA(image.Rect(0, 0, 512, 512))
	draw.CatmullRom.Scale(lowRes, lowRes.Bounds(), img, img.Bounds(), draw.Over, nil)

	// Create directory if it doesn't exist
	dir := filepath.Dir(lowResPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Save the low res version
	outFile, err := os.Create(lowResPath)
	if err != nil {
		return fmt.Errorf("failed to create low res file: %w", err)
	}
	defer outFile.Close()

	// Encode as JPEG with 80% quality
	if err := jpeg.Encode(outFile, lowRes, &jpeg.Options{Quality: 95}); err != nil {
		return fmt.Errorf("failed to encode low res image: %w", err)
	}

	return nil
}

// FormatTimestamp converts a time.Time to our standard string format
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimestampFormat)
}

// ParseTimestamp parses a timestamp string in our standard format
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(TimestampFormat, s)
}

// Comment for frontend developers:
// All timestamps in the API are returned in RFC3339 format (e.g. "2023-12-25T12:00:00Z")
// This is a standardized format that includes:
// - Full date (YYYY-MM-DD)
// - Time (HH:MM:SS)
// - Timezone offset (Z for UTC)
// - All timestamps are in UTC timezone for consistency
// You can parse these timestamps using:
// - JavaScript: new Date(timestamp)
// - moment.js: moment(timestamp)
// - date-fns: parseISO(timestamp)

// Middleware to set request timeout
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// Middleware to add request ID to context
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%d", time.Now().UnixNano())
		}

		ctx := context.WithValue(r.Context(), RequestIDCtxKey, requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Middleware to recover from panics
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID, _ := r.Context().Value(RequestIDCtxKey).(string)
				logger, ok := r.Context().Value(LoggerCtxKey).(*log.Logger)
				if !ok || logger == nil {
					logger = log.New(os.Stdout, "[CarBN] ", log.LstdFlags)
				}
				logger.Printf("Panic occurred in request %s: %v\n", requestID, err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type Cursor struct {
	Timestamp time.Time
	ID        int
}

func EncodeCursor(timestamp time.Time, id int) string {
	data := fmt.Sprintf("%s,%d", timestamp.Format(time.RFC3339), id)
	return base64.StdEncoding.EncodeToString([]byte(data))
}

func DecodeCursor(encoded string) (*Cursor, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(string(data), ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor format")
	}

	timestamp, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return nil, err
	}

	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}

	return &Cursor{
		Timestamp: timestamp,
		ID:        id,
	}, nil
}

type ImageResponse struct {
	Images []string `json:"images"`
	Error  string   `json:"error"`
}

// Generate the image
const imagen_model = "imagen-3.0-generate-002"

// GeminiClient provides a thread-safe client for Gemini API access
type GeminiClient struct {
	client  *genai.Client
	mutex   sync.RWMutex
	limiter *rate.Limiter
}

var (
	geminiSingleton *GeminiClient
	once            sync.Once
)

func GenerateCarImage(ctx context.Context, year string, make, model, trim, color string, isPremium bool, background string) (string, error) {
	client, err := GetGeminiClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get Gemini client: %w", err)
	}

	return client.GenerateCarImage(ctx, year, make, model, trim, color, isPremium, background)
}

// GetGeminiClient returns a singleton instance of GeminiClient
func GetGeminiClient(ctx context.Context) (*GeminiClient, error) {
	once.Do(func() {
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			log.Println("GEMINI_API_KEY not set in environment")
			return
		}

		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			log.Printf("Error initializing Gemini client: %v", err)
			return
		}

		geminiSingleton = &GeminiClient{
			client: client,
			// Allow 10 requests per minute (adjust as needed)
			limiter: rate.NewLimiter(rate.Limit(10.0/60.0), 2),
		}
	})

	if geminiSingleton == nil || geminiSingleton.client == nil {
		return nil, fmt.Errorf("failed to initialize Gemini client")
	}

	return geminiSingleton, nil
}

// GenerateCarImage generates a car image using Gemini with retry logic
func (gc *GeminiClient) GenerateCarImage(ctx context.Context, year string, make, model, trim, color string, isPremium bool, background string) (string, error) {
	// Wait for rate limiter
	if err := gc.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Format prompt (same as before)
	var prompt string
	if isPremium {
		prompt = strings.NewReplacer(
			"{year}", year,
			"{make}", make,
			"{model}", model,
			"{trim}", trim,
			"{color}", color,
			"{selected_background}", background,
		).Replace(PREMIUM_CAR_IMAGE_PROMPT)
	} else {
		prompt = strings.NewReplacer(
			"{year}", year,
			"{make}", make,
			"{model}", model,
			"{trim}", trim,
			"{color}", color,
		).Replace(CREATE_CAR_IMAGE_PROMPT)
	}

	// Retry logic
	var result *genai.GenerateImagesResponse
	var err error

	// Acquire read lock for thread safety
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	// Retry up to 3 times with exponential backoff
	backoff := 1 * time.Second
	for attempts := 0; attempts < 3; attempts++ {
		config := &genai.GenerateImagesConfig{
			NumberOfImages: genai.Ptr[int64](1),
			OutputMIMEType: "image/png",
			AspectRatio:    "1:1",
			EnhancePrompt:  false,
		}

		result, err = gc.client.Models.GenerateImages(ctx, imagen_model, prompt, config)
		if err == nil && len(result.GeneratedImages) > 0 {
			break
		}

		// If context deadline exceeded or canceled, don't retry
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", err
		}

		log.Printf("Image generation attempt %d failed: %v, retrying in %v",
			attempts+1, err, backoff)

		// Wait before retry
		select {
		case <-time.After(backoff):
			backoff *= 2 // Exponential backoff
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	// Handle final error case
	if err != nil {
		return "", fmt.Errorf("image generation failed after retries: %w", err)
	}

	if len(result.GeneratedImages) == 0 {
		return "", fmt.Errorf("no images generated")
	}

	// Convert to base64
	imgBytes := result.GeneratedImages[0].Image.ImageBytes
	imgBase64 := base64.StdEncoding.EncodeToString(imgBytes)

	return imgBase64, nil
}

// GetCurrencyForRarity returns the amount of currency to award based on rarity
func GetCurrencyForRarity(rarity int) int {
	switch rarity {
	case 1:
		return 500
	case 2:
		return 750
	case 3:
		return 1250
	case 4:
		return 2000
	case 5:
		return 5000
	default:
		return 0
	}
}

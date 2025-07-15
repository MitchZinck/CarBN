package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

func main_test1() {
	// Load environment variables from .env file.
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Retrieve the API key from the environment.
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY not set in environment")
	}

	// Initialize the Gen AI client with the API key.
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})

	var config *genai.GenerateImagesConfig = &genai.GenerateImagesConfig{
		NumberOfImages: genai.Ptr[int64](1),
		OutputMIMEType: "image/png",
		AspectRatio:    "1:1",
		EnhancePrompt:  false,
	}
	// Call the GenerateImages method.
	model := "imagen-3.0-generate-002"
	result, err := client.Models.GenerateImages(ctx, model, "Create a blue circle", config)
	if err != nil {
		log.Fatal(err)
	}
	// Marshal the result to JSON and pretty-print it to a byte array.
	response, err := json.MarshalIndent(*result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	// Log the output.
	fmt.Println(string(response))

	// Process and display the response.
	fmt.Println("Image generated successfully!")

	// Save the image to ~/Documents/ directory
	if len(result.GeneratedImages) > 0 {
		// Get the image data from the first generated image
		imgData := result.GeneratedImages[0].Image.ImageBytes

		// Create the file path in the user's Documents directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Error getting user home directory: %v", err)
		}

		// Create a filename with timestamp to avoid overwriting
		timestamp := time.Now().Format("20060102_150405")
		filePath := filepath.Join(homeDir, "Documents", fmt.Sprintf("blue_circle_%s.png", timestamp))

		// Write the image bytes to the file
		err = os.WriteFile(filePath, imgData, 0644)
		if err != nil {
			log.Fatalf("Error writing image file: %v", err)
		}

		fmt.Printf("Image saved to: %s\n", filePath)
	} else {
		fmt.Println("No images were generated")
	}
}

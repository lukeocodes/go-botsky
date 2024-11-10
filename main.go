package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// AuthResponse represents the authentication response from Bluesky
type AuthResponse struct {
	AccessJwt string `json:"accessJwt"`
	Did       string `json:"did"`
}

// ErrorResponse represents the error response structure from Bluesky
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

const (
	bskyAuthUrl   = "https://bsky.social/xrpc/com.atproto.server.createSession"
	bskyPostUrl   = "https://bsky.social/xrpc/com.atproto.repo.createRecord"
	openaiChatUrl = "https://api.openai.com/v1/chat/completions"
)

var (
	now = time.Now()
)

func main() {
	// // Only load .env file in development environment
	if os.Getenv("ENVIRONMENT") != "production" {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	exitDate := time.Date(2029, time.January, 20, 0, 0, 0, 0, time.UTC)

	if now.After(exitDate) {
		return
	}

	// Get the post we will send
	post := getPost()
	fmt.Printf("Generated post: %s", post)

	// Load environment variables
	username := os.Getenv("BLUESKY_USERNAME")
	if username == "" {
		log.Fatal("BLUESKY_USERNAME environment variable not set")
	}

	password := os.Getenv("BLUESKY_PASSWORD")
	if password == "" {
		log.Fatal("BLUESKY_PASSWORD environment variable not set")
	}

	// Authenticate and obtain access token
	authResponse, err := authenticate(username, password)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	// Post message using access token
	err = postMessage(authResponse.AccessJwt, authResponse.Did, post)
	if err != nil {
		log.Fatalf("Failed to post message: %v", err)
	}

	fmt.Println("Message posted successfully!")
}

func authenticate(identifier string, password string) (*AuthResponse, error) {
	authBody := map[string]string{
		"identifier": identifier,
		"password":   password,
	}
	bodyBytes, err := json.Marshal(authBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth request body: %w", err)
	}

	req, err := http.NewRequest("POST", bskyAuthUrl, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var authResponse AuthResponse
		if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
			return nil, fmt.Errorf("failed to decode auth response: %w", err)
		}

		fmt.Println("Authentication successful!")
		return &authResponse, nil
	}

	var errResponse ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResponse); err != nil {
		return nil, fmt.Errorf("failed to decode error response: %w", err)
	}
	return nil, fmt.Errorf("auth error (%d): %s - %s", resp.StatusCode, errResponse.Error, errResponse.Message)
}

func postMessage(accessToken, did, message string) error {
	postBody := map[string]interface{}{
		"repo":       did,
		"collection": "app.bsky.feed.post",
		"record": map[string]interface{}{
			"$type":     "app.bsky.feed.post",
			"text":      message,
			"createdAt": time.Now().UTC().Format(time.RFC3339),
		},
	}
	bodyBytes, err := json.Marshal(postBody)
	if err != nil {
		return fmt.Errorf("failed to marshal post request body: %w", err)
	}

	req, err := http.NewRequest("POST", bskyPostUrl, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create post request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("Post successful!")
		return nil
	}

	var errResponse ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResponse); err != nil {
		return fmt.Errorf("failed to decode error response: %w", err)
	}
	return fmt.Errorf("post error (%d): %s - %s", resp.StatusCode, errResponse.Error, errResponse.Message)
}

func makeOpenAIRequest() (string, error) {
	blueskyUser := os.Getenv("BLUESKY_USERNAME")

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	systemPrompt := os.Getenv("OPENAI_SYSTEM_PROMPT")
	if systemPrompt == "" {
		return "", fmt.Errorf("OPENAI_SYSTEM_PROMPT environment variable not set")
	}

	userPrompt := os.Getenv("OPENAI_USER_PROMPT")
	if userPrompt == "" {
		return "", fmt.Errorf("OPENAI_USER_PROMPT environment variable not set")
	}

	requestBody := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You're a bot on Bluesky social. Your messages should be no more than 300 characters. Only respond as an agent with the content to post to Bluesky Social.",
			},
			{
				"role":    "system",
				"content": "Your Bluesky username is " + blueskyUser,
			},
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":    "user",
				"content": userPrompt,
			},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", openaiChatUrl, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read error response body: %w", err)
		}
		return "", fmt.Errorf("received non-200 response status: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return response.Choices[0].Message.Content, nil
}

func getPost() string {
	response, err := makeOpenAIRequest()
	if err != nil {
		log.Printf("Error getting AI response: %v", err)
	}

	return response
}

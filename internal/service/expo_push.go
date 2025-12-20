

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ExpoPushClient sends push notifications via Expo's Push API.
//
// Expo Push is simpler than FCM for React Native + Expo projects:
// - Works with Expo Go (no standalone build needed for testing)
// - No Apple Developer account needed
// - No APNs/FCM configuration required
//
// How it works:
// 1. React Native app gets an Expo Push Token (looks like "ExponentPushToken[xxx]")
// 2. App sends this token to your backend (we store in device_tokens table)
// 3. When you want to notify a user, you POST to Expo's API with their tokens
// 4. Expo handles delivery to both iOS and Android
type ExpoPushClient struct {
	httpClient *http.Client
}

// ExpoPushMessage is the payload for Expo's Push API.
type ExpoPushMessage struct {
	To       []string               `json:"to"`                 // Expo push tokens
	Title    string                 `json:"title,omitempty"`    // Notification title
	Body     string                 `json:"body"`               // Notification body (required)
	Data     map[string]interface{} `json:"data,omitempty"`     // Custom data payload
	Sound    string                 `json:"sound,omitempty"`    // "default" or custom sound
	Badge    *int                   `json:"badge,omitempty"`    // iOS badge count
	Priority string                 `json:"priority,omitempty"` // "default", "normal", "high"
}

// ExpoPushResponse is the response from Expo's API.
type ExpoPushResponse struct {
	Data []ExpoPushTicket `json:"data"`
}

type ExpoPushTicket struct {
	Status  string `json:"status"` // "ok" or "error"
	ID      string `json:"id"`     // Ticket ID for receipt checking
	Message string `json:"message,omitempty"`
	Details struct {
		Error string `json:"error,omitempty"` // "DeviceNotRegistered", "MessageTooBig", etc.
	} `json:"details,omitempty"`
}

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// NewExpoPushClient creates a new Expo Push client.
// Unlike FCM, Expo Push doesn't require any credentials!
func NewExpoPushClient() *ExpoPushClient {
	return &ExpoPushClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendToTokens sends a push notification to multiple Expo push tokens.
//
// Parameters:
// - tokens: Expo push tokens (e.g., "ExponentPushToken[xxxxxx]")
// - title: Notification title
// - body: Notification body
// - data: Optional custom data for the app to process
func (c *ExpoPushClient) SendToTokens(tokens []string, title, body string, data map[string]interface{}) error {
	if len(tokens) == 0 {
		return nil
	}

	// Filter to only valid Expo push tokens
	validTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if strings.HasPrefix(token, "ExponentPushToken[") || strings.HasPrefix(token, "ExpoPushToken[") {
			validTokens = append(validTokens, token)
		} else {
			log.Printf("[ExpoPush] Skipping invalid token format: %s", token[:min(20, len(token))])
		}
	}

	if len(validTokens) == 0 {
		log.Printf("[ExpoPush] No valid Expo tokens to send to")
		return nil
	}

	// Build the message
	message := ExpoPushMessage{
		To:       validTokens,
		Title:    title,
		Body:     body,
		Sound:    "default",
		Priority: "high",
		Data:     data,
	}

	// Serialize to JSON
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Send to Expo Push API
	req, err := http.NewRequest("POST", expoPushURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expo api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	// Parse response to check for errors
	var pushResp ExpoPushResponse
	if err := json.Unmarshal(respBody, &pushResp); err != nil {
		log.Printf("[ExpoPush] Failed to parse response: %v", err)
		return nil // Don't fail the notification, push was accepted
	}

	// Log results
	successCount := 0
	failCount := 0
	for i, ticket := range pushResp.Data {
		if ticket.Status == "ok" {
			successCount++
		} else {
			failCount++
			log.Printf("[ExpoPush] Token %d failed: %s (error: %s)",
				i, ticket.Message, ticket.Details.Error)
		}
	}

	log.Printf("[ExpoPush] Sent to %d tokens: %d success, %d failed",
		len(validTokens), successCount, failCount)

	return nil
}

// SendToToken sends a push notification to a single token.
func (c *ExpoPushClient) SendToToken(token, title, body string, data map[string]interface{}) error {
	return c.SendToTokens([]string{token}, title, body, data)
}

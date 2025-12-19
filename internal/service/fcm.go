package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// FCMClient wraps the Firebase Cloud Messaging client.
//
// Firebase Cloud Messaging (FCM) is Google's service for sending push notifications
// to mobile devices. When a user's phone is not actively using your app, FCM can
// still deliver notifications by maintaining a persistent connection between
// the user's device and Google's servers.
//
// How it works:
// 1. Mobile app registers with FCM and gets a "device token" (unique ID for that device)
// 2. App sends the token to your backend (we store it in device_tokens table)
// 3. When you want to notify a user, you send a message to FCM with their token(s)
// 4. FCM delivers the notification to the user's device, even if app is closed
//
// The credentials (project ID, client email, private key) come from Firebase Console:
// Project Settings -> Service Accounts -> Generate New Private Key
type FCMClient struct {
	client *messaging.Client
}

// NewFCMClient creates a new FCM client from environment credentials.
//
// Parameters:
// - projectID: Your Firebase project ID (e.g., "iamstagram-f5ccd")
// - clientEmail: Service account email (e.g., "firebase-adminsdk-xxxxx@project.iam.gserviceaccount.com")
// - privateKey: The private key from the service account JSON (PEM format, with \n for newlines)
//
// The private key in .env has literal "\n" strings, so we replace them with actual newlines.
func NewFCMClient(ctx context.Context, projectID, clientEmail, privateKey string) (*FCMClient, error) {
	// Replace literal \n with actual newlines
	// In .env files, newlines are often escaped as \n (two characters)
	// The Firebase SDK expects actual newline characters in the PEM key
	privateKey = strings.ReplaceAll(privateKey, "\\n", "\n")

	// Build the credentials JSON that Firebase SDK expects
	// This is equivalent to the JSON file you download from Firebase Console
	credsJSON := fmt.Sprintf(`{
		"type": "service_account",
		"project_id": %q,
		"private_key": %q,
		"client_email": %q,
		"token_uri": "https://oauth2.googleapis.com/token"
	}`, projectID, privateKey, clientEmail)

	// Initialize Firebase app with the credentials
	opt := option.WithCredentialsJSON([]byte(credsJSON))
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("initialize firebase app: %w", err)
	}

	// Get the messaging client from the Firebase app
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("get messaging client: %w", err)
	}

	log.Printf("[FCM] Initialized for project: %s", projectID)
	return &FCMClient{client: client}, nil
}

// SendToTokens sends a push notification to multiple device tokens.
//
// Parameters:
// - tokens: FCM device tokens (from device_tokens table)
// - title: Notification title (e.g., "New Follower")
// - body: Notification body (e.g., "user123 started following you")
// - data: Optional key-value data for the app to process (e.g., {"type": "follow", "user_id": "123"})
//
// FCM has a limit of 500 tokens per request. If you have more, you'd need to batch them.
// For 10K users scale, this is unlikely to be an issue per-notification.
func (c *FCMClient) SendToTokens(ctx context.Context, tokens []string, title, body string, data map[string]string) error {
	if len(tokens) == 0 {
		return nil
	}

	// Build the notification message
	// "Notification" is the visual notification (title + body shown to user)
	// "Data" is invisible payload that your app can process in the background
	message := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		// Android-specific configuration
		Android: &messaging.AndroidConfig{
			Priority: "high", // Ensures delivery even in battery-saving mode
			Notification: &messaging.AndroidNotification{
				Sound: "default",
			},
		},
		// iOS-specific configuration
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
					Badge: nil, // We'll handle badge count separately if needed
				},
			},
		},
	}

	// Add custom data if provided
	if data != nil {
		message.Data = data
	}

	// Send to all tokens in one API call (FCM handles fan-out)
	response, err := c.client.SendEachForMulticast(ctx, message)
	if err != nil {
		return fmt.Errorf("send multicast: %w", err)
	}

	// Log results
	log.Printf("[FCM] Sent to %d tokens: %d success, %d failure",
		len(tokens), response.SuccessCount, response.FailureCount)

	// Log individual failures for debugging
	// In production, you might want to remove invalid tokens from the database
	for i, resp := range response.Responses {
		if !resp.Success {
			log.Printf("[FCM] Token %d failed: %v", i, resp.Error)
		}
	}

	return nil
}

// SendToToken sends a push notification to a single device token.
// This is a convenience wrapper around SendToTokens.
func (c *FCMClient) SendToToken(ctx context.Context, token, title, body string, data map[string]string) error {
	return c.SendToTokens(ctx, []string{token}, title, body, data)
}

/*
File: cmd/axis/main.go
Description: Entry point for the Axis application. Initializes Google Workspace services
using service account impersonation and starts the web-based terminal server.
*/
package main

import (
	"context"
	"log"
	"os"

	"axis/internal/workspace"

	"github.com/joho/godotenv"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/impersonate"
	keep "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
)

func main() {
	// 1. Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Info: No .env file found, relying on shell environment variables.")
	}

	ctx := context.Background()

	// 2. Validation
	adminEmail := os.Getenv("ADMIN_EMAIL")
	serviceAccountEmail := os.Getenv("SERVICE_ACCOUNT_EMAIL")
	userEmail := os.Getenv("USER_EMAIL")

	if adminEmail == "" || serviceAccountEmail == "" || userEmail == "" {
		log.Fatal("Error: ADMIN_EMAIL, SERVICE_ACCOUNT_EMAIL, and USER_EMAIL must be set.")
	}

	log.Printf("Initializing Services for %s via SA %s...", adminEmail, serviceAccountEmail)

	// 3. Create the Token Source with Admin and Keep scopes
	ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
		TargetPrincipal: serviceAccountEmail,
		Subject:         adminEmail,
		Scopes: []string{
			admin.AdminDirectoryUserScope,
			keep.KeepScope,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create token source: %v", err)
	}

	// 4. Create the Google API Services
	adminSvc, err := admin.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Admin service: %v", err)
	}

	keepSvc, err := keep.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Keep service: %v", err)
	}

	// 5. Initialize internal workspace wrapper
	ws := workspace.NewService(adminSvc, keepSvc)

	// 6. Verification check
	user, err := ws.GetUser(userEmail)
	if err != nil {
		log.Fatalf("Verification failed: %v", err)
	}
	log.Printf("Verification successful: %s (%s)", user.Name, user.Email)

	// 7. Start the Persistent TUI Server
	// This function blocks and handles all incoming TUI requests.
	StartServer(ws)
}

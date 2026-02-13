/*
File: cmd/axis/main.go
Description: Entry point for the Axis application. Initializes Google Workspace services
using service account impersonation and starts the web-based terminal server. Updated
to use read-only scopes matching Domain-Wide Delegation.
*/
package main

import (
	"context"
	"log"
	"os"

	"axis/internal/server"
	"axis/internal/workspace"

	"github.com/joho/godotenv"
	admin "google.golang.org/api/admin/directory/v1"
	docs "google.golang.org/api/docs/v1"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/impersonate"
	keep "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"
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
	// Changed AdminDirectoryUserScope to AdminDirectoryUserReadonlyScope to match DWD permissions
	ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
		TargetPrincipal: serviceAccountEmail,
		Subject:         adminEmail,
		Scopes: []string{
			admin.AdminDirectoryUserReadonlyScope,
			keep.KeepScope,
			docs.DocumentsScope,
			sheets.SpreadsheetsScope,
			drive.DriveReadonlyScope,
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

	docsSvc, err := docs.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Docs service: %v", err)
	}

	sheetsSvc, err := sheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Sheets service: %v", err)
	}

	driveSvc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Drive service: %v", err)
	}

	// 5. Initialize internal workspace wrapper
	ws := workspace.NewService(adminSvc, keepSvc, docsSvc, sheetsSvc, driveSvc)

	// 6. Verification check
	user, err := ws.GetUser(userEmail)
	if err != nil {
		log.Fatalf("Verification failed: %v", err)
	}
	log.Printf("Verification successful: %s (%s)", user.Name, user.Email)

	// 7. Start the Persistent TUI Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := server.NewServer(ws, user)
	if err := srv.Start(port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

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

	// 2. validation
	adminEmail := os.Getenv("ADMIN_EMAIL")
	serviceAccountEmail := os.Getenv("SERVICE_ACCOUNT_EMAIL")
	testEmail := os.Getenv("TEST_USER_EMAIL")

	if adminEmail == "" || serviceAccountEmail == "" || testEmail == "" {
		log.Fatal("Error: ADMIN_EMAIL, SERVICE_ACCOUNT_EMAIL, and TEST_USER_EMAIL must be set.")
	}

	log.Printf("Initializing Services for %s via SA %s...", adminEmail, serviceAccountEmail)

	// 3. Create the Token Source
	// We include both Admin Directory and Keep scopes here.
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

	// 4. Create the Services
	adminSvc, err := admin.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Admin service: %v", err)
	}

	keepSvc, err := keep.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("Failed to create Keep service: %v", err)
	}

	// 5. Initialize internal workspace wrapper with both services
	ws := workspace.NewService(adminSvc, keepSvc)

	// 6. Execute Logic: User Verification
	user, err := ws.GetUser(testEmail)
	if err != nil {
		log.Fatalf("Admin API call failed: %v", err)
	}
	log.Printf("Success! Verified User: %s (%s)", user.Name, user.Email)

	// 7. Execute Logic: Fetch Keep Notes
	log.Println("Fetching Google Keep notes...")
	notes, err := ws.ListNotes()
	if err != nil {
		log.Fatalf("Keep API call failed: %v", err)
	}

	log.Printf("Found %d notes:", len(notes))
	for _, note := range notes {
		log.Printf("- [%s] %s", note.Title, note.Snippet)
	}
}

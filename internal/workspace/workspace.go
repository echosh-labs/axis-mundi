/*
File: internal/workspace/workspace.go
Description: Core Workspace service wrapper. Provides structural definitions and
initialization logic for interfacing with Google Admin and Keep APIs.
*/
package workspace

import (
	"fmt"

	admin "google.golang.org/api/admin/directory/v1"
	docs "google.golang.org/api/docs/v1"
	drive "google.golang.org/api/drive/v3"
	keep "google.golang.org/api/keep/v1"
	sheets "google.golang.org/api/sheets/v4"
)

// Service wraps the Google Workspace APIs
type Service struct {
	adminService  *admin.Service
	keepService   *keep.Service
	docsService   *docs.Service
	sheetsService *sheets.Service
	driveService  *drive.Service
}

// User represents a simplified user structure
type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	ID    string `json:"id"`
}

// RegistryItem defines a unified structure for frontend display.
type RegistryItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// NewService creates a new workspace service wrapper
func NewService(
	adminSvc *admin.Service,
	keepSvc *keep.Service,
	docsSvc *docs.Service,
	sheetsSvc *sheets.Service,
	driveSvc *drive.Service,
) *Service {
	return &Service{
		adminService:  adminSvc,
		keepService:   keepSvc,
		docsService:   docsSvc,
		sheetsService: sheetsSvc,
		driveService:  driveSvc,
	}
}

// GetUser retrieves a user by email
func (s *Service) GetUser(email string) (*User, error) {
	u, err := s.adminService.Users.Get(email).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve user %s: %w", email, err)
	}

	return &User{
		Name:  u.Name.FullName,
		Email: u.PrimaryEmail,
		ID:    u.Id,
	}, nil
}

// ListRegistryItems provides a consolidated list of Keep, Docs, and Sheets.
func (s *Service) ListRegistryItems() ([]RegistryItem, error) {
	var items []RegistryItem

	// 1. Fetch Keep Notes
	notes, err := s.keepService.Notes.List().Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list keep notes: %w", err)
	}
	for _, note := range notes.Notes {
		if !note.Trashed {
			items = append(items, RegistryItem{
				ID:      note.Name,
				Type:    "keep",
				Title:   note.Title,
				Snippet: "Google Keep Note",
			})
		}
	}

	// 2. Fetch Google Docs
	docsList, err := s.driveService.Files.List().Q("mimeType='application/vnd.google-apps.document'").PageSize(50).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list docs: %w", err)
	}
	for _, file := range docsList.Files {
		items = append(items, RegistryItem{
			ID:      file.Id,
			Type:    "doc",
			Title:   file.Name,
			Snippet: "Google Doc",
		})
	}

	// 3. Fetch Google Sheets
	sheetsList, err := s.driveService.Files.List().Q("mimeType='application/vnd.google-apps.spreadsheet'").PageSize(50).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list sheets: %w", err)
	}
	for _, file := range sheetsList.Files {
		items = append(items, RegistryItem{
			ID:      file.Id,
			Type:    "sheet",
			Title:   file.Name,
			Snippet: "Google Sheet",
		})
	}

	return items, nil
}

// GetSheet retrieves a Google Sheet by its ID
func (s *Service) GetSheet(spreadsheetId string) (*sheets.Spreadsheet, error) {
	sheet, err := s.sheetsService.Spreadsheets.Get(spreadsheetId).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve sheet %s: %w", spreadsheetId, err)
	}
	return sheet, nil
}

// DeleteSheet deletes a Google Sheet by its ID
func (s *Service) DeleteSheet(spreadsheetId string) error {
	_, err := s.sheetsService.Spreadsheets.BatchUpdate(spreadsheetId, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				DeleteSheet: &sheets.DeleteSheetRequest{
					SheetId: 0,
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("unable to delete sheet %s: %w", spreadsheetId, err)
	}
	return nil
}

// GetDoc retrieves a Google Doc by its ID
func (s *Service) GetDoc(documentId string) (*docs.Document, error) {
	doc, err := s.docsService.Documents.Get(documentId).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve doc %s: %w", documentId, err)
	}
	return doc, nil
}

// DeleteDoc deletes a Google Doc by its ID
func (s *Service) DeleteDoc(documentId string) error {
	_, err := s.docsService.Documents.BatchUpdate(documentId, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: 1,
					},
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("unable to delete doc %s: %w", documentId, err)
	}
	return nil
}

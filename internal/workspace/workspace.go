// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/workspace/workspace.go
Description: Core Workspace service wrapper. Provides structural definitions and
initialization logic for interfacing with Google Admin and Keep APIs.
*/
package workspace

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	admin "google.golang.org/api/admin/directory/v1"
	calendar "google.golang.org/api/calendar/v3"
	chat "google.golang.org/api/chat/v1"
	docs "google.golang.org/api/docs/v1"
	drive "google.golang.org/api/drive/v3"
	gmail "google.golang.org/api/gmail/v1"
	keep "google.golang.org/api/keep/v1"
	sheets "google.golang.org/api/sheets/v4"
)

// Service wraps the Google Workspace APIs using domain-wide delegated service account credentials.
// Impersonation is centralized here so auditability and policy extensions remain consistent for commercial tier features.
type Service struct {
	adminService  *admin.Service
	keepService   *keep.Service
	docsService   *docs.Service
	sheetsService *sheets.Service
	driveService  *drive.Service
	gmailService  *gmail.Service
	calendarService *calendar.Service
	chatUserSvc   *chat.Service
	chatBotSvc    *chat.Service
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
	Status  string `json:"status,omitempty"`
}

// NewService creates a new workspace service wrapper
func NewService(
	adminSvc *admin.Service,
	keepSvc *keep.Service,
	docsSvc *docs.Service,
	sheetsSvc *sheets.Service,
	driveSvc *drive.Service,
	gmailSvc *gmail.Service,
	calendarSvc *calendar.Service,
	chatUserSvc *chat.Service,
	chatBotSvc *chat.Service,
) *Service {
	return &Service{
		adminService:  adminSvc,
		keepService:   keepSvc,
		docsService:   docsSvc,
		sheetsService: sheetsSvc,
		driveService:  driveSvc,
		gmailService:  gmailSvc,
		calendarService: calendarSvc,
		chatUserSvc:   chatUserSvc,
		chatBotSvc:    chatBotSvc,
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
	docsList, err := s.driveService.Files.List().Q("mimeType='application/vnd.google-apps.document' and trashed=false").PageSize(50).Do()
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
	sheetsList, err := s.driveService.Files.List().Q("mimeType='application/vnd.google-apps.spreadsheet' and trashed=false").PageSize(50).Do()
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

	// 4. Fetch Gmail Threads
	if s.gmailService != nil {
		threadsList, err := s.gmailService.Users.Threads.List("me").Q("in:inbox").MaxResults(50).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to list gmail threads: %w", err)
		}

		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, thread := range threadsList.Threads {
			wg.Add(1)
			go func(th *gmail.Thread) {
				defer wg.Done()

				// Fetch thread metadata for Subject
				fullThread, err := s.gmailService.Users.Threads.Get("me", th.Id).Format("metadata").MetadataHeaders("Subject").Do()
				if err != nil {
					return
				}

				title := "No Subject"
				status := ""

				if len(fullThread.Messages) > 0 {
					msg := fullThread.Messages[0]
					for _, header := range msg.Payload.Headers {
						if header.Name == "Subject" {
							title = header.Value
							break
						}
					}

					var importantLabels []string
					for _, label := range msg.LabelIds {
						if label == "UNREAD" || label == "IMPORTANT" || label == "STARRED" {
							importantLabels = append(importantLabels, label)
						}
					}
					status = strings.Join(importantLabels, ", ")
				}

				mu.Lock()
				items = append(items, RegistryItem{
					ID:      th.Id,
					Type:    "gmail",
					Title:   title,
					Snippet: th.Snippet,
					Status:  status,
				})
				mu.Unlock()
			}(thread)
		}
		wg.Wait()
	}

	// 5. Fetch Calendar Events
	if s.calendarService != nil {
		events, err := s.calendarService.Events.List("primary").
			TimeMin(time.Now().Format(time.RFC3339)).
			MaxResults(50).
			SingleEvents(true).
			OrderBy("startTime").Do()

		if err != nil {
			return nil, fmt.Errorf("failed to list calendar events: %w", err)
		}

		for _, item := range events.Items {
			items = append(items, RegistryItem{
				ID:      item.Id,
				Type:    "calendar",
				Title:   item.Summary,
				Snippet: "Calendar Event",
			})
		}
	}

	return items, nil
}

// GetSheet retrieves a Google Sheet and its values by ID
func (s *Service) GetSheet(spreadsheetId string) (*sheets.Spreadsheet, error) {
	sheet, err := s.sheetsService.Spreadsheets.Get(spreadsheetId).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve sheet %s: %w", spreadsheetId, err)
	}
	return sheet, nil
}

// GetSheetValues pulls the explicit tabular grid data from a range
func (s *Service) GetSheetValues(spreadsheetId string, readRange string) (*sheets.ValueRange, error) {
	resp, err := s.sheetsService.Spreadsheets.Values.Get(spreadsheetId, readRange).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve sheet values %s: %w", spreadsheetId, err)
	}
	return resp, nil
}

// AppendSheetRow pushes an array of values as a new row
func (s *Service) AppendSheetRow(spreadsheetId string, writeRange string, values []interface{}) error {
	valueRange := &sheets.ValueRange{
		Values: [][]interface{}{values},
	}

	_, err := s.sheetsService.Spreadsheets.Values.Append(spreadsheetId, writeRange, valueRange).
		ValueInputOption("USER_ENTERED").
		InsertDataOption("INSERT_ROWS").
		Do()

	if err != nil {
		return fmt.Errorf("failed to append row to %s: %w", spreadsheetId, err)
	}
	return nil
}

// DeleteSheet deletes a Google Sheet by its ID using the Drive API
func (s *Service) DeleteSheet(spreadsheetId string) error {
	err := s.driveService.Files.Delete(spreadsheetId).Do()
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

// ExtractDocContent traverses the rich Google Doc structure and extracts a contiguous plain text string
func ExtractDocContent(content []*docs.StructuralElement) string {
	var text string
	for _, element := range content {
		if element.Paragraph != nil {
			for _, element := range element.Paragraph.Elements {
				if element.TextRun != nil {
					text += element.TextRun.Content
				}
			}
		}
	}
	return text
}

// DeleteDoc deletes a Google Doc by its ID using the Drive API
func (s *Service) DeleteDoc(documentId string) error {
	err := s.driveService.Files.Delete(documentId).Do()
	if err != nil {
		return fmt.Errorf("unable to delete doc %s: %w", documentId, err)
	}
	return nil
}

// GetGmailThread fetches a full thread by ID, including all messages and bodies
func (s *Service) GetGmailThread(threadId string) (*gmail.Thread, error) {
	thread, err := s.gmailService.Users.Threads.Get("me", threadId).Format("full").Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve gmail thread %s: %w", threadId, err)
	}
	return thread, nil
}

// TrashGmailThread moves a thread to the trash
func (s *Service) TrashGmailThread(threadId string) error {
	_, err := s.gmailService.Users.Threads.Trash("me", threadId).Do()
	if err != nil {
		return fmt.Errorf("failed to trash gmail thread %s: %w", threadId, err)
	}
	return nil
}

// ExtractThreadContent distills a complex gmail.Thread into a plain text summary optimized for LLM context
func ExtractThreadContent(thread *gmail.Thread) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Thread ID: %s\n", thread.Id))
	sb.WriteString(fmt.Sprintf("Messages in thread: %d\n", len(thread.Messages)))
	sb.WriteString("--------------------------------------------------\n")

	for i, msg := range thread.Messages {
		sb.WriteString(fmt.Sprintf("--- Message %d ---\n", i+1))

		// Extract headers
		var date, from, to, cc, subject string
		for _, header := range msg.Payload.Headers {
			switch header.Name {
			case "Date":
				date = header.Value
			case "From":
				from = header.Value
			case "To":
				to = header.Value
			case "Cc":
				cc = header.Value
			case "Subject":
				subject = header.Value
			}
		}

		sb.WriteString(fmt.Sprintf("Date: %s\n", date))
		sb.WriteString(fmt.Sprintf("From: %s\n", from))
		if to != "" {
			sb.WriteString(fmt.Sprintf("To: %s\n", to))
		}
		if cc != "" {
			sb.WriteString(fmt.Sprintf("Cc: %s\n", cc))
		}
		if subject != "" {
			sb.WriteString(fmt.Sprintf("Subject: %s\n", subject))
		}

		// Detect attachments
		var attachments []string
		bodyText := extractBodyText(msg.Payload)

		// Walk parts to find filenames
		if msg.Payload.Parts != nil {
			for _, part := range msg.Payload.Parts {
				if part.Filename != "" {
					attachments = append(attachments, part.Filename)
				}
			}
		} else if msg.Payload.Filename != "" {
			attachments = append(attachments, msg.Payload.Filename)
		}

		if len(attachments) > 0 {
			sb.WriteString(fmt.Sprintf("Attachments: %s\n", strings.Join(attachments, ", ")))
		}

		sb.WriteString("\nBody:\n")
		// Strip simple HTML tags if only HTML is present, or just use plain text if found
		if bodyText == "" {
			bodyText = "[No readable text found]"
		}

		// Decode base64 if needed, though gmail API returns base64url encoded. Wait, google api client handles decoding?
		// Actually, `Body.Data` is base64url encoded string in the JSON response, we must decode it.
		sb.WriteString(bodyText)
		sb.WriteString("\n--------------------------------------------------\n")
	}

	return sb.String()
}

func extractBodyText(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}

	// If we found plain text, decode and return it
	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	// Recursively search parts for text/plain
	var htmlFallback string
	for _, subPart := range part.Parts {
		text := extractBodyText(subPart)
		if text != "" && subPart.MimeType == "text/plain" {
			return text
		}
		if subPart.MimeType == "text/html" && subPart.Body != nil && subPart.Body.Data != "" {
			data, err := base64.URLEncoding.DecodeString(subPart.Body.Data)
			if err == nil {
				htmlFallback = stripHTML(string(data))
			}
		}
	}

	// If no plain text was found, but we found HTML and stripped it, return that
	if htmlFallback != "" {
		return htmlFallback
	}

	// If this part itself is HTML and we haven't found anything better
	if part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return stripHTML(string(data))
		}
	}

	return ""
}

// simple HTML stripper to save token context
func stripHTML(htmlStr string) string {
	var text strings.Builder
	inTag := false
	for _, char := range htmlStr {
		if char == '<' {
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag {
			text.WriteRune(char)
		}
	}
	return strings.TrimSpace(text.String())
}

// GetCalendarEvent retrieves a Google Calendar event by its ID
func (s *Service) GetCalendarEvent(eventId string) (*calendar.Event, error) {
	event, err := s.calendarService.Events.Get("primary", eventId).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve calendar event %s: %w", eventId, err)
	}
	return event, nil
}

// DeleteCalendarEvent deletes a Google Calendar event by its ID
func (s *Service) DeleteCalendarEvent(eventId string) error {
	err := s.calendarService.Events.Delete("primary", eventId).Do()
	if err != nil {
		return fmt.Errorf("unable to delete calendar event %s: %w", eventId, err)
	}
	return nil
}

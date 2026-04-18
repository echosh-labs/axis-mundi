// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/workspace/workspace_test.go
Description: Unit tests for the Workspace service. Validates service initialization,
registry item consolidation, and Google API service wrapping logic.
*/
package workspace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"
	calendar "google.golang.org/api/calendar/v3"
	chat "google.golang.org/api/chat/v1"
	docs "google.golang.org/api/docs/v1"
	drive "google.golang.org/api/drive/v3"
	gmail "google.golang.org/api/gmail/v1"
	keep "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"
)

func TestNewService(t *testing.T) {
	adminSvc := &admin.Service{}
	keepSvc := &keep.Service{}
	docsSvc := &docs.Service{}
	sheetsSvc := &sheets.Service{}
	driveSvc := &drive.Service{}
	gmailSvc := &gmail.Service{}
	calendarSvc := &calendar.Service{}
	chatUserSvc := &chat.Service{}
	chatBotSvc := &chat.Service{}

	ws := NewService(adminSvc, keepSvc, docsSvc, sheetsSvc, driveSvc, gmailSvc, calendarSvc, chatUserSvc, chatBotSvc)

	if ws.adminService != adminSvc {
		t.Error("Admin service not correctly assigned")
	}
	if ws.keepService != keepSvc {
		t.Error("Keep service not correctly assigned")
	}
	if ws.docsService != docsSvc {
		t.Error("Docs service not correctly assigned")
	}
	if ws.sheetsService != sheetsSvc {
		t.Error("Sheets service not correctly assigned")
	}
	if ws.driveService != driveSvc {
		t.Error("Drive service not correctly assigned")
	}
	if ws.gmailService != gmailSvc {
		t.Error("Gmail service not correctly assigned")
	}
	if ws.calendarService != calendarSvc {
		t.Error("Calendar service not correctly assigned")
	}
	if ws.chatUserSvc != chatUserSvc {
		t.Error("Chat user service not correctly assigned")
	}
	if ws.chatBotSvc != chatBotSvc {
		t.Error("Chat bot service not correctly assigned")
	}
}

func TestListRegistryItems(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/notes" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"notes": [{"name": "notes/1", "title": "Test Note", "trashed": false}]}`))
			return
		}
		if r.URL.Path == "/calendar/v3/calendars/primary/events" || r.URL.Path == "/calendars/primary/events" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items": [{"id": "event1", "summary": "Test Event"}]}`))
			return
		}
		// Drive API requests (Docs, Sheets) return empty to isolate list keep registry item count.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"files": []}`))
	}))
	defer ts.Close()

	ctx := context.Background()
	keepSvc, err := keep.NewService(ctx, option.WithEndpoint(ts.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}

	driveSvc, err := drive.NewService(ctx, option.WithEndpoint(ts.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}

	calendarSvc, err := calendar.NewService(ctx, option.WithEndpoint(ts.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}

	ws := NewService(nil, keepSvc, nil, nil, driveSvc, nil, calendarSvc, nil, nil)
	items, err := ws.ListRegistryItems()
	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	
	// Check Keep Note presence
	foundNote := false
	for _, item := range items {
		if item.Title == "Test Note" && item.Type == "keep" {
			foundNote = true
		}
	}
	if !foundNote {
		t.Errorf("expected Keep Note 'Test Note' to be present")
	}

	// Check Calendar Event presence
	foundEvent := false
	for _, item := range items {
		if item.Title == "Test Event" && item.Type == "calendar" {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Errorf("expected Calendar Event 'Test Event' to be present")
	}
}

func TestExtractDocContent(t *testing.T) {
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{
							Content: "Hello ",
						},
					},
					{
						TextRun: &docs.TextRun{
							Content: "World\n",
						},
					},
				},
			},
		},
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{}, // Empty element to test nil text run check
					{
						TextRun: &docs.TextRun{
							Content: "Test content",
						},
					},
				},
			},
		},
		{}, // Empty structural element
	}

	result := ExtractDocContent(content)
	expected := "Hello World\nTest content"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

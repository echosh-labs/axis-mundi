// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/workspace/details.go
Description: Canonical domain detail models and formatting methods for Workspace entities
(Docs, Sheets, Gmail threads, Calendar events, and Keep notes). Unifies content
extraction and Markdown presentation across MCP and REST interfaces.
*/
package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	calendar "google.golang.org/api/calendar/v3"
	gmail "google.golang.org/api/gmail/v1"
	keepapi "google.golang.org/api/keep/v1"
)

// DocDetail encapsulates a Google Doc's metadata and plain-text body content.
type DocDetail struct {
	Title      string `json:"title"`
	DocumentID string `json:"documentId"`
	Content    string `json:"content"`
}

// Markdown formats the DocDetail as a Markdown document.
func (d *DocDetail) Markdown() string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(d.Title)
	b.WriteString("\n\n")
	if d.Content != "" {
		b.WriteString(d.Content)
	}
	return b.String()
}

// SheetDetail encapsulates a Google Sheet's metadata and 2D grid cell values.
type SheetDetail struct {
	Title         string          `json:"title"`
	SpreadsheetID string          `json:"spreadsheetId"`
	Values        [][]interface{} `json:"values"`
}

// Markdown formats the SheetDetail grid as a tab-delimited text/markdown document.
func (s *SheetDetail) Markdown() string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(s.Title)
	b.WriteString("\n\n")

	if len(s.Values) == 0 {
		b.WriteString("[empty sheet or no values in range]\n")
		return b.String()
	}

	for _, row := range s.Values {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = fmt.Sprintf("%v", cell)
		}
		b.WriteString(strings.Join(cells, "\t"))
		b.WriteString("\n")
	}
	return b.String()
}

// GmailThreadDetail encapsulates a Gmail thread summary and message payload.
type GmailThreadDetail struct {
	Title    string        `json:"title"`
	ThreadID string        `json:"threadId"`
	Content  string        `json:"content"`
	Raw      *gmail.Thread `json:"raw,omitempty"`
}

// Markdown formats the Gmail thread as plain text / markdown.
func (g *GmailThreadDetail) Markdown() string {
	return g.Content
}

// CalendarEventDetail encapsulates a Google Calendar event's structured fields.
type CalendarEventDetail struct {
	Title       string          `json:"title"`
	EventID     string          `json:"eventId"`
	Summary     string          `json:"summary,omitempty"`
	Start       string          `json:"start,omitempty"`
	End         string          `json:"end,omitempty"`
	Location    string          `json:"location,omitempty"`
	Description string          `json:"description"`
	Raw         *calendar.Event `json:"raw,omitempty"`
}

// Markdown formats the Calendar event into a readable summary.
func (e *CalendarEventDetail) Markdown() string {
	var b strings.Builder
	summary := e.Summary
	if summary == "" {
		summary = e.Title
	}
	b.WriteString(fmt.Sprintf("Event: %s\n", summary))
	if e.Start != "" {
		b.WriteString(fmt.Sprintf("Start: %s\n", e.Start))
	}
	if e.End != "" {
		b.WriteString(fmt.Sprintf("End: %s\n", e.End))
	}
	if e.Location != "" {
		b.WriteString(fmt.Sprintf("Location: %s\n", e.Location))
	}
	b.WriteString("\nDescription:\n")
	if e.Description != "" {
		b.WriteString(e.Description)
	} else {
		b.WriteString("[No description]")
	}
	b.WriteString("\n")
	return b.String()
}

// KeepNoteDetail encapsulates a Google Keep note's structured fields and content.
type KeepNoteDetail struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Title      string        `json:"title"`
	Content    string        `json:"content"`
	CreateTime string        `json:"createTime,omitempty"`
	UpdateTime string        `json:"updateTime,omitempty"`
	Raw        *keepapi.Note `json:"raw,omitempty"`
}

// Markdown formats the Keep note into a Markdown document.
func (n *KeepNoteDetail) Markdown() string {
	var b strings.Builder
	b.WriteString("# ")
	title := n.Title
	if title == "" {
		title = "Untitled"
	}
	b.WriteString(title)
	b.WriteString("\n\n")

	if n.CreateTime != "" {
		if t, err := time.Parse(time.RFC3339, n.CreateTime); err == nil {
			b.WriteString(fmt.Sprintf("Created: %s\n", t.Format("2006-01-02 15:04:05")))
		}
	}
	if n.UpdateTime != "" {
		if t, err := time.Parse(time.RFC3339, n.UpdateTime); err == nil {
			b.WriteString(fmt.Sprintf("Updated: %s\n", t.Format("2006-01-02 15:04:05")))
		}
	}
	b.WriteString("\n")
	if n.Content != "" {
		b.WriteString(n.Content)
	}
	return b.String()
}

// FormatKeepNote formats a Google Keep note into a Markdown document string.
func FormatKeepNote(note *keepapi.Note) string {
	if note == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("# ")
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = "Untitled"
	}
	b.WriteString(title)
	b.WriteString("\n\n")

	if note.CreateTime != "" {
		if t, err := time.Parse(time.RFC3339, note.CreateTime); err == nil {
			b.WriteString(fmt.Sprintf("Created: %s\n", t.Format("2006-01-02 15:04:05")))
		}
	}
	if note.UpdateTime != "" {
		if t, err := time.Parse(time.RFC3339, note.UpdateTime); err == nil {
			b.WriteString(fmt.Sprintf("Updated: %s\n", t.Format("2006-01-02 15:04:05")))
		}
	}
	b.WriteString("\n")

	content := ExtractFullContent(note.Body)
	if content != "" {
		b.WriteString(content)
	}

	return b.String()
}

// GetDocDetail retrieves a Google Doc and encapsulates its metadata and extracted content.
func (s *Service) GetDocDetail(documentID string) (*DocDetail, error) {
	doc, err := s.GetDoc(documentID)
	if err != nil {
		return nil, err
	}

	content := ""
	if doc.Body != nil {
		content = ExtractDocContent(doc.Body.Content)
	}

	return &DocDetail{
		Title:      doc.Title,
		DocumentID: doc.DocumentId,
		Content:    content,
	}, nil
}

// GetSheetDetail retrieves a Google Sheet and its values across the requested range (defaults to A1:Z100).
func (s *Service) GetSheetDetail(spreadsheetID string, readRange string) (*SheetDetail, error) {
	if readRange == "" {
		readRange = "A1:Z100"
	}

	sheet, err := s.GetSheet(spreadsheetID)
	if err != nil {
		return nil, err
	}

	valuesResp, err := s.GetSheetValues(spreadsheetID, readRange)
	var values [][]interface{}
	if err == nil && valuesResp != nil {
		values = valuesResp.Values
	}

	title := ""
	if sheet.Properties != nil {
		title = sheet.Properties.Title
	}

	return &SheetDetail{
		Title:         title,
		SpreadsheetID: sheet.SpreadsheetId,
		Values:        values,
	}, nil
}

// GetGmailThreadDetail fetches a full Gmail thread and distills its messages into structured content.
func (s *Service) GetGmailThreadDetail(threadID string) (*GmailThreadDetail, error) {
	thread, err := s.GetGmailThread(threadID)
	if err != nil {
		return nil, err
	}

	content := ExtractThreadContent(thread)

	return &GmailThreadDetail{
		Title:    "Gmail Thread",
		ThreadID: thread.Id,
		Content:  content,
		Raw:      thread,
	}, nil
}

// GetCalendarEventDetail retrieves a Google Calendar event and produces structured event details.
func (s *Service) GetCalendarEventDetail(eventID string) (*CalendarEventDetail, error) {
	event, err := s.GetCalendarEvent(eventID)
	if err != nil {
		return nil, err
	}

	detail := &CalendarEventDetail{
		Title:       "Calendar Event",
		EventID:     event.Id,
		Summary:     event.Summary,
		Description: event.Description,
		Raw:         event,
	}

	if event.Start != nil {
		detail.Start = event.Start.DateTime
	}
	if event.End != nil {
		detail.End = event.End.DateTime
	}
	detail.Location = event.Location

	return detail, nil
}

// GetKeepNoteDetail retrieves a Keep note and formats its content into a structured detail view.
func (s *Service) GetKeepNoteDetail(ctx context.Context, noteID string) (*KeepNoteDetail, error) {
	note, err := s.GetNote(ctx, noteID)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = "Untitled"
	}

	content := ""
	if note.Body != nil {
		content = ExtractFullContent(note.Body)
	}

	return &KeepNoteDetail{
		ID:         strings.TrimPrefix(note.Name, "notes/"),
		Name:       note.Name,
		Title:      title,
		Content:    content,
		CreateTime: note.CreateTime,
		UpdateTime: note.UpdateTime,
		Raw:        note,
	}, nil
}

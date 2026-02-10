package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	keepapi "google.golang.org/api/keep/v1"
)

const (
	noteSnippetLimit    = 50
	defaultListPageSize = 10
)

var errKeepUnavailable = errors.New("google keep service is not configured")

// ListNotesOptions allows callers to control pagination and filtering.
type ListNotesOptions struct {
	Filter    string
	PageSize  int64
	PageToken string
}

// ListNotes fetches the first 10 notes for the authenticated user and returns summaries.
func (s *Service) ListNotes() ([]Note, error) {
	summaries, _, err := s.ListNoteSummaries(context.Background(), ListNotesOptions{PageSize: defaultListPageSize})
	return summaries, err
}

// ListNoteSummaries returns a page of note summaries and the next page token.
func (s *Service) ListNoteSummaries(ctx context.Context, opts ListNotesOptions) ([]Note, string, error) {
	resp, err := s.listNotes(ctx, opts)
	if err != nil {
		return nil, "", err
	}

	summaries := make([]Note, 0, len(resp.Notes))
	for _, note := range resp.Notes {
		summaries = append(summaries, summarizeNote(note))
	}

	return summaries, resp.NextPageToken, nil
}

// ListAllNoteSummaries exhausts the iterator and returns all note summaries matching the options.
func (s *Service) ListAllNoteSummaries(ctx context.Context, opts ListNotesOptions) ([]Note, error) {
	var all []Note
	pageToken := opts.PageToken
	for {
		opts.PageToken = pageToken
		page, next, err := s.ListNoteSummaries(ctx, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		pageToken = next
	}
	return all, nil
}

// ListKeepNotes returns a page of raw keep notes and the next page token.
func (s *Service) ListKeepNotes(ctx context.Context, opts ListNotesOptions) ([]*keepapi.Note, string, error) {
	resp, err := s.listNotes(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	return resp.Notes, resp.NextPageToken, nil
}

// ListAllKeepNotes fetches every note matching the supplied options.
func (s *Service) ListAllKeepNotes(ctx context.Context, opts ListNotesOptions) ([]*keepapi.Note, error) {
	var all []*keepapi.Note
	pageToken := opts.PageToken
	for {
		opts.PageToken = pageToken
		notes, next, err := s.ListKeepNotes(ctx, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, notes...)
		if next == "" {
			break
		}
		pageToken = next
	}
	return all, nil
}

// GetNote retrieves a single keep note.
func (s *Service) GetNote(ctx context.Context, noteID string) (*keepapi.Note, error) {
	svc, err := s.ensureKeepService()
	if err != nil {
		return nil, err
	}
	name := ensureNoteName(noteID)
	note, err := svc.Notes.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get note %s: %w", name, err)
	}
	return note, nil
}

// CreateNote submits a fully specified keep note to the API.
func (s *Service) CreateNote(ctx context.Context, note *keepapi.Note) (*keepapi.Note, error) {
	if note == nil {
		return nil, errors.New("note must not be nil")
	}
	svc, err := s.ensureKeepService()
	if err != nil {
		return nil, err
	}
	created, err := svc.Notes.Create(note).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to create note: %w", err)
	}
	return created, nil
}

// CreateTextNote is a convenience for creating a note containing only text.
func (s *Service) CreateTextNote(ctx context.Context, title, content string) (*keepapi.Note, error) {
	return s.CreateNote(ctx, &keepapi.Note{
		Title: title,
		Body: &keepapi.Section{
			Text: &keepapi.TextContent{Text: content},
		},
	})
}

// ListItemInput describes a single entry in a list note, including optional nesting.
type ListItemInput struct {
	Text     string
	Checked  bool
	Children []ListItemInput
}

// CreateListNote is a convenience for building list-based notes.
func (s *Service) CreateListNote(ctx context.Context, title string, items []ListItemInput) (*keepapi.Note, error) {
	listItems := buildListItems(items)
	return s.CreateNote(ctx, &keepapi.Note{
		Title: title,
		Body: &keepapi.Section{
			List: &keepapi.ListContent{ListItems: listItems},
		},
	})
}

// DeleteNote removes a keep note permanently.
func (s *Service) DeleteNote(ctx context.Context, noteID string) error {
	svc, err := s.ensureKeepService()
	if err != nil {
		return err
	}
	name := ensureNoteName(noteID)
	_, err = svc.Notes.Delete(name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("unable to delete note %s: %w", name, err)
	}
	return nil
}

// AddNoteWriters grants writer access to the specified note for the provided emails.
func (s *Service) AddNoteWriters(ctx context.Context, noteID string, writerEmails []string) ([]*keepapi.Permission, error) {
	if len(writerEmails) == 0 {
		return nil, nil
	}
	svc, err := s.ensureKeepService()
	if err != nil {
		return nil, err
	}

	parent := ensureNoteName(noteID)
	requests := make([]*keepapi.CreatePermissionRequest, 0, len(writerEmails))
	for _, raw := range writerEmails {
		email := strings.TrimSpace(raw)
		if email == "" {
			continue
		}
		requests = append(requests, &keepapi.CreatePermissionRequest{
			Parent: parent,
			Permission: &keepapi.Permission{
				Role:  "WRITER",
				Email: email,
			},
		})
	}
	if len(requests) == 0 {
		return nil, nil
	}

	resp, err := svc.Notes.Permissions.BatchCreate(parent, &keepapi.BatchCreatePermissionsRequest{Requests: requests}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to add writer permissions for %s: %w", parent, err)
	}
	return resp.Permissions, nil
}

// RemoveNotePermissions revokes the supplied permission resource names from the note.
func (s *Service) RemoveNotePermissions(ctx context.Context, noteID string, permissionNames []string) error {
	if len(permissionNames) == 0 {
		return nil
	}
	svc, err := s.ensureKeepService()
	if err != nil {
		return err
	}

	parent := ensureNoteName(noteID)
	names := make([]string, 0, len(permissionNames))
	for _, raw := range permissionNames {
		normalized := normalizePermissionName(parent, raw)
		if normalized != "" {
			names = append(names, normalized)
		}
	}
	if len(names) == 0 {
		return nil
	}

	_, err = svc.Notes.Permissions.BatchDelete(parent, &keepapi.BatchDeletePermissionsRequest{Names: names}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("unable to remove permissions for %s: %w", parent, err)
	}
	return nil
}

// GetAttachmentMetadata fetches metadata for a single attachment.
func (s *Service) GetAttachmentMetadata(ctx context.Context, attachmentName string) (*keepapi.Attachment, error) {
	svc, err := s.ensureKeepService()
	if err != nil {
		return nil, err
	}
	attachment, err := svc.Media.Download(attachmentName).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to fetch attachment %s metadata: %w", attachmentName, err)
	}
	return attachment, nil
}

// DownloadAttachmentMedia downloads the raw bytes for an attachment.
func (s *Service) DownloadAttachmentMedia(ctx context.Context, attachmentName, mimeType string) ([]byte, error) {
	svc, err := s.ensureKeepService()
	if err != nil {
		return nil, err
	}
	call := svc.Media.Download(attachmentName).Context(ctx)
	if mimeType != "" {
		call.MimeType(mimeType)
	}
	resp, err := call.Download()
	if err != nil {
		return nil, fmt.Errorf("unable to download attachment %s: %w", attachmentName, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read attachment %s: %w", attachmentName, err)
	}
	return data, nil
}

func (s *Service) ensureKeepService() (*keepapi.Service, error) {
	if s.keepService == nil {
		return nil, errKeepUnavailable
	}
	return s.keepService, nil
}

func (s *Service) listNotes(ctx context.Context, opts ListNotesOptions) (*keepapi.ListNotesResponse, error) {
	svc, err := s.ensureKeepService()
	if err != nil {
		return nil, err
	}
	call := svc.Notes.List()
	if opts.Filter != "" {
		call.Filter(opts.Filter)
	}
	if opts.PageSize > 0 {
		call.PageSize(opts.PageSize)
	}
	if opts.PageToken != "" {
		call.PageToken(opts.PageToken)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to list notes: %w", err)
	}
	return resp, nil
}

func summarizeNote(note *keepapi.Note) Note {
	if note == nil {
		return Note{Title: "Untitled", Snippet: "..."}
	}

	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = "Untitled"
	}

	return Note{
		ID:      note.Name,
		Title:   title,
		Snippet: noteSnippet(note.Body),
	}
}

func noteSnippet(section *keepapi.Section) string {
	if section == nil {
		return "..."
	}
	if section.Text != nil && section.Text.Text != "" {
		return truncateSnippet(section.Text.Text)
	}
	if section.List != nil {
		count := len(section.List.ListItems)
		if count > 0 {
			return fmt.Sprintf("[%d list items]", count)
		}
	}
	return "..."
}

func truncateSnippet(src string) string {
	if len(src) <= noteSnippetLimit {
		return src
	}
	if noteSnippetLimit <= 3 {
		return src[:noteSnippetLimit]
	}
	return src[:noteSnippetLimit-3] + "..."
}

func buildListItems(inputs []ListItemInput) []*keepapi.ListItem {
	if len(inputs) == 0 {
		return nil
	}
	items := make([]*keepapi.ListItem, 0, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input.Text) == "" && len(input.Children) == 0 {
			continue
		}
		item := &keepapi.ListItem{Checked: input.Checked}
		if strings.TrimSpace(input.Text) != "" {
			item.Text = &keepapi.TextContent{Text: input.Text}
		}
		if len(input.Children) > 0 {
			item.ChildListItems = buildListItems(input.Children)
		}
		items = append(items, item)
	}
	return items
}

func normalizePermissionName(parent, name string) string {
	candidate := strings.TrimSpace(name)
	if candidate == "" {
		return ""
	}
	if strings.HasPrefix(candidate, "notes/") {
		return candidate
	}
	candidate = strings.TrimPrefix(candidate, "permissions/")
	return fmt.Sprintf("%s/permissions/%s", parent, candidate)
}

func ensureNoteName(noteID string) string {
	trimmed := strings.TrimSpace(noteID)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "notes/") {
		return trimmed
	}
	return "notes/" + trimmed
}

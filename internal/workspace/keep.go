package workspace

import (
	"fmt"
)

// ListNotes fetches the first 10 notes for the authenticated user
func (s *Service) ListNotes() ([]Note, error) {
	// The Keep API 'list' call
	resp, err := s.keepService.Notes.List().PageSize(10).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to list notes: %w", err)
	}

	var notes []Note
	for _, n := range resp.Notes {
		title := "Untitled"
		if n.Title != "" {
			title = n.Title
		}

		// Attempt to extract text content for a snippet
		snippet := ""
		if n.Body != nil {
			if n.Body.Text != nil {
				// FIX: Changed .Value to .Text based on the Keep API struct definition
				snippet = n.Body.Text.Text
			} else if n.Body.List != nil {
				snippet = fmt.Sprintf("[%d list items]", len(n.Body.List.ListItems))
			}
		}

		if snippet == "" {
			snippet = "..."
		} else if len(snippet) > 50 {
			snippet = snippet[:47] + "..."
		}

		notes = append(notes, Note{
			ID:      n.Name, // Keep API returns ID in the 'Name' field (e.g. "notes/123...")
			Title:   title,
			Snippet: snippet,
		})
	}

	return notes, nil
}

package workspace

import (
	"fmt"

	admin "google.golang.org/api/admin/directory/v1"
	keep "google.golang.org/api/keep/v1"
)

// Service wraps the Google Workspace APIs
type Service struct {
	adminService *admin.Service
	keepService  *keep.Service
}

// User represents a simplified user structure
type User struct {
	Name  string
	Email string
	ID    string
}

// Note represents a simplified Keep note
type Note struct {
	Title   string
	Snippet string
	ID      string
}

// NewService creates a new workspace service wrapper
func NewService(adminSvc *admin.Service, keepSvc *keep.Service) *Service {
	return &Service{
		adminService: adminSvc,
		keepService:  keepSvc,
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

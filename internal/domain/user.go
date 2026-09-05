package domain

import "time"

// User represents a user entity in the system
type User struct {
	ID           string    `json:"id" db:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Email        string    `json:"email" db:"email" example:"rami@mail.ru"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Name         string    `json:"name" db:"name" example:"Rami Suliman"`
	CreatedAt    time.Time `json:"created_at" db:"created_at" example:"2024-01-15T10:30:00Z"`
}

// Response converts the user to its public representation.
func (u *User) Response() UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: u.CreatedAt}
}

// RegisterRequest represents user registration data
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email,max=255" example:"rami@mail.ru"`
	Password string `json:"password" validate:"required,min=8,max=72" example:"StrongPass123!"`
	Name     string `json:"name" validate:"required,notblank,min=2,max=255" example:"Rami Suliman"`
}

// LoginRequest represents user login credentials
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255" example:"rami@mail.ru"`
	Password string `json:"password" validate:"required,max=72" example:"StrongPass123!"`
}

// UserResponse represents user data returned in API responses
type UserResponse struct {
	ID        string    `json:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Email     string    `json:"email" example:"rami@mail.ru"`
	Name      string    `json:"name" example:"Rami Suliman"`
	CreatedAt time.Time `json:"created_at,omitzero" example:"2024-01-15T10:30:00Z"`
}

// UpdateProfileRequest is the body of PATCH /users/me.
type UpdateProfileRequest struct {
	Name string `json:"name" validate:"required,notblank,min=2,max=255" example:"Rami Suliman"`
}

// ChangePasswordRequest is the body of PUT /users/me/password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,max=72" example:"OldPass123!"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=72,nefield=CurrentPassword" example:"NewPass456!"`
}

// LoginResponse represents successful login response
type LoginResponse struct {
	User        UserResponse `json:"user"`
	AccessToken string       `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	TokenType   string       `json:"token_type" example:"Bearer"`
	ExpiresIn   int64        `json:"expires_in" example:"900"`
}

// RefreshResponse represents token refresh response
type RefreshResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	TokenType   string `json:"token_type" example:"Bearer"`
	ExpiresIn   int64  `json:"expires_in" example:"900"`
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	ExpiresIn    int64  `json:"expires_in" example:"900"`
}

// AuthResult represents authentication result with user and tokens
type AuthResult struct {
	User  *User      `json:"user"`
	Token *TokenPair `json:"token"`
}

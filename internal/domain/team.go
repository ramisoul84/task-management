package domain

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Team struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedBy string    `json:"created_by" db:"created_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type TeamMember struct {
	TeamID string `json:"team_id" db:"team_id"`
	UserID string `json:"user_id" db:"user_id"`
	Role   Role   `json:"role" db:"role"`
}

type CreateTeamRequest struct {
	Name string `json:"name" validate:"required,min=2,max=255"`
}

type InviteMemberRequest struct {
	UserID string `json:"user_id" validate:"required,uuid"`
}

type ChangeMemberRoleRequest struct {
	Role Role `json:"role" validate:"required,oneof=admin member"`
}

package domain

import "time"

// Role represents user's role in a team
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// IsManager reports whether the role can manage the team (owner or admin).
func (r Role) IsManager() bool {
	return r == RoleOwner || r == RoleAdmin
}

// Team represents a team entity
type Team struct {
	ID        string    `json:"id" db:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Name      string    `json:"name" db:"name" example:"Development Team"`
	CreatedBy string    `json:"created_by" db:"created_by" example:"123e4567-e89b-12d3-a456-426614174001"`
	CreatedAt time.Time `json:"created_at" db:"created_at" example:"2024-01-15T10:30:00Z"`
	// Role is the current user's role in the team (set on list endpoints).
	Role Role `json:"role,omitempty" db:"role" example:"owner"`
}

// TeamMember represents a member in a team
type TeamMember struct {
	TeamID string `json:"team_id" db:"team_id" example:"123e4567-e89b-12d3-a456-426614174000"`
	UserID string `json:"user_id" db:"user_id" example:"123e4567-e89b-12d3-a456-426614174001"`
	Role   Role   `json:"role" db:"role" example:"member"`
}

// CreateTeamRequest represents team creation data
type CreateTeamRequest struct {
	Name string `json:"name" validate:"required,notblank,min=2,max=255" example:"Development Team"`
}

// InviteMemberRequest represents member invitation data
type InviteMemberRequest struct {
	UserID string `json:"user_id" validate:"required,uuid" example:"123e4567-e89b-12d3-a456-426614174001"`
}

// ChangeMemberRoleRequest represents role change data
type ChangeMemberRoleRequest struct {
	Role Role `json:"role" validate:"required,oneof=admin member" example:"admin"`
}

// TeamMemberResponse represents team member data returned in responses
type TeamMemberResponse struct {
	UserID string `json:"user_id" db:"user_id" example:"123e4567-e89b-12d3-a456-426614174001"`
	Email  string `json:"email" db:"email" example:"rami@mail.ru"`
	Name   string `json:"name"  db:"name" example:"Rami Suliman"`
	Role   Role   `json:"role" db:"role" example:"member"`
}

// TeamStats represents team analytics report
type TeamStats struct {
	TasksByStatus map[TaskStatus]int `json:"tasks_by_status"`
	TopPerformers []TopPerformer     `json:"top_performers"`
	AvgCloseTime  float64            `json:"avg_close_time_hours" example:"48.5"`
	TotalComments int                `json:"total_comments" example:"25"`
	TotalTasks    int                `json:"total_tasks" example:"20"`
}

// TopPerformer represents a user with closed tasks count
type TopPerformer struct {
	UserID      string `json:"user_id" db:"user_id" example:"123e4567-e89b-12d3-a456-426614174001"`
	UserName    string `json:"user_name" db:"user_name" example:"John Doe"`
	ClosedTasks int    `json:"closed_tasks" db:"closed_tasks" example:"6"`
}

package models

import "time"

// Role — роль пользователя (спека 99.2.14 §2).
type Role string

const (
	RolePlayer      Role = "player"
	RoleAdmin       Role = "admin"
	RoleSkycomposer Role = "skycomposer"
)

type User struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	PasswordHash   string    `json:"-"`
	Email          *string   `json:"email,omitempty"`
	AgentID        *string   `json:"agent_id,omitempty"`
	CurrentWorldID *string   `json:"current_world_id,omitempty"` // текущий мир
	ShipIcon       string    `json:"ship_icon"`                  // выбранная иконка корабля (PNG-имя)
	ShipColor      *string   `json:"ship_color"`                 // цвет перекраски спрайта (NULL = «Оригинал», спека 61b §3.3)
	Role           Role      `json:"role"`                       // player/admin/skycomposer
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

package models

import "time"

type User struct {
    ID             string     `json:"id"`
    Username       string     `json:"username"`
    PasswordHash   string     `json:"-"`
    Email          *string    `json:"email,omitempty"`
    AgentID        *string    `json:"agent_id,omitempty"`
    CurrentWorldID *string    `json:"current_world_id,omitempty"` // текущий мир
    ShipIcon       string     `json:"ship_icon"`                  // выбранная иконка корабля
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
}
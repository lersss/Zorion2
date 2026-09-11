package repository

import (
	"database/sql"
	"fmt"
	"time"

	"zorion/internal/models"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create создаёт нового пользователя
func (r *UserRepository) Create(user *models.User) error {
	query := `
		INSERT INTO users (id, username, password_hash, email, agent_id, current_world_id, ship_icon, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now()
	_, err := r.db.Exec(query,
		user.ID,
		user.Username,
		user.PasswordHash,
		user.Email,
		user.AgentID,
		user.CurrentWorldID,
		user.ShipIcon,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

// GetByUsername возвращает пользователя по логину
func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	query := `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, created_at, updated_at FROM users WHERE username = $1`
	row := r.db.QueryRow(query, username)

	var u models.User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.Email,
		&u.AgentID,
		&u.CurrentWorldID,
		&u.ShipIcon,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByID возвращает пользователя по ID
func (r *UserRepository) GetByID(id string) (*models.User, error) {
	query := `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, created_at, updated_at FROM users WHERE id = $1`
	row := r.db.QueryRow(query, id)

	var u models.User
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.Email,
		&u.AgentID,
		&u.CurrentWorldID,
		&u.ShipIcon,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateCurrentWorld обновляет текущий мир пользователя
func (r *UserRepository) UpdateCurrentWorld(userID, worldID string) error {
	query := `UPDATE users SET current_world_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, worldID, userID)
	return err
}

// UpdateShipIcon обновляет выбранную иконку корабля
func (r *UserRepository) UpdateShipIcon(userID, icon string) error {
	query := `UPDATE users SET ship_icon = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, icon, userID)
	return err
}
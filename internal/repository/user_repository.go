package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"zorion/internal/models"
)

// Сентинел-ошибки инвариантов раздела «Пользователи» (спека 99.2.14 §7):
// И1 — в системе всегда ≥ 1 учётка skycomposer; И2 — нельзя менять/удалять себя.
var (
	ErrSelfChange      = errors.New("нельзя менять или удалять самого себя")
	ErrLastSkycomposer = errors.New("нельзя снять последнего skycomposer")
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create создаёт нового пользователя. Роль по умолчанию — player.
func (r *UserRepository) Create(user *models.User) error {
	role := user.Role
	if role == "" {
		role = models.RolePlayer
	}
	query := `
		INSERT INTO users (id, username, password_hash, email, agent_id, current_world_id, ship_icon, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
		role,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	user.Role = role
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

// GetByUsername возвращает пользователя по логину
func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	query := `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE username = $1`
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
		&u.ShipColor,
		&u.Role,
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
	query := `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users WHERE id = $1`
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
		&u.ShipColor,
		&u.Role,
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

// UpdateShipColor обновляет цвет перекраски спрайта корабля (спека 61b §7):
// color == nil — «Оригинал» (NULL), иначе hex из палитры.
func (r *UserRepository) UpdateShipColor(userID string, color *string) error {
	query := `UPDATE users SET ship_color = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, color, userID)
	return err
}

// userSelect — общие колонки для листинга.
const userSelect = `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, role, created_at, updated_at FROM users`

// Count возвращает число пользователей под фильтрами раздела «Пользователи»
// (§6.1): подстрока по username/email (case-insensitive) + фильтр роли.
func (r *UserRepository) Count(query, role string) (int, error) {
	where, args := userFilterWhere(query, role)
	sqlQuery := `SELECT COUNT(*) FROM users` + where
	var n int
	if err := r.db.QueryRow(sqlQuery, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// List возвращает страницу пользователей (§6.1). page — 1-based, limit — размер
// страницы. Сортировка стабильная: по дате создания убывающе, затем по id.
func (r *UserRepository) List(query, role string, page, limit int) ([]*models.User, error) {
	where, args := userFilterWhere(query, role)
	args = append(args, limit, (page-1)*limit)
	sqlQuery := userSelect + where +
		` ORDER BY created_at DESC, id LIMIT $` + fmt.Sprint(len(args)-1) +
		` OFFSET $` + fmt.Sprint(len(args))
	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.PasswordHash,
			&u.Email,
			&u.AgentID,
			&u.CurrentWorldID,
			&u.ShipIcon,
			&u.ShipColor,
			&u.Role,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// userFilterWhere собирает WHERE-часть для поиска/фильтра списка пользователей.
// Символы LIKE-шаблона в query экранируются, чтобы поиск был подстрокой буквально.
func userFilterWhere(query, role string) (string, []interface{}) {
	var conds []string
	var args []interface{}
	if query != "" {
		q := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
		conds = append(conds, `(username ILIKE '%' || $1 || '%' ESCAPE '\' OR email ILIKE '%' || $1 || '%' ESCAPE '\')`)
		args = append(args, q)
	}
	if role != "" {
		conds = append(conds, fmt.Sprintf(`role = $%d`, len(args)+1))
		args = append(args, role)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return ` WHERE ` + strings.Join(conds, " AND "), args
}

// UpdateRole меняет роль пользователя. Инварианты §7 проверяются в той же
// транзакции, что и изменение (защита от гонки):
//   - И1: нельзя снять последнего skycomposer;
//   - И2: нельзя менять роль самому себе (actingUserID == id).
//
// Возвращает sql.ErrNoRows, если пользователь не найден.
func (r *UserRepository) UpdateRole(id string, newRole models.Role, actingUserID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var curRole models.Role
	err = tx.QueryRow(`SELECT role FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&curRole)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}

	if id == actingUserID {
		return ErrSelfChange
	}

	if curRole == models.RoleSkycomposer && newRole != models.RoleSkycomposer {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM users WHERE role = $1`, models.RoleSkycomposer,
		).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastSkycomposer
		}
	}

	if _, err := tx.Exec(
		`UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`, newRole, id,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdatePassword обновляет пароль пользователя (bcrypt-хэш).
// Возвращает sql.ErrNoRows, если пользователь не найден.
func (r *UserRepository) UpdatePassword(id, passwordHash string) error {
	res, err := r.db.Exec(
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		passwordHash, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete удаляет пользователя. Инварианты §7 проверяются в той же транзакции,
// что и удаление: И1 — нельзя удалить последнего skycomposer; И2 — нельзя
// удалить самого себя (actingUserID == id). Возвращает sql.ErrNoRows, если
// пользователь не найден.
func (r *UserRepository) Delete(id string, actingUserID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var role models.Role
	err = tx.QueryRow(`SELECT role FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&role)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}

	if id == actingUserID {
		return ErrSelfChange
	}

	if role == models.RoleSkycomposer {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM users WHERE role = $1`, models.RoleSkycomposer,
		).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastSkycomposer
		}
	}

	if _, err := tx.Exec(`DELETE FROM users WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CountByRole возвращает число пользователей с данной ролью (бутстрап §5).
func (r *UserRepository) CountByRole(role models.Role) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE role = $1`, role,
	).Scan(&n)
	return n, err
}

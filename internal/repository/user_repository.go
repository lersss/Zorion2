package repository

import (
	"database/sql"
	"encoding/json"
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
// Стартовая комплектация 77a (спека §3.3): ship_model_id='starter',
// equipment={radar:radar_1, scanner:scanner_1, engine:null} — если не заданы
// явно (как дефолты 61b при регистрации).
func (r *UserRepository) Create(user *models.User) error {
	role := user.Role
	if role == "" {
		role = models.RolePlayer
	}
	shipModelID := user.ShipModelID
	if shipModelID == nil {
		s := models.StarterShipModelID
		shipModelID = &s
	}
	equipment := user.Equipment
	if equipment == nil {
		equipment = models.StarterEquipment
	}
	equipmentJSON, err := json.Marshal(equipment)
	if err != nil {
		return fmt.Errorf("create user: equipment marshal: %w", err)
	}
	query := `
		INSERT INTO users (id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_model_id, equipment, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	now := time.Now()
	_, err = r.db.Exec(query,
		user.ID,
		user.Username,
		user.PasswordHash,
		user.Email,
		user.AgentID,
		user.CurrentWorldID,
		user.ShipIcon,
		shipModelID,
		equipmentJSON,
		role,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	user.Role = role
	user.ShipModelID = shipModelID
	user.Equipment = equipment
	user.CreatedAt = now
	user.UpdatedAt = now
	return nil
}

// GetByUsername возвращает пользователя по логину
func (r *UserRepository) GetByUsername(username string) (*models.User, error) {
	query := userSelect + ` WHERE username = $1`
	row := r.db.QueryRow(query, username)
	return scanUser(row)
}

// GetByID возвращает пользователя по ID
func (r *UserRepository) GetByID(id string) (*models.User, error) {
	query := userSelect + ` WHERE id = $1`
	row := r.db.QueryRow(query, id)
	return scanUser(row)
}

// scanUser — сканирует одну строку users (общие колонки userSelect).
func scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	var shipModelID sql.NullString
	var equipmentRaw []byte
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.Email,
		&u.AgentID,
		&u.CurrentWorldID,
		&u.ShipIcon,
		&u.ShipColor,
		&shipModelID,
		&equipmentRaw,
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
	if shipModelID.Valid {
		u.ShipModelID = &shipModelID.String
	}
	if len(equipmentRaw) > 0 && string(equipmentRaw) != "null" {
		if err := json.Unmarshal(equipmentRaw, &u.Equipment); err != nil {
			return nil, err
		}
	}
	return &u, nil
}

// UpdateCurrentWorld обновляет текущий мир пользователя
func (r *UserRepository) UpdateCurrentWorld(userID, worldID string) error {
	query := `UPDATE users SET current_world_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, worldID, userID)
	return err
}

// UpdateCurrentWorldAndPosition — атомарно: current_world_id + current_position
// одним UPDATE (спека 99.2.27 §3.6.3, С-1): прибытие межзвёздного полёта →
// «орбита звезды» (ИП-2); позиция никогда не остаётся битой между двумя
// апдейтами (краш-окно закрыто).
func (r *UserRepository) UpdateCurrentWorldAndPosition(userID, worldID string, pos *models.CurrentPosition) error {
	posJSON, err := json.Marshal(pos)
	if err != nil {
		return fmt.Errorf("update world+position: marshal: %w", err)
	}
	_, err = r.db.Exec(
		`UPDATE users SET current_world_id = $1, current_position = $2, updated_at = NOW() WHERE id = $3`,
		worldID, posJSON, userID,
	)
	return err
}

// ClearCurrentWorld — обнуляет current_world_id и current_position (игрок
// «без мира»): дефенсив onArrival при съеденной цели (пакман, спека
// 2026-09-20 §7.2) — вместо FK-violation на worlds.
func (r *UserRepository) ClearCurrentWorld(userID string) error {
	_, err := r.db.Exec(
		`UPDATE users SET current_world_id = NULL, current_position = NULL, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

// GetByIDWithPosition — GetByID + внутрисистемная позиция (спека 99.2.27
// §4.3/§4.4): users.current_position JSONB. NULL-позиция → pos = nil.
// Аддитивно (99.2.30 §6.3): users.pending_destination JSONB — намерение
// композитного маршрута; NULL → dest = nil.
func (r *UserRepository) GetByIDWithPosition(id string) (*models.User, *models.CurrentPosition, *models.PendingDestination, error) {
	query := `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at, current_position, pending_destination FROM users WHERE id = $1`
	row := r.db.QueryRow(query, id)
	return scanUserWithPosition(row)
}

// scanUserWithPosition — сканирует строку users + current_position +
// pending_destination.
func scanUserWithPosition(row *sql.Row) (*models.User, *models.CurrentPosition, *models.PendingDestination, error) {
	var u models.User
	var shipModelID sql.NullString
	var equipmentRaw []byte
	var posRaw []byte
	var destRaw []byte
	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.PasswordHash,
		&u.Email,
		&u.AgentID,
		&u.CurrentWorldID,
		&u.ShipIcon,
		&u.ShipColor,
		&shipModelID,
		&equipmentRaw,
		&u.Role,
		&u.CreatedAt,
		&u.UpdatedAt,
		&posRaw,
		&destRaw,
	)
	if err == sql.ErrNoRows {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if shipModelID.Valid {
		u.ShipModelID = &shipModelID.String
	}
	if len(equipmentRaw) > 0 && string(equipmentRaw) != "null" {
		if err := json.Unmarshal(equipmentRaw, &u.Equipment); err != nil {
			return nil, nil, nil, err
		}
	}
	var pos *models.CurrentPosition
	if len(posRaw) > 0 && string(posRaw) != "null" {
		if err := json.Unmarshal(posRaw, &pos); err != nil {
			return nil, nil, nil, err
		}
	}
	var dest *models.PendingDestination
	if len(destRaw) > 0 && string(destRaw) != "null" {
		if err := json.Unmarshal(destRaw, &dest); err != nil {
			return nil, nil, nil, err
		}
	}
	return &u, pos, dest, nil
}

// GetPendingDestination — намерение композитного маршрута игрока (спека
// 99.2.30 §4.1): users.pending_destination JSONB; nil = намерения нет.
func (r *UserRepository) GetPendingDestination(userID string) (*models.PendingDestination, error) {
	var destRaw []byte
	err := r.db.QueryRow(`SELECT pending_destination FROM users WHERE id = $1`, userID).Scan(&destRaw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(destRaw) == 0 || string(destRaw) == "null" {
		return nil, nil
	}
	var dest models.PendingDestination
	if err := json.Unmarshal(destRaw, &dest); err != nil {
		return nil, err
	}
	return &dest, nil
}

// ListPendingDestinations — игроки с намерением композитного маршрута
// (спека 99.2.30 §4.5, фаза 3 Restore): SELECT id, pending_destination FROM
// users WHERE pending_destination IS NOT NULL. Стоимость — O(игроки с
// намерением) (И8).
func (r *UserRepository) ListPendingDestinations() ([]*models.User, error) {
	rows, err := r.db.Query(`SELECT id, pending_destination FROM users WHERE pending_destination IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		var destRaw []byte
		if err := rows.Scan(&u.ID, &destRaw); err != nil {
			return nil, err
		}
		if len(destRaw) > 0 && string(destRaw) != "null" {
			var dest models.PendingDestination
			if err := json.Unmarshal(destRaw, &dest); err != nil {
				return nil, err
			}
			u.PendingDestination = &dest
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// SetPendingDestination — запись/очистка намерения композитного маршрута
// (спека 99.2.30 §3.5, M2): 202-идемпотентный путь /travel (полёт к req.WorldID
// уже идёт, позиция уже NULL) — отдельный UPDATE (полёт не перезапускается).
// dest == nil → NULL (игрок явно «перелетел» к звезде).
func (r *UserRepository) SetPendingDestination(userID string, dest *models.PendingDestination) error {
	destJSON, err := marshalDestination(dest)
	if err != nil {
		return fmt.Errorf("set pending destination: marshal: %w", err)
	}
	_, err = r.db.Exec(
		`UPDATE users SET pending_destination = $1, updated_at = NOW() WHERE id = $2`,
		destJSON, userID,
	)
	return err
}

// ClearPendingDestination — очистка намерения (спека 99.2.30 §4.4/§4.5):
// UPDATE users SET pending_destination = NULL. Точки: исполнение автостарта
// (в любом исходе), дефенсив onArrival (мир не совпал / съеден), Restore.
func (r *UserRepository) ClearPendingDestination(userID string) error {
	_, err := r.db.Exec(
		`UPDATE users SET pending_destination = NULL, updated_at = NOW() WHERE id = $1`,
		userID,
	)
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

// userSelect — общие колонки для листинга (включая 77a: ship_model_id, equipment).
const userSelect = `SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users`

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
		var shipModelID sql.NullString
		var equipmentRaw []byte
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.PasswordHash,
			&u.Email,
			&u.AgentID,
			&u.CurrentWorldID,
			&u.ShipIcon,
			&u.ShipColor,
			&shipModelID,
			&equipmentRaw,
			&u.Role,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if shipModelID.Valid {
			u.ShipModelID = &shipModelID.String
		}
		if len(equipmentRaw) > 0 && string(equipmentRaw) != "null" {
			if err := json.Unmarshal(equipmentRaw, &u.Equipment); err != nil {
				return nil, err
			}
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

// PlayerPositions — игроки для карты (спека 77a §5.3): id, username,
// ship_icon, ship_color, current_world_id, role, current_position. Без
// password_hash/email — позиции чужих игроков не должны тянуть лишнее.
// current_position (спека 99.2.27 §4.5): стоящие на орбите видны в радиусе
// (изменение 90a, решение создателя С2). При малом числе онлайн-игроков —
// просто запрос с фильтром по радиусу на чтении (§11.3).
func (r *UserRepository) PlayerPositions() ([]*models.User, error) {
	rows, err := r.db.Query(
		`SELECT id, username, ship_icon, ship_color, current_world_id, role, current_position FROM users`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		var posRaw []byte
		if err := rows.Scan(&u.ID, &u.Username, &u.ShipIcon, &u.ShipColor, &u.CurrentWorldID, &u.Role, &posRaw); err != nil {
			return nil, err
		}
		if len(posRaw) > 0 && string(posRaw) != "null" {
			var pos models.CurrentPosition
			if err := json.Unmarshal(posRaw, &pos); err != nil {
				return nil, err
			}
			u.CurrentPosition = &pos
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

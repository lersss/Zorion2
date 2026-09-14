package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"zorion/internal/models"
)

// NPCRepository — доступ к таблице npc_agents (спека 20a.1 §2.1).
type NPCRepository struct {
	db *sql.DB
}

func NewNPCRepository(db *sql.DB) *NPCRepository {
	return &NPCRepository{db: db}
}

// zeroUUID — курсор «с начала»: любой валидный uuid больше нулевого,
// uuid не сравнивается с text, поэтому пустой курсор кодируется нулём.
const zeroUUID = "00000000-0000-0000-0000-000000000000"

// npcAgentColumns — порядок колонок всех SELECT по npc_agents (совпадает
// с модельным скан-порядком scanNPCAgents).
const npcAgentColumns = `id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at`

// ListBatch — курсорная выборка до limit агентов со статусом status и
// id > after (спека 20a.1 §2.2.A: cursor-based, не OFFSET). after = "" —
// с начала (нулевой uuid). Результат меньше limit строк — конец круга,
// планировщик сбрасывает курсор.
func (r *NPCRepository) ListBatch(status models.NPCAgentStatus, after string, limit int) ([]models.NPCAgent, error) {
	if after == "" {
		after = zeroUUID
	}
	query := `SELECT ` + npcAgentColumns + ` FROM npc_agents WHERE status = $1 AND id > $2 ORDER BY id LIMIT $3`
	rows, err := r.db.Query(query, status, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNPCAgents(rows)
}

// ListDueArrivals — курсорная выборка прибытий: status='flying' и
// arrive_at <= now (спека 20a.1 §3.1 п.1). Прибытия — приоритет бюджета
// тика (§2.2.A), поэтому фильтр по времени в SQL, а не в памяти.
func (r *NPCRepository) ListDueArrivals(now time.Time, after string, limit int) ([]models.NPCAgent, error) {
	if after == "" {
		after = zeroUUID
	}
	query := `SELECT ` + npcAgentColumns + ` FROM npc_agents WHERE status = 'flying' AND arrive_at <= $1 AND id > $2 ORDER BY id LIMIT $3`
	rows, err := r.db.Query(query, now, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNPCAgents(rows)
}

// scanNPCAgents — сканирование строк курсорных SELECT в модели.
func scanNPCAgents(rows *sql.Rows) ([]models.NPCAgent, error) {
	var agents []models.NPCAgent
	for rows.Next() {
		a, err := scanNPCAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// scanNPCAgent — сканирование одной строки SELECT npcAgentColumns; nullable
// колонки полёта/наблюдения — в nil-указатели. Работает с *sql.Row и *sql.Rows.
func scanNPCAgent(scanner interface{ Scan(dest ...any) error }) (models.NPCAgent, error) {
	var a models.NPCAgent
	var from, target sql.NullString
	var depart, arrive, observed sql.NullTime
	if err := scanner.Scan(&a.ID, &a.Name, &a.Status, &a.CurrentWorldID,
		&from, &target, &depart, &arrive, &a.NotifyEnabled, &observed,
		&a.CreatedAt, &a.UpdatedAt); err != nil {
		return models.NPCAgent{}, err
	}
	if from.Valid {
		a.FromWorldID = &from.String
	}
	if target.Valid {
		a.TargetWorldID = &target.String
	}
	if depart.Valid {
		a.DepartAt = &depart.Time
	}
	if arrive.Valid {
		a.ArriveAt = &arrive.Time
	}
	if observed.Valid {
		a.LastObservedAt = &observed.Time
	}
	return a, nil
}

// GetByID — агент по id; nil, nil — если не найден.
func (r *NPCRepository) GetByID(id string) (*models.NPCAgent, error) {
	a, err := scanNPCAgent(r.db.QueryRow(`SELECT `+npcAgentColumns+` FROM npc_agents WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Update — частичное обновление агента из админки (PATCH /admin/npc/{id},
// спека §8): name и notify_enabled — независимые поля, nil-указатель —
// поле не меняется. sql.ErrNoRows — агента нет.
func (r *NPCRepository) Update(id string, name *string, notifyEnabled *bool) error {
	var sets []string
	var args []interface{}
	arg := 1
	if name != nil {
		sets = append(sets, "name = $"+strconv.Itoa(arg))
		args = append(args, *name)
		arg++
	}
	if notifyEnabled != nil {
		sets = append(sets, "notify_enabled = $"+strconv.Itoa(arg))
		args = append(args, *notifyEnabled)
		arg++
	}
	if len(sets) == 0 {
		return nil // нет полей — нечего обновлять
	}
	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	query := "UPDATE npc_agents SET " + strings.Join(sets, ", ") + " WHERE id = $" + strconv.Itoa(arg)
	res, err := r.db.Exec(query, args...)
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

// ListAll — все агенты без фильтров (спека 20a.1 §2.2.B: позиции всех
// агентов пересчитываются на каждый тик; источник правды — БД).
func (r *NPCRepository) ListAll() ([]models.NPCAgent, error) {
	rows, err := r.db.Query(`SELECT ` + npcAgentColumns + ` FROM npc_agents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNPCAgents(rows)
}

// UpdateStatusBatch применяет смены состояния агентов одной транзакцией
// (спека 20a.1 §2.2.A: «Всё изменение одного агента — одной транзакцией»,
// batch UPDATE вместо транзакции на каждого — §2.3). Каждая смена — один
// UPDATE одной строки, атомарен сам по себе; транзакция на весь batch
// даёт «всё или ничего» на тик. Пустой список — без обращения к БД.
func (r *NPCRepository) UpdateStatusBatch(updates []models.AgentStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, u := range updates {
		if err := applyAgentUpdate(tx, u); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// applyAgentUpdate — один UPDATE по типу смены состояния (спека 20a.1 §3.1,
// §4): прибытие (flying → idle, без пересчёта населения) или старт
// (idle → flying, заполняется кортеж полёта).
func applyAgentUpdate(tx *sql.Tx, u models.AgentStatusUpdate) error {
	switch u.Status {
	case models.NPCAgentStatusIdle: // прибытие
		_, err := tx.Exec(`UPDATE npc_agents SET status = 'idle', current_world_id = $1, last_observed_at = $2, updated_at = NOW() WHERE id = $3`,
			u.CurrentWorldID, u.LastObservedAt, u.ID)
		return err
	case models.NPCAgentStatusFlying: // старт
		_, err := tx.Exec(`UPDATE npc_agents SET status = 'flying', from_world_id = $1, target_world_id = $2, depart_at = $3, arrive_at = $4, updated_at = NOW() WHERE id = $5`,
			u.FromWorldID, u.TargetWorldID, u.DepartAt, u.ArriveAt, u.ID)
		return err
	default:
		return fmt.Errorf("unknown npc agent status %q", u.Status)
	}
}

// Insert создаёт агента (спека 20a.1 §8): статус idle на стартовом мире,
// дальше подхватывает планировщик. Пустой Status в модели → idle.
func (r *NPCRepository) Insert(a *models.NPCAgent) error {
	if a.Status == "" {
		a.Status = models.NPCAgentStatusIdle
	}
	// Создание всегда с уведомлениями (спека §8: POST {name, start_world_id?} —
	// поля уведомлений нет; выключение per-agent — через PATCH). Совпадает с
	// DEFAULT true в БД; PATCH — отдельный метод.
	a.NotifyEnabled = true
	query := `INSERT INTO npc_agents (id, name, status, current_world_id, from_world_id, target_world_id, depart_at, arrive_at, notify_enabled, last_observed_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`
	_, err := r.db.Exec(query,
		a.ID, a.Name, a.Status, a.CurrentWorldID,
		a.FromWorldID, a.TargetWorldID, a.DepartAt, a.ArriveAt,
		a.NotifyEnabled, a.LastObservedAt)
	if err != nil {
		return err
	}
	// Модель — для ответа API (как WorldRepository.Create): БД ставит
	// NOW(), здесь — то же значение для ответа.
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	return nil
}

// Delete удаляет агента по id; sql.ErrNoRows — если агента нет.
func (r *NPCRepository) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM npc_agents WHERE id = $1`, id)
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
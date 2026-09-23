package repository

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"zorion/internal/models"
	"zorion/internal/npc"
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

// ==================== ПУЛ «РАСА → РОДНОЙ МИР» (спека 2026-09-23 §5.1) =========

// RaceHomeworlds — пул «раса → родной мир» для генерации агентов: по одной
// фракции на расу (uq_factions_race, миграция 000066). factions.homeworld_id
// ссылается на planets(id), а агент живёт в мире (worlds) — возвращается мир
// родной планеты (planets.world_id), чтобы CurrentWorldID был валидным миром
// (npc_agents.current_world_id REFERENCES worlds(id)).
func (r *NPCRepository) RaceHomeworlds() ([]npc.RaceHomeworld, error) {
	rows, err := r.db.Query(`
		SELECT f.race_id, p.world_id
		FROM factions f
		JOIN planets p ON p.id = f.homeworld_id
		WHERE f.race_id IS NOT NULL AND f.homeworld_id IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []npc.RaceHomeworld
	for rows.Next() {
		var o npc.RaceHomeworld
		if err := rows.Scan(&o.RaceID, &o.HomeworldID); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// UpdateStatusBatch применяет смены состояния агентов одной транзакцией
// (спека 20a.1 §2.2.A: «Всё изменение одного агента — одной транзакцией»,
// batch UPDATE вместо транзакции на каждого — §2.3). Каждый тип смены
// состояния (прибытие / старт) — ОДИН multi-row UPDATE через VALUES-джойн
// (идея 26c, A1): до 2000 одиночных Exec на тик → 2 запроса. Транзакция
// на весь batch даёт «всё или ничего» на тик. Пустой список — без
// обращения к БД.
func (r *NPCRepository) UpdateStatusBatch(updates []models.AgentStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	// Прибытие (flying → idle) и старт (idle → flying) пишут разные наборы
	// колонок — две формы запроса (statusBatchArrivalQuery / statusBatchStartQuery).
	// Внутри одного вызова id уникальны (планировщик шлёт отдельные батчи
	// для прибытий и стартов), порядок форм между собой не значим.
	arrivals := make([]models.AgentStatusUpdate, 0, len(updates))
	starts := make([]models.AgentStatusUpdate, 0, len(updates))
	for _, u := range updates {
		switch u.Status {
		case models.NPCAgentStatusIdle:
			arrivals = append(arrivals, u)
		case models.NPCAgentStatusFlying:
			starts = append(starts, u)
		default:
			return fmt.Errorf("unknown npc agent status %q", u.Status)
		}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(arrivals) > 0 {
		query, args := statusBatchArrivalQuery(arrivals)
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}
	if len(starts) > 0 {
		query, args := statusBatchStartQuery(starts)
		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// statusBatchArrivalQuery — multi-row UPDATE прибытия (flying → idle):
// current_world_id = цель полёта, last_observed_at = now (спека 20a.1 §3.1,
// §4: без пересчёта населения). VALUES-джойн: 2000 прибытий = 1 запрос.
func statusBatchArrivalQuery(updates []models.AgentStatusUpdate) (string, []interface{}) {
	var sb strings.Builder
	sb.WriteString(`UPDATE npc_agents SET status = 'idle', current_world_id = v.current_world_id, last_observed_at = v.last_observed_at, updated_at = NOW() FROM (VALUES `)
	args := make([]interface{}, 0, len(updates)*3)
	for i, u := range updates {
		if i > 0 {
			sb.WriteString(", ")
		}
		n := i * 3
		sb.WriteString(fmt.Sprintf("($%d::uuid, $%d::uuid, $%d::timestamptz)", n+1, n+2, n+3))
		args = append(args, u.ID, u.CurrentWorldID, u.LastObservedAt)
	}
	sb.WriteString(`) AS v(id, current_world_id, last_observed_at) WHERE npc_agents.id = v.id`)
	return sb.String(), args
}

// statusBatchStartQuery — multi-row UPDATE старта (idle → flying):
// заполняется кортеж полёта (from/target/depart/arrive, спека §3.2).
func statusBatchStartQuery(updates []models.AgentStatusUpdate) (string, []interface{}) {
	var sb strings.Builder
	sb.WriteString(`UPDATE npc_agents SET status = 'flying', from_world_id = v.from_world_id, target_world_id = v.target_world_id, depart_at = v.depart_at, arrive_at = v.arrive_at, updated_at = NOW() FROM (VALUES `)
	args := make([]interface{}, 0, len(updates)*5)
	for i, u := range updates {
		if i > 0 {
			sb.WriteString(", ")
		}
		n := i * 5
		sb.WriteString(fmt.Sprintf("($%d::uuid, $%d::uuid, $%d::uuid, $%d::timestamptz, $%d::timestamptz)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, u.ID, u.FromWorldID, u.TargetWorldID, u.DepartAt, u.ArriveAt)
	}
	sb.WriteString(`) AS v(id, from_world_id, target_world_id, depart_at, arrive_at) WHERE npc_agents.id = v.id`)
	return sb.String(), args
}

// Insert создаёт агента (спека 20a.1 §8): статус idle на стартовом мире,
// дальше подхватывает планировщик. Пустой Status в модели → idle.
// notify_enabled = false — дефолт новых агентов (спека 26a.1 §7.4: пуши —
// только через глобальный рубильник; был true до 26a).
func (r *NPCRepository) Insert(a *models.NPCAgent) error {
	if a.Status == "" {
		a.Status = models.NPCAgentStatusIdle
	}
	a.NotifyEnabled = false
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

// DeleteAll — массовое удаление всех агентов: один DELETE без WHERE
// (правка создателя 2026-09-15: атомарно и быстро, НЕ одиночные DELETE
// по id). Возвращает число удалённых строк.
func (r *NPCRepository) DeleteAll() (int64, error) {
	res, err := r.db.Exec(`DELETE FROM npc_agents`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==================== МАССОВАЯ ГЕНЕРАЦИЯ (спека 26a.1 §4) ====================

// BulkInsert — массовое создание агентов одной COPY-операцией (спека §4.2,
// pq.CopyIn, паттерн planet_data_batch.go: в 5–10 раз быстрее multi-row,
// без лимита 65535 параметров). Одна транзакция — «всё или ничего».
// notify_enabled = false для всех (§7.4: пуши — только через глобальный
// рубильник). Повторный запуск с тем же count — новая пачка.
func (r *NPCRepository) BulkInsert(agents []models.NPCAgent) error {
	if len(agents) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Колонки создания; кортеж полёта не входит — DEFAULT NULL (спека §4.2).
	// race_id — раса агента (спека 2026-09-23 §5.2): генерация всегда её ставит.
	stmt, err := tx.Prepare(pq.CopyIn("npc_agents",
		"id", "name", "status", "current_world_id", "race_id", "notify_enabled", "created_at", "updated_at"))
	if err != nil {
		return fmt.Errorf("prepare copy npc_agents: %w", err)
	}

	now := time.Now()
	for _, a := range agents {
		if _, err := stmt.Exec(a.ID, a.Name, models.NPCAgentStatusIdle, a.CurrentWorldID, a.RaceID, false, now, now); err != nil {
			stmt.Close()
			return fmt.Errorf("copy npc_agents row: %w", err)
		}
	}
	// Финальный Exec без аргументов — flush.
	if _, err := stmt.Exec(); err != nil {
		stmt.Close()
		return fmt.Errorf("copy npc_agents flush: %w", err)
	}
	if err := stmt.Close(); err != nil {
		return err
	}
	return tx.Commit()
}

// ListNames — все имена агентов (спека §3.2: seed уникальности перед пачкой —
// гарантия между пачками). Одна колонка, один запрос.
func (r *NPCRepository) ListNames() ([]string, error) {
	rows, err := r.db.Query(`SELECT name FROM npc_agents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ==================== ПАГИНАЦИЯ СПИСКА (спека 26a.1 §5) ====================

// ErrInvalidCursor — cursor не является валидной opaque-строкой (хендлер
// отвечает 400).
var ErrInvalidCursor = errors.New("невалидный cursor")

// encodeCursor — opaque-курсор: base64url(created_atRFC3339Nano + "," + id)
// точки последней записи страницы (спека §5.2).
func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "," + id))
}

func decodeCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	parts := strings.SplitN(string(raw), ",", 2)
	if len(parts) != 2 {
		return time.Time{}, "", ErrInvalidCursor
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	return t, parts[1], nil
}

// ListPage — страница агентов keyset-курсором по (created_at, id) (спека
// §5.2): свежие сверху, курсор — точка последней записи предыдущей страницы.
// cursor = "" — первая страница (без WHERE). Возвращает next_cursor ("" —
// страниц больше нет). O(страница) по индексу (created_at DESC, id DESC).
func (r *NPCRepository) ListPage(limit int, cursor string) ([]models.NPCAgent, string, error) {
	query := `SELECT ` + npcAgentColumns + ` FROM npc_agents`
	var args []interface{}
	if cursor != "" {
		t, id, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		query += ` WHERE (created_at, id) < ($1, $2)`
		args = append(args, t, id)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	agents, err := scanNPCAgents(rows)
	if err != nil {
		return nil, "", err
	}

	var next string
	if len(agents) == limit {
		last := agents[len(agents)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return agents, next, nil
}

// ==================== СЧЁТЧИКИ МЕТРИК (спека 26a.1 §8) ====================

func (r *NPCRepository) CountTotal() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM npc_agents`).Scan(&n)
	return n, err
}

// CountByStatus — число агентов в статусе (индекс (status, id) из 000026).
func (r *NPCRepository) CountByStatus(status models.NPCAgentStatus) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM npc_agents WHERE status = $1`, status).Scan(&n)
	return n, err
}

// CountOverdue — очередь на тик: прибытия, которые уже должны были обработаны
// (живой COUNT при запросе метрик, спека §8.1 «вариант (а)»: flying И
// arrive_at <= now — главный индикатор лагов планировщика).
func (r *NPCRepository) CountOverdue(now time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM npc_agents WHERE status = 'flying' AND arrive_at <= $1`, now).Scan(&n)
	return n, err
}

// ==================== ПОИСК ПО ИМЕНИ (спека 26a.1 §6.1) ====================

// SearchByName — регистронезависимый поиск агентов по имени (ILIKE '%q%').
// На 100к — seq scan ~10–50 мс; для разового поиска на карте приемлемо,
// pg_trgm-индекс — кандидат 26b (§11, ограничение 4).
func (r *NPCRepository) SearchByName(q string, limit int) ([]models.NPCAgent, error) {
	rows, err := r.db.Query(
		`SELECT id, name, status, current_world_id, target_world_id FROM npc_agents WHERE name ILIKE $1 LIMIT $2`,
		"%"+q+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.NPCAgent
	for rows.Next() {
		var a models.NPCAgent
		var target sql.NullString
		if err := rows.Scan(&a.ID, &a.Name, &a.Status, &a.CurrentWorldID, &target); err != nil {
			return nil, err
		}
		if target.Valid {
			a.TargetWorldID = &target.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
// internal/repository/ship_repository.go
// Доступ к каталогу деталей кораблей (таблица ship_parts, спека 99.2.15 §2)
// и к схеме корабля игрока (users.ship_visual). Каталог в памяти
// (immutable snapshot + atomic.Pointer) — ship_catalog.go; здесь только БД.
package repository

import (
	"database/sql"
	"encoding/json"

	"zorion/internal/models"
)

// ShipRepository — CRUD каталога ship_parts + чтение/запись users.ship_visual.
type ShipRepository struct {
	db *sql.DB
}

func NewShipRepository(db *sql.DB) *ShipRepository {
	return &ShipRepository{db: db}
}

// shipPartColumns — порядок колонок всех SELECT по ship_parts.
const shipPartColumns = `id, category, name, svg, params, created_at`

// UpsertPart — вставка или обновление детали (перегенерация категории —
// ON CONFLICT DO UPDATE, спека §6/§11 Этап 3). params сериализуется в JSONB.
func (r *ShipRepository) UpsertPart(p *models.ShipPart) error {
	params, err := json.Marshal(p.Params)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`
		INSERT INTO ship_parts (id, category, name, svg, params, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			category = EXCLUDED.category,
			name = EXCLUDED.name,
			svg = EXCLUDED.svg,
			params = EXCLUDED.params
	`, p.ID, p.Category, p.Name, p.SVG, params)
	return err
}

// ListParts — все детали каталога (для загрузки в память и админки).
func (r *ShipRepository) ListParts() ([]models.ShipPart, error) {
	rows, err := r.db.Query(`SELECT ` + shipPartColumns + ` FROM ship_parts ORDER BY category, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShipParts(rows)
}

// PartsByCategory — детали одной категории (для вкладки «Корабли»).
func (r *ShipRepository) PartsByCategory(category string) ([]models.ShipPart, error) {
	rows, err := r.db.Query(`SELECT `+shipPartColumns+` FROM ship_parts WHERE category = $1 ORDER BY id`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShipParts(rows)
}

// GetPart — деталь по id; nil, nil — если не найдена.
func (r *ShipRepository) GetPart(id string) (*models.ShipPart, error) {
	row := r.db.QueryRow(`SELECT `+shipPartColumns+` FROM ship_parts WHERE id = $1`, id)
	p, err := scanShipPart(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePart удаляет деталь по id; sql.ErrNoRows — если детали нет.
func (r *ShipRepository) DeletePart(id string) error {
	res, err := r.db.Exec(`DELETE FROM ship_parts WHERE id = $1`, id)
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

// ==================== СХЕМА ИГРОКА (users.ship_visual) ====================

// SaveShipVisual пишет схему корабля игрока (PUT /me/ship-visual, спека §10).
func (r *ShipRepository) SaveShipVisual(userID string, v *models.ShipVisual) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`UPDATE users SET ship_visual = $1, updated_at = NOW() WHERE id = $2`,
		data, userID,
	)
	return err
}

// GetShipVisual читает схему игрока; nil, nil — «ещё не собирал» (NULL).
func (r *ShipRepository) GetShipVisual(userID string) (*models.ShipVisual, error) {
	var raw []byte
	err := r.db.QueryRow(`SELECT ship_visual FROM users WHERE id = $1`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return models.ShipVisualFromJSON(raw)
}

// ==================== СКАНИРОВАНИЕ ====================

func scanShipParts(rows *sql.Rows) ([]models.ShipPart, error) {
	var parts []models.ShipPart
	for rows.Next() {
		p, err := scanShipPart(rows)
		if err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

// scanShipPart — одна строка SELECT shipPartColumns. Работает с *sql.Row
// и *sql.Rows.
func scanShipPart(scanner interface{ Scan(dest ...any) error }) (models.ShipPart, error) {
	var p models.ShipPart
	var params []byte
	if err := scanner.Scan(&p.ID, &p.Category, &p.Name, &p.SVG, &params, &p.CreatedAt); err != nil {
		return models.ShipPart{}, err
	}
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p.Params); err != nil {
			return models.ShipPart{}, err
		}
	}
	return p, nil
}
// internal/repository/deposit_repository.go
//
// Залежи поверхности (спека 2026-09-22-поселение-добыча-сырья-биома-ленивый-
// буфер §3.3): чтение залежей планет системы построчно (§5.2 — группировку
// делает клиент), вставка (админ-ручка §6), счётчик залежей ресурса во всех
// мирах для предупреждения при удалении ресурса (§3.3, T14).
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"zorion/internal/models"
)

// depositSelectByPlanetsSQL — залежи планет системы одним запросом
// (`= ANY($1)`, инвариант 2). good_name — JOIN goods; порядок стабильный
// (planet_id, id) для детерминированного ответа.
const depositSelectByPlanetsSQL = `
	SELECT d.id, d.planet_id, d.good_id, g.name, d.stratum, d.wealth, d.amount
	FROM deposits d
	JOIN goods g ON g.id = d.good_id
	WHERE d.planet_id = ANY($1)
	ORDER BY d.planet_id, d.id
`

// countDepositsByGoodSQL — число залежей ресурса во всех мирах (§3.3/T14).
// Единственный источник числа: этим же SQL считает DeleteGood
// (goods_repository.go) — метод CountDepositsByGood.
const countDepositsByGoodSQL = `SELECT COUNT(*) FROM deposits WHERE good_id = $1`

// rowQueryer — общее для *sql.DB и *sql.Tx: предпроверка залежей внутри
// транзакции удаления (DeleteGood) и отдельный счётчик — один SQL.
type rowQueryer interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// countDepositsByGood — число залежей ресурса (единый SQL для удаления и
// предпроверки, §3.3/T14).
func countDepositsByGood(q rowQueryer, goodID int64) (int, error) {
	var n int
	if err := q.QueryRow(countDepositsByGoodSQL, goodID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count deposits: %w", err)
	}
	return n, nil
}

// DepositRepository — доступ к таблице deposits.
type DepositRepository struct {
	db *sql.DB
}

func NewDepositRepository(db *sql.DB) *DepositRepository {
	return &DepositRepository{db: db}
}

// GetDepositsByPlanetIDs — залежи планет одной системы, сгруппированные по
// planet_id (GET /api/worlds/{id}/planets → attachDeposits). Пустой вход —
// пустая карта без запроса.
func (r *DepositRepository) GetDepositsByPlanetIDs(planetIDs []string) (map[string][]models.SurfaceDeposit, error) {
	return getDepositsByPlanetIDs(context.Background(), r.db, planetIDs)
}

// GetDepositsByPlanetIDsTx — то же чтение внутри транзакции (админ-ручка
// «добавить залежь» §6: чтение блока deposits после INSERT в одной tx).
func (r *DepositRepository) GetDepositsByPlanetIDsTx(ctx context.Context, tx *sql.Tx, planetIDs []string) (map[string][]models.SurfaceDeposit, error) {
	return getDepositsByPlanetIDs(ctx, tx, planetIDs)
}

// queryContextExec — общее для *sql.DB и *sql.Tx.
type queryContextExec interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// getDepositsByPlanetIDs — общее чтение залежей для DB и Tx (один SQL).
func getDepositsByPlanetIDs(ctx context.Context, q queryContextExec, planetIDs []string) (map[string][]models.SurfaceDeposit, error) {
	out := make(map[string][]models.SurfaceDeposit, len(planetIDs))
	if len(planetIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, depositSelectByPlanetsSQL, pqStringArray(planetIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query deposits: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d models.SurfaceDeposit
		if err := rows.Scan(&d.ID, &d.PlanetID, &d.GoodID, &d.GoodName, &d.Stratum, &d.Wealth, &d.Amount); err != nil {
			return nil, fmt.Errorf("failed to scan deposit: %w", err)
		}
		out[d.PlanetID] = append(out[d.PlanetID], d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deposits iteration error: %w", err)
	}
	return out, nil
}

// InsertDeposit — вставка залежи (админ-ручка «добавить залежь» §6) в
// переданной транзакции: id и planet_id задаёт вызывающий; число пятен не
// ограничено (§3.2). Транзакция — обязательна: SELECT world_id → INSERT →
// чтение deposits атомарны (§6).
func (r *DepositRepository) InsertDeposit(ctx context.Context, tx *sql.Tx, d models.SurfaceDeposit) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO deposits (id, planet_id, good_id, stratum, wealth, amount, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		d.ID, d.PlanetID, d.GoodID, d.Stratum, d.Wealth, d.Amount,
	)
	if err != nil {
		return fmt.Errorf("failed to insert deposit: %w", err)
	}
	return nil
}

// CountDepositsByGood — число залежей ресурса во всех мирах (§3.3/T14):
// предупреждение при удалении ресурса в студии. Один источник числа —
// используется и предпроверкой, и удалением (DeleteGood).
func (r *DepositRepository) CountDepositsByGood(goodID int64) (int, error) {
	return countDepositsByGood(r.db, goodID)
}

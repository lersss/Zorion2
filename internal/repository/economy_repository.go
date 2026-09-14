package repository

import (
	"database/sql"
	"math"
	"time"

	"github.com/lib/pq"

	"zorion/internal/economy/settlement"
	"zorion/internal/models"
)

type EconomyRepository struct {
	db *sql.DB
}

func NewEconomyRepository(db *sql.DB) *EconomyRepository {
	return &EconomyRepository{db: db}
}

// Settlement
func (r *EconomyRepository) CreateSettlement(s *models.Settlement) error {
	query := `INSERT INTO settlements (id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), NOW())`
	_, err := r.db.Exec(query, s.ID, s.PlanetID, s.Population, float64(s.Population), s.Stability)
	return err
}

// GetSettlementsByPlanetIDs — возвращает поселения планет,
// сгруппированные по planet_id. Пустой список — планета без поселений.
func (r *EconomyRepository) GetSettlementsByPlanetIDs(planetIDs []string) (map[string][]models.Settlement, error) {
	if len(planetIDs) == 0 {
		return map[string][]models.Settlement{}, nil
	}

	query := `SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at
	          FROM settlements WHERE planet_id = ANY($1) ORDER BY created_at ASC`
	rows, err := r.db.Query(query, pqStringArray(planetIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string][]models.Settlement{}
	for rows.Next() {
		var s models.Settlement
		if err := rows.Scan(
			&s.ID, &s.PlanetID, &s.Population,
			&s.PopulationExact, &s.Stability, &s.ComputedAt,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[s.PlanetID] = append(result[s.PlanetID], s)
	}
	return result, rows.Err()
}

// RecomputeSettlementPopulation продвигает население поселения на момент now.
// Чек-точка для пересчёта уже загружена в s (population_exact, computed_at).
// Два пути (docs/gamedesign/18a_population_death.md, решение игрока
// 2026-09-13 — модель «правда на сервере, синк по событию»):
//   - «простой визит» (Δt < MinPersistInterval): население считается только
//     в памяти. Пересчёт — чистая функция от чек-точки (p0·exp(−λ·Δt)), запись
//     на хот-пате чтения не нужна, никаких запросов к БД, кроме уже сделанного.
//   - «событие» (Δt ≥ MinPersistInterval): чек-точка продвигается в БД
//     (транзакция, SELECT ... FOR UPDATE — конкурентная безопасность, два
//     одновременных события не исказят population_exact, AGENTS.md §0,
//     13_tiers_impl.md §13.13.4), чтобы сохранённое население не устаревало
//     для читателей без пересчёта (admin-stats, генератор фракций).
// В обоих путях в ответ попадает актуальное население и чек-точка на момент
// now, а r_per_sec/n_dead — для косметической экстраполяции на клиенте
// (λ-механизм убран, 99.2.13: всё изменение в r_per_sec).
// Если «событие» впервые видит обвал (чек-точка живая, population_exact >
// NDead, и next = 0), той же транзакцией создаётся запись лога «Вымерло»
// (18a §«Лог поселения»): INSERT ... ON CONFLICT DO NOTHING — анти-дубль на
// уровне БД (uq_settlement_log_extinct). Мёртвая чек-точка (population_exact
// ≤ NDead) запись не создаёт — бэкфилл отменён.
func (r *EconomyRepository) RecomputeSettlementPopulation(s *models.Settlement, input settlement.PlanetInput, now time.Time) (models.Settlement, error) {
	rPerSec := settlement.ChangeComponents(input)

	if now.Sub(s.ComputedAt) < settlement.MinPersistInterval {
		next := settlement.Recompute(input, s.PopulationExact, s.ComputedAt, now, s.CreatedAt)
		s.Population = int(math.Round(next))
		s.PopulationExact = next
		s.ComputedAt = now
		s.RPerSec = rPerSec
		s.NDead = settlement.NDead
		return *s, nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return models.Settlement{}, err
	}
	defer tx.Rollback()

	var stored models.Settlement
	err = tx.QueryRow(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at
		FROM settlements WHERE id = $1 FOR UPDATE`, s.ID,
	).Scan(&stored.ID, &stored.PlanetID, &stored.Population, &stored.PopulationExact, &stored.Stability, &stored.ComputedAt, &stored.CreatedAt, &stored.UpdatedAt)
	if err != nil {
		return models.Settlement{}, err
	}

	newExact := settlement.Recompute(input, stored.PopulationExact, stored.ComputedAt, now, stored.CreatedAt)
	newPopulation := int(math.Round(newExact))

	if _, err := tx.Exec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`,
		newPopulation, newExact, now, stored.ID,
	); err != nil {
		return models.Settlement{}, err
	}

	// Обвал с живой чек-точки: население обнулилось целиком (next < NDead →
	// 0) — дата смерти вычислима, пишем «Вымерло» в лог той же транзакцией.
	// INSERT ... ON CONFLICT DO NOTHING: второй одновременный синк упирается
	// в uq_settlement_log_extinct и ничего не пишет (18a §«Анти-дубль и синк»).
	if stored.PopulationExact > settlement.NDead && newExact == 0 {
		deathAt, ok := settlement.DeathTime(stored.PopulationExact, rPerSec, stored.ComputedAt, now, stored.CreatedAt)
		if ok {
			cause := settlement.DeathCause(input)
			if _, err := tx.Exec(`
				INSERT INTO settlement_log (settlement_id, type, occurred_at, cause)
				VALUES ($1, 'extinct', $2, $3)
				ON CONFLICT DO NOTHING`,
				stored.ID, deathAt, cause,
			); err != nil {
				return models.Settlement{}, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Settlement{}, err
	}

	stored.Population = newPopulation
	stored.PopulationExact = newExact
	stored.ComputedAt = now
	stored.RPerSec = rPerSec
	stored.NDead = settlement.NDead
	return stored, nil
}

// GetSettlementLogBySettlementIDs возвращает последние 3 записи лога на
// поселение, сгруппированные по settlement_id, сортировка по дате убывающая
// (18a §«UI»). Пустой список id — пустой результат, без запроса.
func (r *EconomyRepository) GetSettlementLogBySettlementIDs(ids []string) (map[string][]models.SettlementLogEntry, error) {
	if len(ids) == 0 {
		return map[string][]models.SettlementLogEntry{}, nil
	}

	query := `
		SELECT id, settlement_id, type, occurred_at, cause, created_at
		FROM (
			SELECT id, settlement_id, type, occurred_at, cause, created_at,
			       ROW_NUMBER() OVER (PARTITION BY settlement_id ORDER BY occurred_at DESC) AS rn
			FROM settlement_log
			WHERE settlement_id = ANY($1)
		) sub
		WHERE rn <= 3
		ORDER BY occurred_at DESC`
	rows, err := r.db.Query(query, pqStringArray(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string][]models.SettlementLogEntry{}
	for rows.Next() {
		var e models.SettlementLogEntry
		var cause sql.NullString
		if err := rows.Scan(
			&e.ID, &e.SettlementID, &e.Type, &e.OccurredAt, &cause, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if cause.Valid {
			e.Cause = &cause.String
		}
		result[e.SettlementID] = append(result[e.SettlementID], e)
	}
	return result, rows.Err()
}

// pqStringArray — []string в тип для `= ANY($1)`. lib/pq сам кодирует
// []string как text[], но требует тип pq.Array.
func pqStringArray(ids []string) interface{} {
	return pq.Array(ids)
}

// Factory
func (r *EconomyRepository) CreateFactory(f *models.Factory) error {
	query := `INSERT INTO factories (id, planet_id, name, type, input_resource, output_product, quality, status, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())`
	_, err := r.db.Exec(query, f.ID, f.PlanetID, f.Name, f.Type, f.InputResource, f.OutputProduct, f.Quality, f.Status)
	return err
}

// GoodsBatch
func (r *EconomyRepository) CreateGoodsBatch(b *models.GoodsBatch) error {
	query := `INSERT INTO goods_batches (id, planet_id, product_name, quantity, quality, producer_id, produced_at, created_at, expires_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)`
	_, err := r.db.Exec(query, b.ID, b.PlanetID, b.ProductName, b.Quantity, b.Quality, b.ProducerID, b.ProducedAt, b.ExpiresAt)
	return err
}

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
// now, а decay_lambda/n_dead — для косметической экстраполяции на клиенте.
func (r *EconomyRepository) RecomputeSettlementPopulation(s *models.Settlement, input settlement.PlanetInput, now time.Time) (models.Settlement, error) {
	lambda := settlement.TotalLambda(input, settlement.DefaultScale)

	if now.Sub(s.ComputedAt) < settlement.MinPersistInterval {
		next := settlement.Recompute(input, settlement.DefaultScale, s.PopulationExact, s.ComputedAt, now)
		s.Population = int(math.Round(next))
		s.PopulationExact = next
		s.ComputedAt = now
		s.DecayLambda = lambda
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

	newExact := settlement.Recompute(input, settlement.DefaultScale, stored.PopulationExact, stored.ComputedAt, now)
	newPopulation := int(math.Round(newExact))

	if _, err := tx.Exec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`,
		newPopulation, newExact, now, stored.ID,
	); err != nil {
		return models.Settlement{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Settlement{}, err
	}

	stored.Population = newPopulation
	stored.PopulationExact = newExact
	stored.ComputedAt = now
	stored.DecayLambda = lambda
	stored.NDead = settlement.NDead
	return stored, nil
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

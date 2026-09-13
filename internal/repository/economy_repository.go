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

	query := `SELECT id, planet_id, population, stability, created_at, updated_at
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
			&s.Stability, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[s.PlanetID] = append(result[s.PlanetID], s)
	}
	return result, rows.Err()
}

// RecomputeSettlementPopulation пересчитывает и сохраняет население
// поселения от среды планеты (docs/gamedesign/18a_population_death.md) на
// момент now. Блокировка строки в транзакции — конкурентная безопасность:
// два одновременных обращения к одному поселению не должны исказить
// population_exact (AGENTS.md §0, 13_tiers_impl.md §13.13.4).
func (r *EconomyRepository) RecomputeSettlementPopulation(id string, input settlement.PlanetInput, now time.Time) (models.Settlement, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return models.Settlement{}, err
	}
	defer tx.Rollback()

	var s models.Settlement
	err = tx.QueryRow(`
		SELECT id, planet_id, population, population_exact, stability, computed_at, created_at, updated_at
		FROM settlements WHERE id = $1 FOR UPDATE`, id,
	).Scan(&s.ID, &s.PlanetID, &s.Population, &s.PopulationExact, &s.Stability, &s.ComputedAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return models.Settlement{}, err
	}

	newExact := settlement.Recompute(input, settlement.DefaultScale, s.PopulationExact, s.ComputedAt, now)
	newPopulation := int(math.Round(newExact))

	if _, err := tx.Exec(`
		UPDATE settlements SET population = $1, population_exact = $2, computed_at = $3, updated_at = NOW()
		WHERE id = $4`,
		newPopulation, newExact, now, id,
	); err != nil {
		return models.Settlement{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.Settlement{}, err
	}

	s.Population = newPopulation
	s.PopulationExact = newExact
	s.ComputedAt = now
	return s, nil
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
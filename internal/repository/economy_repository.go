package repository

import (
	"database/sql"

	"github.com/lib/pq"

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
	query := `INSERT INTO settlements (id, planet_id, level, population, capacity, stability, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	_, err := r.db.Exec(query, s.ID, s.PlanetID, s.Level, s.Population, s.Capacity, s.Stability)
	return err
}

// GetSettlementsByPlanetIDs — возвращает поселения планет,
// сгруппированные по planet_id. Пустой список — планета без поселений.
func (r *EconomyRepository) GetSettlementsByPlanetIDs(planetIDs []string) (map[string][]models.Settlement, error) {
	if len(planetIDs) == 0 {
		return map[string][]models.Settlement{}, nil
	}

	query := `SELECT id, planet_id, level, population, capacity, stability, created_at, updated_at
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
			&s.ID, &s.PlanetID, &s.Level, &s.Population,
			&s.Capacity, &s.Stability, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[s.PlanetID] = append(result[s.PlanetID], s)
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
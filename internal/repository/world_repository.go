package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"zorion/internal/models"
)

// worldColumns — колонки чтения миров (включая экзотические типы, 99.2.4 §3,
// массу, 29a §4м, и возраст, 41a §3.4). COALESCE(spectral_class,'') — NULL-
// спектр экзотики не роняет Scan.
const worldColumns = `id, name, coord_x, coord_y, COALESCE(spectral_class,''), temperature, star_type, system_type, stellar_mods, stellar_mass, age, created_at, updated_at`

// scanWorld — читает строку мира (worldColumns) в models.World.
func scanWorld(scanner interface{ Scan(...interface{}) error }) (*models.World, error) {
	var w models.World
	var modsRaw sql.NullString
	var massRaw sql.NullFloat64
	var ageRaw sql.NullFloat64
	err := scanner.Scan(
		&w.ID, &w.Name, &w.CoordX, &w.CoordY, &w.SpectralClass, &w.Temperature,
		&w.StarType, &w.SystemType, &modsRaw, &massRaw, &ageRaw, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if modsRaw.Valid && modsRaw.String != "" {
		if err := json.Unmarshal([]byte(modsRaw.String), &w.StellarMods); err != nil {
			return nil, err
		}
	}
	if massRaw.Valid {
		w.StellarMass = &massRaw.Float64
	}
	if ageRaw.Valid {
		w.Age = &ageRaw.Float64
	}
	return &w, nil
}

type WorldRepository struct {
	DB *sql.DB
}

func NewWorldRepository(db *sql.DB) *WorldRepository {
	return &WorldRepository{DB: db}
}

func (r *WorldRepository) Create(world *models.World) error {
	query := `
		INSERT INTO worlds (id, name, coord_x, coord_y, spectral_class, temperature, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	now := time.Now()
	_, err := r.DB.Exec(query, world.ID, world.Name, world.CoordX, world.CoordY, world.SpectralClass, world.Temperature, now, now)
	if err != nil {
		return err
	}
	world.CreatedAt = now
	world.UpdatedAt = now
	return nil
}

func (r *WorldRepository) GetByID(id string) (*models.World, error) {
	query := `SELECT ` + worldColumns + ` FROM worlds WHERE id = $1`
	row := r.DB.QueryRow(query, id)

	w, err := scanWorld(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (r *WorldRepository) GetAll() ([]*models.World, error) {
	query := `SELECT ` + worldColumns + ` FROM worlds ORDER BY name`
	rows, err := r.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var worlds []*models.World
	for rows.Next() {
		w, err := scanWorld(rows)
		if err != nil {
			return nil, err
		}
		worlds = append(worlds, w)
	}
	return worlds, nil
}

func (r *WorldRepository) GetAllPaginated(page, limit int, search string) ([]*models.World, int, error) {
	offset := (page - 1) * limit

	countQuery := `SELECT COUNT(*) FROM worlds`
	args := []interface{}{}
	argIdx := 1

	if search != "" {
		countQuery += ` WHERE name ILIKE $1`
		args = append(args, "%"+search+"%")
	}

	var total int
	err := r.DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + worldColumns + ` FROM worlds`
	if search != "" {
		query += ` WHERE name ILIKE $` + string(rune(48+argIdx))
		argIdx++
	}
	query += ` ORDER BY name LIMIT $` + string(rune(48+argIdx)) + ` OFFSET $` + string(rune(48+argIdx+1))
	args = append(args, limit, offset)

	rows, err := r.DB.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var worlds []*models.World
	for rows.Next() {
		w, err := scanWorld(rows)
		if err != nil {
			return nil, 0, err
		}
		worlds = append(worlds, w)
	}
	return worlds, total, nil
}
package probe

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMockRunner(t *testing.T) (*Runner, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewRunner(db, filepath.Join(t.TempDir(), "results"))
	return r, mock
}

func TestRunNoPlanets(t *testing.T) {
	r, mock := newMockRunner(t)
	mock.ExpectQuery("SELECT id FROM planets").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := r.Run(context.Background(), "linear", DefaultCurveParams(), DefaultRunOptions(), nil)
	if err == nil {
		t.Fatal("ожидал ошибку при пустой БД")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func planetRow(id, worldID, name, typ string, temp, water float64, atmo string) (string, string, string, string) {
	data, _ := json.Marshal(map[string]interface{}{
		"type": typ, "temperature": temp, "water_percent": water, "atmosphere": atmo,
	})
	return id, worldID, name, string(data)
}

func TestRunComputesCurves(t *testing.T) {
	r, mock := newMockRunner(t)
	mock.ExpectQuery("SELECT id FROM planets").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("p1").AddRow("p2"))

	p1, w1, n1, d1 := planetRow("p1", "w1", "Земля-1", "землеподобная", 280, 60, "азотно-кислородная")
	p2, w2, n2, d2 := planetRow("p2", "w2", "Лёд-1", "ледяная", 80, 5, "метановая")
	mock.ExpectQuery("SELECT id, world_id, name, data FROM planets WHERE id = ANY").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "data"}).
			AddRow(p1, w1, n1, d1).AddRow(p2, w2, n2, d2))

	res, err := r.Run(context.Background(), "linear", DefaultCurveParams(), DefaultRunOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || len(res.Curves) != 2 {
		t.Fatalf("ожидал 2, получил total=%v curves=%v", res.Total, len(res.Curves))
	}
	if res.Curves[0].Class != ClassEarthlike {
		t.Fatalf("класс землеподобной не распознан: %v", res.Curves[0].Class)
	}
	if res.Stats.Count != 2 {
		t.Fatalf("статистика не посчитана: %+v", res.Stats)
	}
	if _, ok := r.Get(res.ID); !ok {
		t.Fatal("результат не сохранён в памяти")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunSampling(t *testing.T) {
	r, mock := newMockRunner(t)
	mock.ExpectQuery("SELECT id FROM planets").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("p1").AddRow("p2").AddRow("p3"))

	mock.ExpectQuery("SELECT id, world_id, name, data FROM planets WHERE id = ANY").
		WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "data"}).
			AddRow("p1", "w1", "Земля-1", `{}`).AddRow("p2", "w2", "Лёд-1", `{}`))

	opts := DefaultRunOptions()
	opts.SampleSize = 2
	res, err := r.Run(context.Background(), "linear", DefaultCurveParams(), opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("выборка 2 из 3, получил total=%v", res.Total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunDeterministicSample(t *testing.T) {
	// Тот же seed → та же выборка.
	r1, m1 := newMockRunner(t)
	r2, m2 := newMockRunner(t)
	ids := []string{"p1", "p2", "p3", "p4", "p5"}
	for _, m := range []sqlmock.Sqlmock{m1, m2} {
		rows := sqlmock.NewRows([]string{"id"})
		for _, id := range ids {
			rows.AddRow(id)
		}
		m.ExpectQuery("SELECT id FROM planets").WillReturnRows(rows)
		m.ExpectQuery("SELECT id, world_id, name, data FROM planets WHERE id = ANY").
			WillReturnRows(sqlmock.NewRows([]string{"id", "world_id", "name", "data"}).
				AddRow("p1", "w1", "Земля-1", `{}`).AddRow("p2", "w2", "Лёд-1", `{}`))
	}

	opts := DefaultRunOptions()
	opts.SampleSize = 2

	ctx := context.Background()
	res1, err := r1.Run(ctx, "x", DefaultCurveParams(), opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := r2.Run(ctx, "x", DefaultCurveParams(), opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res1.Curves[0].PlanetID != res2.Curves[0].PlanetID ||
		res1.Curves[1].PlanetID != res2.Curves[1].PlanetID {
		t.Fatalf("тот же seed — та же выборка: %v vs %v",
			res1.Curves, res2.Curves)
	}
}
// internal/repository/content_import_entry.go
// Точка входа импорта: ImportContent — одна транзакция с advisory-локом
// каталога; dryRun — тот же прогон с ROLLBACK. При dry_run дифф возвращается
// без ошибки даже при blocked/unmatched; при применении blocked/unmatched дают
// 409 и полный откат (никакого частичного применения, §5.4/§5.5).
package repository

import (
	"database/sql"
	"fmt"

	"zorion/internal/goodsstudio/contentio"
)

// --- импорт ---

// ImportContent — импорт снимка (§5). Одна транзакция с advisory-локом
// каталога; dryRun — тот же прогон с ROLLBACK. При dry_run дифф возвращается
// без ошибки даже при blocked/unmatched; при применении blocked/unmatched дают
// 409 и полный откат (никакого частичного применения, §5.4/§5.5).
func (r *GoodsRepository) ImportContent(snap *contentio.Snapshot, dryRun bool) (*ImportResult, error) {
	if snap == nil {
		return nil, errCatalog(400, "пустой снимок")
	}
	if snap.SchemaVersion > contentio.SupportedSchemaVersion {
		return nil, errCatalog(400, fmt.Sprintf("schema_version %d не поддерживается (максимум %d)",
			snap.SchemaVersion, contentio.SupportedSchemaVersion))
	}

	tx, err := r.beginMutation()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res := &ImportResult{DryRun: dryRun, Diff: &ImportDiff{
		Create: map[string]int{}, Update: map[string]int{},
		Delete: []ImportDiffEntry{}, Blocked: []ImportBlocked{}, Unmatched: []ImportDiffEntry{},
		Remap: []ImportRemapEntry{},
	}}
	diff := res.Diff

	st, err := loadImportState(tx)
	if err != nil {
		return nil, err
	}
	mode, unmatched := resolveImportMode(st, snap)
	st.mode = mode
	diff.Mode, res.Mode = mode, mode
	diff.Unmatched = unmatched

	snapProducers := orderSnapshotProducers(snap.ProducerTypes)
	if err := validateSnapshotRefs(snap, snapProducers); err != nil {
		return nil, err
	}
	st.buildIndexes()

	plan, err := buildImportPlan(st, snap, snapProducers, diff)
	if err != nil {
		return nil, err
	}
	if err := collectImportDeletions(st, plan, diff, tx); err != nil {
		return nil, err
	}

	if !dryRun && (len(diff.Blocked) > 0 || len(diff.Unmatched) > 0) {
		return res, errCatalog(409, fmt.Sprintf("импорт отклонён: заблокировано %d, расхождений %d (§5.4)",
			len(diff.Blocked), len(diff.Unmatched)))
	}
	if dryRun {
		return res, nil
	}

	report := &ImportReport{Created: map[string]int{}, Updated: map[string]int{}, Deleted: map[string]int{}}
	res.Report = report
	if err := applyImport(tx, st, snap, snapProducers, plan, report); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func codeOrEmpty(c sql.NullString) string {
	if c.Valid {
		return c.String
	}
	return ""
}

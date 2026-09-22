// internal/economy/settlement/balancer_presets.go
// Слой пресетов кривых балансировщика (спека 99.2.17 §5/§6, итерация 7):
// пресет = полный набор точек (узлы + изгибы) ОДНОЙ R-компоненты. Хранение —
// JSON-файл (дефолт `config/balancer_presets.json`, env BALANCER_PRESETS_FILE
// переопределяет; путь стабильный, не зависит от cwd). Один набор на сервер
// (без владельцев). «Сохранил = сразу применил»: SavePreset пишет пресет в
// файл И делает кривую активной в store (как PUT). Активные пресеты
// переживают рестарт (LoadBalancerPresets при старте). Все операции — под
// balancerStore.mu (store и файл меняются согласованно); запись файла —
// атомарная (tmp + os.Rename).
package settlement

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// BalancerPreset — пресет кривой одной компоненты (§5). Recovery — скаляр
// `recovery` эффект-компоненты (`hunger`, §7.1): nullable — старые пресеты
// heat/cold/gravity/radiation читаются как nil, поведение не меняется.
type BalancerPreset struct {
	Name      string        `json:"name"`
	Component string        `json:"component"`
	Nodes     []SegmentNode `json:"nodes"`
	Bends     []float64     `json:"bends"`
	Recovery  *float64      `json:"recovery,omitempty"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// BalancerPresetSummary — компактное представление для GET presets (§6):
// только имена + даты (узлы не нужны до применения).
type BalancerPresetSummary struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

// balancerPresetsFile — формат файла (§5): плоский список + активные имена.
type balancerPresetsFile struct {
	Presets []BalancerPreset  `json:"presets"`
	Active  map[string]string `json:"active"` // component → имя активного пресета
}

// balancerPresetLayer — состояние слоя пресетов. ВСЕ поля под
// balancerStore.mu (та же блокировка, что и у кривых — store и файл
// меняются согласованно, см. §5 «Атомарность записи»).
type balancerPresetLayer struct {
	presets []BalancerPreset
	active  map[string]string
	path    string
}

var balancerPresetState = balancerPresetLayer{
	presets: nil,
	active:  map[string]string{},
	path:    "",
}

// ErrPresetNotFound — пресет (component, name) не найден (404 на уровне хендлера).
var ErrPresetNotFound = errors.New("пресет не найден")

// balancerComponentOrder — стабильный порядок компонент (map — нестабильный).
var balancerComponentOrder = []string{"heat", "cold", "gravity", "radiation", HungerCurveKey}

// presetRecovery — скаляр `recovery` пресета: у эффект-компоненты заполнен
// (nil → заводское значение, обратная совместимость старых файлов, §7.1); у
// не-эффект-компонент скаляра нет (ok=false).
func presetRecovery(component string, p *BalancerPreset) (float64, bool) {
	if !effectComponents[component] {
		return 0, false
	}
	if p != nil && p.Recovery != nil {
		return *p.Recovery, true
	}
	return defaultComponentScalar(component), true
}

// presetRecoveryPtr — указатель на скаляр пресета для записи в файл (nil для
// не-эффект-компонент).
func presetRecoveryPtr(component string, value float64) *float64 {
	if !effectComponents[component] {
		return nil
	}
	v := value
	return &v
}

// presetNameRe — допустимые символы имени пресета: буквы любых алфавитов
// (включая кириллицу), цифры, подчёркивание, дефис, пробел (§6).
var presetNameRe = regexp.MustCompile(`^[\p{L}\p{N}_\- ]+$`)

// validatePresetName — имя: непустое, ≤ 32 символа, допустимые символы.
func validatePresetName(name string) error {
	if name == "" {
		return fmt.Errorf("имя пресета не может быть пустым")
	}
	if len([]rune(name)) > 32 {
		return fmt.Errorf("имя пресета длиннее 32 символов")
	}
	if !presetNameRe.MatchString(name) {
		return fmt.Errorf("имя пресета содержит недопустимые символы (буквы/цифры/пробел/дефис/подчёркивание)")
	}
	return nil
}

// LoadBalancerPresets — загрузка/создание файла пресетов при старте (§5):
// нет файла → создать из заводских дефолтов (default*Curve) и записать;
// битый JSON → ошибка (кривые = дефолты, сервер не падает — лог на стороне
// main.go; путь уже установлен — первый SavePreset пересоздаст файл);
// невалидные пресеты пропускаются с лог-предупреждением; активные → SetCurve,
// fallback на default (с перезаписью файла, если что-то изменилось).
func LoadBalancerPresets(path string) error {
	if path == "" {
		path = "config/balancer_presets.json"
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("balancer presets: не удалось создать каталог %s: %w", dir, err)
	}

	// Путь устанавливаем до чтения: при битом JSON пресеты остаются
	// сессионными, но первый SavePreset сможет пересоздать файл (§5).
	balancerCurveStore.mu.Lock()
	balancerPresetState.path = path
	balancerPresetState.presets = nil
	balancerPresetState.active = map[string]string{}
	balancerCurveStore.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return createPresetsFileLocked(path)
		}
		return fmt.Errorf("balancer presets: чтение %s: %w", path, err)
	}

	var f balancerPresetsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("balancer presets: битый JSON в %s: %w", path, err)
	}

	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()

	changed := false
	for _, p := range f.Presets {
		if !ValidComponent(p.Component) {
			log.Printf("balancer presets: пресет %q/%q пропущен: неизвестная компонента", p.Name, p.Component)
			changed = true
			continue
		}
		if err := validatePresetName(p.Name); err != nil {
			log.Printf("balancer presets: пресет %q/%q пропущен: %v", p.Name, p.Component, err)
			changed = true
			continue
		}
		if err := validateCurve(p.Component, ComponentCurve{Nodes: p.Nodes, Bends: p.Bends}); err != nil {
			log.Printf("balancer presets: пресет %q/%q пропущен: %v", p.Name, p.Component, err)
			changed = true
			continue
		}
		if p.Recovery != nil && *p.Recovery < 0 {
			log.Printf("balancer presets: пресет %q/%q пропущен: recovery < 0 (%v)", p.Name, p.Component, *p.Recovery)
			changed = true
			continue
		}
		balancerPresetState.presets = append(balancerPresetState.presets, clonePreset(&p))
	}

	// Активные пресеты → SetCurve; отсутствующий/невалидный — fallback на
	// default (кривая = кодовый дефолт, active перезаписывается). Скаляр
	// `recovery` эффект-компоненты восстанавливается вместе с кривой (§7.1).
	for _, comp := range balancerComponentOrder {
		p := findPreset(comp, f.Active[comp])
		if p == nil || !ValidComponent(p.Component) {
			balancerCurveStore.curves[comp] = defaultCurve(comp)
			if effectComponents[comp] {
				balancerCurveStore.scalars[comp] = defaultComponentScalar(comp)
			}
			balancerPresetState.active[comp] = "default"
			changed = true
			continue
		}
		balancerCurveStore.curves[comp] = cloneCurve(&ComponentCurve{Nodes: p.Nodes, Bends: p.Bends})
		if v, ok := presetRecovery(comp, p); ok {
			balancerCurveStore.scalars[comp] = v
		}
		balancerPresetState.active[comp] = p.Name
	}

	// Заводской «default» каждой компоненты обязан существовать: если в
	// файле его нет (правили руками) — добавить из кода.
	for _, comp := range balancerComponentOrder {
		if findPreset(comp, "default") == nil {
			d := defaultCurve(comp)
			balancerPresetState.presets = append(balancerPresetState.presets, BalancerPreset{
				Name:      "default",
				Component: comp,
				Nodes:     append([]SegmentNode(nil), d.Nodes...),
				Bends:     append([]float64(nil), d.Bends...),
				Recovery:  presetRecoveryPtr(comp, defaultComponentScalar(comp)),
				UpdatedAt: time.Now().UTC(),
			})
			changed = true
		}
	}

	if changed {
		return writePresetsFileLocked()
	}
	return nil
}

// createPresetsFileLocked — создание файла с заводскими пресетами (§5 п.1):
// default для каждой компоненты из default*Curve(), active = default.
func createPresetsFileLocked(path string) error {
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()

	now := time.Now().UTC()
	f := balancerPresetsFile{Active: map[string]string{}}
	for _, comp := range balancerComponentOrder {
		d := defaultCurve(comp)
		f.Presets = append(f.Presets, BalancerPreset{
			Name:      "default",
			Component: comp,
			Nodes:     append([]SegmentNode(nil), d.Nodes...),
			Bends:     append([]float64(nil), d.Bends...),
			Recovery:  presetRecoveryPtr(comp, defaultComponentScalar(comp)),
			UpdatedAt: now,
		})
		f.Active[comp] = "default"
	}
	balancerPresetState.presets = clonePresets(f.Presets)
	balancerPresetState.active = f.Active
	balancerPresetState.path = path
	return writePresetsFileLocked()
}

// SavePresetsFile — атомарная запись файла пресетов (отдельный вызов;
// все операции слоя пишут сами — этот для теста атомарности §9 тест 17).
func SavePresetsFile() error {
	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()
	return writePresetsFileLocked()
}

// writePresetsFileLocked — атомарная запись файла под Lock: tmp в той же
// директории + os.Rename (атомарный на одной ФС) — при падении посреди
// записи старый файл остаётся целым (§5). Ошибка записи — только лог на
// стороне вызывающего (режим отказа: пресеты сессионные).
func writePresetsFileLocked() error {
	path := balancerPresetState.path
	if path == "" {
		return fmt.Errorf("balancer presets: путь файла не задан")
	}
	f := balancerPresetsFile{
		Presets: clonePresets(balancerPresetState.presets),
		Active:  map[string]string{},
	}
	for k, v := range balancerPresetState.active {
		f.Active[k] = v
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("balancer presets: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("balancer presets: каталог %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("balancer presets: запись tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("balancer presets: rename: %w", err)
	}
	return nil
}

// ListPresets — компактный список пресетов компоненты + имя активного
// (§6 GET presets). ok = false для неизвестной компоненты.
func ListPresets(component string) ([]BalancerPresetSummary, string, bool) {
	if !ValidComponent(component) {
		return nil, "", false
	}
	balancerCurveStore.mu.RLock()
	defer balancerCurveStore.mu.RUnlock()

	sum := []BalancerPresetSummary{}
	for _, p := range balancerPresetState.presets {
		if p.Component == component {
			sum = append(sum, BalancerPresetSummary{Name: p.Name, UpdatedAt: p.UpdatedAt})
		}
	}
	sort.Slice(sum, func(i, j int) bool { return sum[i].Name < sum[j].Name })
	return sum, balancerPresetState.active[component], true
}

// SavePreset — «Сохранить как пресет» (§6 POST presets): текущая кривая
// компоненты из store под именем. Перезапись занятого имени разрешена
// (идемпотентно, updated_at обновляется). «Сохранил = сразу применил»:
// кривая уже в store (активна), active[component] = name, файл пишется
// атомарно. Валидация имени → error (422 на уровне хендлера).
func SavePreset(component, name string) (BalancerPreset, error) {
	if !ValidComponent(component) {
		return BalancerPreset{}, fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	if err := validatePresetName(name); err != nil {
		return BalancerPreset{}, err
	}

	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()

	c, ok := balancerCurveStore.curves[component]
	if !ok {
		return BalancerPreset{}, fmt.Errorf("кривая компоненты %q недоступна", component)
	}
	p := BalancerPreset{
		Name:      name,
		Component: component,
		Nodes:     append([]SegmentNode(nil), c.Nodes...),
		Bends:     append([]float64(nil), c.Bends...),
		Recovery:  presetRecoveryPtr(component, balancerCurveStore.scalars[component]),
		UpdatedAt: time.Now().UTC(),
	}
	upsertPreset(p)
	balancerPresetState.active[component] = name
	if err := writePresetsFileLocked(); err != nil {
		log.Printf("balancer presets: запись файла: %v (пресет сессионный)", err)
	}
	return p, nil
}

// ApplyPreset — «Применить пресет» (§6 POST apply): SetCurve из пресета +
// active[component] = name + файл. Не найден → ErrPresetNotFound (404).
// Невалидные данные (файл правили руками) → error (422), store не меняется.
func ApplyPreset(component, name string) ([]SegmentNode, []float64, error) {
	if !ValidComponent(component) {
		return nil, nil, fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	if err := validatePresetName(name); err != nil {
		return nil, nil, err
	}

	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()

	p := findPreset(component, name)
	if p == nil {
		return nil, nil, ErrPresetNotFound
	}
	if err := validateCurve(component, ComponentCurve{Nodes: p.Nodes, Bends: p.Bends}); err != nil {
		return nil, nil, fmt.Errorf("пресет %q/%q невалиден: %w", name, component, err)
	}
	if p.Recovery != nil && *p.Recovery < 0 {
		return nil, nil, fmt.Errorf("пресет %q/%q невалиден: recovery < 0 (%v)", name, component, *p.Recovery)
	}
	balancerCurveStore.curves[component] = cloneCurve(&ComponentCurve{Nodes: p.Nodes, Bends: p.Bends})
	if v, ok := presetRecovery(component, p); ok {
		balancerCurveStore.scalars[component] = v
	}
	balancerPresetState.active[component] = name
	if err := writePresetsFileLocked(); err != nil {
		log.Printf("balancer presets: запись файла: %v (пресет сессионный)", err)
	}
	return append([]SegmentNode(nil), p.Nodes...), append([]float64(nil), p.Bends...), nil
}

// DeletePreset — удалить пресет (§6 DELETE): default → error (422);
// не найден → ErrPresetNotFound (404). Текущая кривая в store НЕ меняется
// (удаление пресета не сбрасывает активную кривую).
func DeletePreset(component, name string) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}
	if name == "default" {
		return fmt.Errorf("default — заводской пресет; используйте \"Вернуть заводской\" (reset-default)")
	}
	if err := validatePresetName(name); err != nil {
		return err
	}

	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()

	idx := -1
	for i := range balancerPresetState.presets {
		if balancerPresetState.presets[i].Component == component && balancerPresetState.presets[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrPresetNotFound
	}
	balancerPresetState.presets = append(balancerPresetState.presets[:idx], balancerPresetState.presets[idx+1:]...)
	if err := writePresetsFileLocked(); err != nil {
		log.Printf("balancer presets: запись файла: %v (пресет сессионный)", err)
	}
	return nil
}

// ResetDefaultPreset — «Вернуть заводской» (§6): кодовые дефолты в store +
// пресет default перезаписан кодовыми узлами + active = default + файл.
func ResetDefaultPreset(component string) ([]SegmentNode, []float64, error) {
	if !ValidComponent(component) {
		return nil, nil, fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation/hunger)", component)
	}

	balancerCurveStore.mu.Lock()
	defer balancerCurveStore.mu.Unlock()

	d := defaultCurve(component)
	balancerCurveStore.curves[component] = cloneCurve(d)
	if v, ok := presetRecovery(component, nil); ok {
		balancerCurveStore.scalars[component] = v
	}
	upsertPreset(BalancerPreset{
		Name:      "default",
		Component: component,
		Nodes:     append([]SegmentNode(nil), d.Nodes...),
		Bends:     append([]float64(nil), d.Bends...),
		Recovery:  presetRecoveryPtr(component, defaultComponentScalar(component)),
		UpdatedAt: time.Now().UTC(),
	})
	balancerPresetState.active[component] = "default"
	if err := writePresetsFileLocked(); err != nil {
		log.Printf("balancer presets: запись файла: %v (пресет сессионный)", err)
	}
	return append([]SegmentNode(nil), d.Nodes...), append([]float64(nil), d.Bends...), nil
}

// findPreset — поиск пресета по (component, name) БЕЗ блокировки (все
// вызовы — под balancerStore.mu).
func findPreset(component, name string) *BalancerPreset {
	for i := range balancerPresetState.presets {
		p := &balancerPresetState.presets[i]
		if p.Component == component && p.Name == name {
			return p
		}
	}
	return nil
}

// upsertPreset — вставка или перезапись пресета в списке (под Lock).
func upsertPreset(p BalancerPreset) {
	for i := range balancerPresetState.presets {
		pp := &balancerPresetState.presets[i]
		if pp.Component == p.Component && pp.Name == p.Name {
			*pp = p
			return
		}
	}
	balancerPresetState.presets = append(balancerPresetState.presets, p)
}

// clonePreset — глубокая копия пресета.
func clonePreset(p *BalancerPreset) BalancerPreset {
	out := *p
	out.Nodes = append([]SegmentNode(nil), p.Nodes...)
	out.Bends = append([]float64(nil), p.Bends...)
	if p.Recovery != nil {
		v := *p.Recovery
		out.Recovery = &v
	}
	return out
}

// clonePresets — глубокая копия списка пресетов.
func clonePresets(ps []BalancerPreset) []BalancerPreset {
	out := make([]BalancerPreset, len(ps))
	for i := range ps {
		out[i] = clonePreset(&ps[i])
	}
	return out
}

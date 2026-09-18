// internal/resource/real.go — каталог 111 реальных веществ (оценка ёмкости
// модели осей, идея 2026-09-18 §4а/§4б): 10 осей 0–100 + T_melt/T_boil (K),
// семейство (химическая группа) и категория (6, categories.go). Read-only
// каталог-витрина для админки — в генерацию не входит, базовый слой 94a
// (layer.go) не трогает.
package resource

// RealResource — реальное вещество каталога-витрины: 10 осей 0–100,
// T_melt/T_boil (K), семейство (химическая группа) и категория.
// Поля осей — те же, что у Resource (layer.go).
type RealResource struct {
	ID       string // id-заглушка (транслит)
	Name     string // русское имя
	Family   string // семейство (химическая группа)
	Category string // одна из 6 категорий (categories.go)

	// 10 осей свойств 0–100 (§9.1.2).
	Hardness         float64 // твёрдость
	Elasticity       float64 // эластичность
	Conductivity     float64 // проводимость
	Density          float64 // плотность
	EnergyDensity    float64 // энергоёмкость
	Biocompatibility float64 // биосовместимость
	Radioactivity    float64 // радиоактивность
	Toxicity         float64 // токсичность
	Flammability     float64 // горючесть
	ChemicalActivity float64 // химическая активность

	TMelt float64 // температура плавления, K
	TBoil float64 // температура кипения, K
}

// Sublimating — сублимирующее вещество (T_boil ≤ T_melt, жидкой фазы нет).
// Выводится из чисел, не хранится (как Resource.Sublimating, layer.go).
func (r *RealResource) Sublimating() bool {
	return r.TBoil <= r.TMelt
}

// realRes — компактный конструктор записи каталога: 111 строк данных вместо
// ~2200 строк полных литералов (поля те же, что у RealResource; порядок
// осей — H Эл Пр Пл Эн Би Ра То Го Ха, как в таблице оценки §4б).
func realRes(id, name, family, category string, h, e, c, d, en, b, ra, to, fl, ca, tm, tb float64) *RealResource {
	return &RealResource{
		ID: id, Name: name, Family: family, Category: category,
		Hardness: h, Elasticity: e, Conductivity: c, Density: d,
		EnergyDensity: en, Biocompatibility: b, Radioactivity: ra,
		Toxicity: to, Flammability: fl, ChemicalActivity: ca,
		TMelt: tm, TBoil: tb,
	}
}

// Семейства — химические кластеры из оценки ёмкости (идея §4а/§4б).
const (
	famMetals       = "Металлы"
	famHeavyMetals  = "Тяжёлые металлы"
	famAlkaliMetals = "Лёгкие/щелочные металлы"
	famRadioMetals  = "Радиоактивные металлы"
	famRareEarth    = "Редкоземельные"
	famNonmetals    = "Неметаллы"
	famOxidesRocks  = "Оксиды и породы"
	famOres         = "Руды"
	famSalts        = "Соли"
	famHydrocarbonG = "Углеводородные газы"
	famOilProducts  = "Нефтепродукты"
	famCoals        = "Угли"
	famBiomaterials = "Биоматериалы"
	famAlcohols     = "Спирты и растворители"
	famAcids        = "Кислоты"
	famWaterSols    = "Вода и растворы"
	famNobleGases   = "Благородные газы"
	famActiveGases  = "Активные газы"
	famAtmoGases    = "Атмосферные газы"
	famIsotopes     = "Изотопы"
	famNuclear      = "Ядерные материалы"
	famSpecial      = "Спецматериалы"
)

// realCatalog — каталог 111 реальных веществ (таблица @designer, идея §4б).
// Read-only после инициализации (паттерн layerCatalog). Клампы T: He/He-3
// → 10 K (T_boil < 10 K вне диапазона модели), WC → 6000 K (T_boil > 6000).
// Смеси и разлагающиеся материалы (топлива, угли, породы, биоматериалы)
// оцифрованы по игровой калибровке — T оценочные (каталог-витрина).
var realCatalog = []*RealResource{
	// --- Минералы и металлы (41) ---
	realRes("zhelezo", "Железо Fe", famMetals, CategoryMineral, 50, 45, 75, 80, 10, 10, 5, 15, 5, 30, 1811, 3134),
	realRes("alyuminij", "Алюминий Al", famMetals, CategoryMineral, 28, 40, 85, 65, 15, 15, 5, 20, 20, 40, 933, 2792),
	realRes("titan", "Титан Ti", famMetals, CategoryMineral, 60, 40, 35, 70, 10, 10, 5, 15, 25, 25, 1941, 3560),
	realRes("med", "Медь Cu", famMetals, CategoryMineral, 30, 40, 95, 80, 10, 10, 5, 15, 5, 25, 1358, 2835),
	realRes("serebro", "Серебро Ag", famMetals, CategoryMineral, 25, 40, 100, 85, 10, 10, 5, 15, 5, 25, 1235, 2435),
	realRes("zoloto", "Золото Au", famMetals, CategoryMineral, 25, 40, 90, 95, 10, 10, 5, 15, 5, 20, 1337, 3243),
	realRes("svinets", "Свинец Pb", famHeavyMetals, CategoryMineral, 15, 25, 60, 85, 5, 5, 10, 80, 5, 30, 601, 2022),
	realRes("rtut", "Ртуть Hg", famHeavyMetals, CategoryMineral, 2, 10, 70, 90, 5, 5, 8, 95, 5, 35, 234, 630),
	realRes("olovo", "Олово Sn", famMetals, CategoryMineral, 15, 35, 65, 75, 5, 10, 5, 25, 5, 25, 505, 2875),
	realRes("volfram", "Вольфрам W", famMetals, CategoryMineral, 75, 20, 60, 95, 10, 10, 5, 15, 5, 20, 3695, 5828),
	realRes("tsink", "Цинк Zn", famMetals, CategoryMineral, 25, 35, 70, 75, 10, 10, 5, 20, 10, 30, 693, 1180),
	realRes("nikel", "Никель Ni", famMetals, CategoryMineral, 40, 40, 65, 80, 10, 10, 5, 20, 5, 25, 1728, 3186),
	realRes("khrom", "Хром Cr", famMetals, CategoryMineral, 85, 20, 65, 75, 10, 10, 5, 20, 10, 25, 2180, 2944),
	realRes("kobalt", "Кобальт Co", famMetals, CategoryMineral, 50, 40, 60, 80, 10, 10, 5, 20, 5, 25, 1768, 3200),
	realRes("litij", "Литий Li", famAlkaliMetals, CategoryMineral, 5, 30, 35, 20, 30, 10, 5, 30, 40, 80, 454, 1615),
	realRes("berillij", "Бериллий Be", famAlkaliMetals, CategoryMineral, 55, 25, 65, 40, 15, 10, 5, 35, 10, 35, 1560, 2742),
	realRes("magnij", "Магний Mg", famAlkaliMetals, CategoryMineral, 25, 30, 60, 40, 15, 10, 5, 20, 30, 40, 923, 1363),
	realRes("kaltsij", "Кальций Ca", famAlkaliMetals, CategoryMineral, 15, 30, 65, 35, 15, 10, 5, 25, 40, 70, 1115, 1757),
	realRes("natrij", "Натрий Na", famAlkaliMetals, CategoryMineral, 5, 15, 80, 25, 20, 5, 5, 40, 70, 100, 371, 1156),
	realRes("kalij", "Калий K", famAlkaliMetals, CategoryMineral, 5, 20, 55, 25, 20, 5, 5, 35, 70, 100, 337, 1032),
	realRes("uran", "Уран U", famRadioMetals, CategoryRare, 60, 30, 30, 95, 80, 5, 80, 70, 40, 60, 1405, 4404),
	realRes("plutonij", "Плутоний Pu", famRadioMetals, CategoryRare, 40, 25, 25, 95, 75, 5, 95, 85, 30, 65, 913, 3505),
	realRes("neodim", "Неодим Nd", famRareEarth, CategoryRare, 45, 30, 35, 75, 10, 10, 15, 30, 10, 35, 1297, 3347),
	realRes("tserij", "Церий Ce", famRareEarth, CategoryRare, 25, 30, 30, 75, 10, 10, 10, 25, 15, 45, 1071, 3697),
	realRes("kremnij", "Кремний Si", famNonmetals, CategoryMineral, 63, 20, 30, 55, 10, 5, 8, 25, 5, 35, 1687, 3538),
	realRes("sera", "Сера S", famNonmetals, CategoryMineral, 25, 15, 5, 55, 15, 10, 5, 65, 55, 55, 388, 718),
	realRes("grafit", "Графит C", famNonmetals, CategoryMineral, 15, 10, 40, 50, 30, 15, 3, 10, 15, 10, 3800, 3900), // сублимирующий
	realRes("almaz", "Алмаз C", famNonmetals, CategoryMineral, 100, 5, 5, 60, 5, 15, 3, 5, 5, 5, 4000, 4500),     // сублимирующий
	realRes("steklo_kvartsevoe", "Стекло кварцевое", famOxidesRocks, CategoryMineral, 55, 5, 5, 55, 5, 15, 5, 5, 5, 5, 1400, 2500),
	realRes("sol_nacl", "Соль NaCl", famSalts, CategoryMineral, 25, 10, 10, 60, 10, 15, 8, 20, 5, 45, 1074, 1738),
	realRes("kvarts", "Кварц SiO₂", famOxidesRocks, CategoryMineral, 70, 5, 5, 55, 5, 15, 5, 5, 5, 5, 1986, 2503),
	realRes("asbest", "Асбест", famOxidesRocks, CategoryMineral, 50, 30, 5, 45, 5, 10, 10, 60, 5, 15, 1300, 2000),
	realRes("glina", "Глина", famOxidesRocks, CategoryMineral, 15, 20, 5, 35, 5, 10, 10, 15, 5, 10, 1400, 2000),
	realRes("izvestnyak", "Известняк CaCO₃", famOxidesRocks, CategoryMineral, 30, 10, 5, 60, 5, 10, 10, 15, 5, 15, 1100, 1800),
	realRes("gips", "Гипс", famSalts, CategoryMineral, 20, 10, 5, 50, 5, 10, 5, 15, 5, 15, 400, 1500),
	realRes("granit", "Гранит", famOxidesRocks, CategoryMineral, 65, 10, 5, 60, 5, 10, 10, 20, 5, 10, 1500, 2500),
	realRes("bazalt", "Базальт", famOxidesRocks, CategoryMineral, 60, 10, 5, 65, 5, 10, 10, 25, 5, 10, 1400, 2500),
	realRes("gematit", "Гематит Fe₂O₃", famOres, CategoryMineral, 60, 10, 5, 70, 5, 10, 10, 20, 5, 15, 1838, 2500),
	realRes("boksit", "Боксит", famOres, CategoryMineral, 50, 10, 5, 55, 5, 10, 10, 15, 5, 15, 1300, 2300),
	realRes("khalkopirit", "Халькопирит CuFeS₂", famOres, CategoryMineral, 35, 10, 30, 65, 10, 10, 10, 30, 5, 25, 1100, 1800),
	realRes("karbid_volframa", "Карбид вольфрама WC", famSpecial, CategoryRare, 90, 15, 55, 80, 10, 10, 5, 15, 5, 20, 3058, 6000), // T_boil кламп к 6000

	// --- Органика (21) ---
	realRes("neft_syraya", "Нефть сырая", famOilProducts, CategoryFuel, 2, 30, 5, 30, 55, 10, 5, 50, 65, 40, 250, 600),
	realRes("ugol_kamennyj", "Уголь каменный", famCoals, CategoryFuel, 20, 10, 20, 40, 45, 10, 10, 35, 50, 30, 600, 800),
	realRes("ugol_buryj", "Уголь бурый", famCoals, CategoryFuel, 15, 10, 15, 35, 35, 10, 10, 30, 45, 25, 550, 750),
	realRes("antratsit", "Антрацит", famCoals, CategoryFuel, 25, 10, 25, 45, 50, 10, 10, 30, 45, 25, 700, 900),
	realRes("torf", "Торф", famCoals, CategoryFuel, 10, 15, 5, 25, 25, 10, 8, 25, 40, 20, 500, 700),
	realRes("prirodnyj_gaz", "Природный газ", famHydrocarbonG, CategoryGas, 2, 5, 10, 15, 60, 8, 2, 30, 62, 40, 90, 110),
	realRes("drevesina", "Древесина/целлюлоза", famBiomaterials, CategoryOrganic, 15, 50, 5, 25, 35, 55, 10, 10, 50, 20, 500, 700),
	realRes("sakharoza", "Сахароза", famBiomaterials, CategoryOrganic, 25, 15, 5, 35, 35, 55, 3, 5, 40, 20, 460, 600),
	realRes("krakhmal", "Крахмал", famBiomaterials, CategoryOrganic, 10, 20, 5, 30, 30, 50, 3, 5, 40, 15, 450, 550),
	realRes("belki", "Белки", famBiomaterials, CategoryOrganic, 10, 30, 5, 30, 35, 55, 3, 10, 40, 20, 450, 550),
	realRes("zhiry_masla", "Жиры/масла", famBiomaterials, CategoryOrganic, 5, 25, 5, 25, 55, 30, 3, 15, 55, 25, 280, 550),
	realRes("etanol", "Этанол", famAlcohols, CategoryOrganic, 2, 5, 5, 25, 50, 30, 3, 45, 65, 35, 159, 351),
	realRes("atseton", "Ацетон", famAlcohols, CategoryOrganic, 2, 5, 5, 25, 50, 15, 3, 35, 70, 30, 178, 329),
	realRes("metanol", "Метанол", famAlcohols, CategoryOrganic, 2, 5, 5, 25, 40, 25, 3, 60, 60, 35, 176, 338),
	realRes("uksusnaya_kislota", "Уксусная кислота", famAcids, CategoryWater, 2, 5, 5, 25, 30, 25, 3, 40, 55, 45, 290, 391),
	realRes("biomassa", "Биомасса", famBiomaterials, CategoryOrganic, 10, 40, 5, 25, 30, 60, 5, 15, 45, 25, 400, 600),
	realRes("gumus", "Гумус", famBiomaterials, CategoryOrganic, 10, 30, 5, 30, 20, 50, 5, 20, 40, 20, 400, 600),
	realRes("lateks_kauchuk", "Латекс/каучук", famBiomaterials, CategoryOrganic, 5, 90, 5, 20, 40, 40, 3, 15, 55, 15, 400, 600),
	realRes("sherst_kozha", "Шерсть/кожа", famBiomaterials, CategoryOrganic, 15, 60, 5, 20, 30, 50, 3, 10, 45, 15, 450, 600),
	realRes("smoly", "Смолы", famBiomaterials, CategoryOrganic, 20, 40, 5, 25, 45, 25, 3, 25, 55, 30, 400, 600),
	realRes("polietilen", "Полиэтилен", famBiomaterials, CategoryOrganic, 15, 65, 5, 20, 45, 25, 3, 15, 40, 10, 400, 700),

	// --- Топливо и энергия (11) ---
	realRes("metan", "Метан CH₄", famHydrocarbonG, CategoryGas, 2, 5, 10, 15, 61, 8, 2, 30, 63, 40, 91, 112),
	realRes("etan", "Этан C₂H₆", famHydrocarbonG, CategoryGas, 2, 5, 10, 20, 58, 8, 2, 25, 65, 35, 90, 185),
	realRes("propan", "Пропан C₃H₈", famHydrocarbonG, CategoryGas, 2, 5, 10, 20, 55, 8, 2, 25, 65, 35, 85, 231),
	realRes("butan", "Бутан C₄H₁₀", famHydrocarbonG, CategoryGas, 2, 5, 10, 20, 52, 8, 2, 25, 65, 35, 135, 273),
	realRes("vodorod", "Водород H₂", famActiveGases, CategoryGas, 2, 5, 15, 8, 58, 5, 5, 20, 62, 30, 14, 20),
	realRes("benzin", "Бензин", famOilProducts, CategoryFuel, 2, 20, 5, 30, 55, 10, 5, 45, 70, 35, 180, 400),
	realRes("kerosin", "Керосин", famOilProducts, CategoryFuel, 2, 20, 5, 30, 52, 10, 5, 40, 65, 30, 220, 500),
	realRes("dizel", "Дизель", famOilProducts, CategoryFuel, 2, 25, 5, 30, 50, 10, 5, 40, 65, 30, 250, 600),
	realRes("mazut", "Мазут", famOilProducts, CategoryFuel, 2, 25, 5, 30, 48, 10, 5, 40, 60, 30, 280, 650),
	realRes("atsetilen", "Ацетилен C₂H₂", famHydrocarbonG, CategoryGas, 2, 5, 10, 15, 50, 8, 3, 35, 75, 50, 192, 189), // сублимирующий
	realRes("uran_oksid", "Уран-оксид UO₂", famNuclear, CategoryRare, 55, 20, 25, 90, 75, 5, 75, 65, 35, 50, 3120, 3800),

	// --- Вода и жидкости (11) ---
	realRes("voda", "Вода H₂O", famWaterSols, CategoryWater, 5, 15, 10, 35, 8, 63, 3, 12, 5, 35, 273, 373),
	realRes("ammiak", "Аммиак NH₃", famWaterSols, CategoryWater, 5, 10, 25, 30, 25, 15, 5, 62, 45, 52, 195, 240),
	realRes("sernaya_kislota", "Серная кислота H₂SO₄", famAcids, CategoryWater, 2, 5, 5, 30, 10, 5, 3, 85, 5, 90, 283, 610),
	realRes("solyanaya_kislota", "Соляная кислота HCl", famAcids, CategoryWater, 2, 5, 5, 30, 10, 5, 3, 80, 5, 85, 190, 360),
	realRes("azotnaya_kislota", "Азотная кислота HNO₃", famAcids, CategoryWater, 2, 5, 5, 30, 15, 5, 3, 85, 5, 95, 231, 356),
	realRes("rassol", "Рассол (раствор NaCl)", famWaterSols, CategoryWater, 5, 10, 10, 40, 8, 25, 5, 25, 5, 30, 255, 380),
	realRes("perekis_vodoroda", "Перекись водорода H₂O₂", famWaterSols, CategoryWater, 2, 5, 10, 35, 15, 10, 3, 50, 10, 75, 273, 423),
	realRes("gidrazin", "Гидразин N₂H₄", famWaterSols, CategoryWater, 2, 5, 5, 30, 45, 5, 5, 70, 60, 65, 275, 387),
	realRes("glitserin", "Глицерин", famAlcohols, CategoryOrganic, 2, 20, 5, 35, 30, 35, 3, 15, 40, 25, 291, 563),
	realRes("benzol", "Бензол C₆H₆", famAlcohols, CategoryOrganic, 2, 10, 5, 25, 55, 15, 3, 50, 70, 30, 279, 353),
	realRes("toluol", "Толуол C₇H₈", famAlcohols, CategoryOrganic, 2, 10, 5, 25, 52, 15, 3, 45, 70, 30, 178, 384),

	// --- Газы (20) ---
	realRes("azot", "Азот N₂", famAtmoGases, CategoryGas, 2, 5, 5, 15, 5, 10, 3, 10, 5, 5, 63, 77),
	realRes("kislorod", "Кислород O₂", famAtmoGases, CategoryGas, 2, 5, 5, 15, 10, 30, 3, 15, 5, 60, 54, 90),
	realRes("co2_suhoy_led", "CO₂ сухой лёд", famAtmoGases, CategoryGas, 15, 5, 5, 25, 10, 10, 3, 35, 5, 40, 196, 189), // сублимирующий
	realRes("co_ugarnyj_gaz", "CO угарный газ", famActiveGases, CategoryGas, 2, 5, 10, 15, 15, 5, 3, 70, 40, 40, 68, 81),
	realRes("gelij", "Гелий He", famNobleGases, CategoryGas, 2, 5, 10, 10, 5, 5, 3, 5, 5, 5, 10, 10), // кламп T к 10 K
	realRes("neon", "Неон Ne", famNobleGases, CategoryGas, 2, 5, 10, 10, 5, 5, 3, 5, 5, 5, 25, 27),
	realRes("argon", "Аргон Ar", famNobleGases, CategoryGas, 2, 5, 5, 15, 5, 5, 3, 5, 5, 5, 84, 87),
	realRes("kripton", "Криптон Kr", famNobleGases, CategoryGas, 2, 5, 5, 20, 5, 5, 3, 5, 5, 5, 116, 120),
	realRes("ksenon", "Ксенон Xe", famNobleGases, CategoryGas, 2, 5, 5, 25, 5, 5, 3, 5, 5, 5, 161, 165),
	realRes("khlor", "Хлор Cl₂", famActiveGases, CategoryGas, 2, 5, 5, 20, 5, 5, 5, 95, 5, 90, 172, 239),
	realRes("ftor", "Фтор F₂", famActiveGases, CategoryGas, 2, 5, 5, 20, 10, 5, 5, 100, 10, 100, 53, 85),
	realRes("serovodorod", "Сероводород H₂S", famActiveGases, CategoryGas, 2, 5, 10, 15, 20, 5, 3, 80, 60, 50, 187, 213),
	realRes("so2", "SO₂", famActiveGases, CategoryGas, 2, 5, 5, 20, 10, 5, 3, 70, 5, 60, 197, 263),
	realRes("no2", "NO₂", famActiveGases, CategoryGas, 2, 5, 5, 20, 15, 5, 3, 75, 5, 70, 262, 294),
	realRes("no", "NO", famActiveGases, CategoryGas, 2, 5, 5, 15, 10, 5, 3, 70, 5, 65, 110, 121),
	realRes("n2o", "N₂O", famActiveGases, CategoryGas, 2, 5, 10, 15, 15, 10, 3, 60, 5, 35, 182, 184),
	realRes("freon_r12", "Фреон R-12", famActiveGases, CategoryGas, 2, 5, 5, 25, 5, 5, 5, 40, 5, 15, 115, 243),
	realRes("sf6", "SF₆", famActiveGases, CategoryGas, 2, 5, 5, 30, 5, 5, 5, 30, 5, 10, 223, 337),
	realRes("silan", "Силан SiH₄", famActiveGases, CategoryGas, 2, 5, 10, 15, 40, 5, 5, 60, 70, 60, 88, 161),
	realRes("fosfin", "Фосфин PH₃", famActiveGases, CategoryGas, 2, 5, 10, 15, 35, 5, 5, 75, 70, 55, 140, 185),

	// --- Редкие и спецматериалы (7) ---
	realRes("gelij_3", "Гелий-3", famIsotopes, CategoryRare, 2, 5, 10, 10, 5, 5, 3, 5, 5, 5, 10, 10), // кламп T к 10 K
	realRes("dejterij", "Дейтерий D₂", famIsotopes, CategoryRare, 2, 5, 15, 10, 58, 5, 5, 20, 62, 30, 18, 23),
	realRes("tritij", "Тритий T₂", famIsotopes, CategoryRare, 2, 5, 15, 12, 58, 5, 8, 25, 62, 30, 21, 25),
	realRes("radioaktivnye_izotopy", "Радиоактивные изотопы", famNuclear, CategoryRare, 35, 10, 25, 50, 45, 8, 65, 60, 10, 40, 800, 1500),
	realRes("ybco", "YBCO-сверхпроводник", famSpecial, CategoryRare, 45, 15, 100, 70, 10, 5, 10, 60, 5, 30, 1200, 2000),
	realRes("gaas", "GaAs арсенид галлия", famSpecial, CategoryRare, 45, 10, 35, 65, 10, 5, 10, 60, 5, 25, 1511, 2500),
	realRes("titan_splav", "Титан-сплав Ti-6Al-4V", famSpecial, CategoryRare, 70, 50, 30, 70, 10, 10, 5, 15, 25, 20, 1900, 3600),
}

// RealCatalog — каталог реальных веществ (read-only).
func RealCatalog() []*RealResource {
	return realCatalog
}

// RealByID — вещество каталога по id (nil, если нет).
func RealByID(id string) *RealResource {
	for _, r := range realCatalog {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// RealFamilies — уникальные семейства каталога в порядке появления.
func RealFamilies() []string {
	seen := make(map[string]bool)
	fams := make([]string, 0, 22)
	for _, r := range realCatalog {
		if !seen[r.Family] {
			seen[r.Family] = true
			fams = append(fams, r.Family)
		}
	}
	return fams
}
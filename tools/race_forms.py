# -*- coding: utf-8 -*-
# race_forms.py — БОЛЬШОЙ словарь форм-архетипов (1000+ комбинаций).
# Собирается программно: форма x модификатор структуры x характер x деталь.
# Каждая комбинация — осмысленная неантропоморфная фраза.
# Категории: чтобы подбирать под семейство (крио -> лёд, газ -> вихрь и т.д.).

# --- базовые ФОРМЫ ---
SHAPES = [
    ("spire", ["crystal", "cold", "hot"]),
    ("cone", ["gas", "sulfur", "hot"]),
    ("ring", ["energy", "gas", "sphere"]),
    ("disc", ["sphere", "gas", "mech"]),
    ("helix", ["gas", "cold", "energy"]),
    ("sphere", ["sphere", "cold", "hot"]),
    ("column", ["crystal", "cold", "hot"]),
    ("lattice", ["crystal", "energy", "mech"]),
    ("star", ["energy", "crystal", "sulfur"]),
    ("blade", ["organic", "cold", "mech"]),
    ("fan", ["organic", "cold", "energy"]),
    ("web", ["organic", "gas", "energy"]),
    ("chain", ["organic", "cold", "sulfur", "mech"]),
    ("mound", ["sulfur", "hot", "cold"]),
    ("shell", ["organic", "cold", "sulfur"]),
    ("obelisk", ["crystal", "cold", "hot"]),
    ("torus", ["energy", "gas", "mech"]),
    ("pyramid", ["crystal", "sulfur", "hot"]),
    ("prism", ["crystal", "cold", "energy"]),
    ("knot", ["organic", "gas", "sulfur"]),
    ("burst", ["energy", "crystal", "hot"]),
    ("crown", ["organic", "energy", "cold"]),
    ("fountain", ["liquid", "hot", "energy"]),
    ("cradle", ["organic", "sulfur", "cold"]),
    ("scaffold", ["mech", "crystal", "energy"]),
    ("whorl", ["gas", "cold", "energy"]),
    ("cluster", ["crystal", "cold", "sulfur"]),
    ("sculpture", ["crystal", "cold", "hot", "sulfur"]),
]

# --- модификаторы СТРУКТУРЫ ---
STRUCT = [
    "clustered", "twisted", "layered", "branching", "hollow",
    "nested", "tangled", "faceted", "segmented", "radiating",
    "coiled", "pierced", "weathered", "crystalline", "interlocking",
    "stacked", "folded", "fused", "woven", "spiral-wrapped",
    "overlapping", "tiered", "concentric", "asymmetric", "zigzag",
]

# --- ХАРАКТЕР поверхности ---
CHARACTER = [
    "jagged", "smooth", "spiky", "rippled", "porous",
    "glassy", "frosted", "pitted", "polished", "craggy",
    "feathered", "thorned", "scalloped", "ridged", "veined",
    "blistered", "webbed", "molten", "chiseled", "luminous-edged",
]

# --- ДЕТАЛИ / элементы ---
PARTS = [
    "shards", "blades", "lobes", "filaments", "plates",
    "prisms", "spikes", "cubes", "rods", "petals",
    "needles", "scales", "tubes", "flakes", "ridges",
    "fingers", "ribs", "fronds", "spears", "chords",
    "beads", "ribbons", "spurs", "tendrils", "knuckles",
]

# --- антропоморфные формы (для галки «антропоморфный») ---
# гуманоиды ИЗ МАТЕРИАЛА расы, РАЗНЫЕ по строению (не только «лицо с чертами»)
ANTHRO_FORMS = [
    "humanoid face with angular features",
    "humanoid figure with a broad face",
    "humanoid head with sharp cheekbones",
    "humanoid bust with a stern face",
    "humanoid face with a high forehead",
    "humanoid figure with glowing eyes",
    "humanoid head with fin-like ears",
    "humanoid bust with a regal face",
    "humanoid face with a strong jaw",
    "humanoid figure with elegant features",
    "humanoid head with a noble face",
    "humanoid bust with almond-shaped eyes",
    "humanoid face with weathered features",
    "humanoid figure with a serene face",
    "humanoid head with a fierce face",
    "humanoid bust with a wise face",
    "humanoid face with luminous eyes",
    "humanoid figure with a gentle face",
    # --- разнообразие строения головы/фигуры (по основе расы) ---
    "humanoid head with two small curved horns",
    "humanoid figure with a high crest on the head",
    "humanoid bust with wide-set eyes and no nose",
    "humanoid head with a long pointed chin",
    "humanoid figure with a domed bald head",
    "humanoid bust with large round eyes",
    "humanoid head with a crown of spikes",
    "humanoid figure with a flat featureless face",
    "humanoid bust with a narrow elongated skull",
    "humanoid head with gill slits on the neck",
    "humanoid figure with a wide flat face",
    "humanoid bust with a single central eye",
    "humanoid head with three small eyes",
    "humanoid figure with a beak-like mouth",
    "humanoid bust with an elongated nose ridge",
    "humanoid head with fin-like ears and crest",
    "humanoid figure with a split lower jaw",
    "humanoid bust with a wide thick neck",
    "humanoid head with protruding brow ridges",
    "humanoid figure with small vestigial arms",
    "humanoid bust with a smooth featureless head",
    "humanoid head with eye stalks",
    "humanoid figure with a hunched posture",
    "humanoid bust with asymmetrical face",
    "humanoid head with a lantern-like glow",
]


# категории семейств -> разрешённые категории форм
CATEGORY_KEYS = {
    "F2": ["crystal", "cold", "gas", "organic", "sphere"],
    "F3": ["cold", "organic", "gas", "sphere"],
    "F4": ["sulfur", "hot", "organic", "mech", "crystal"],
    "F5": ["hot", "liquid", "sulfur", "mech", "energy", "crystal"],
    "F6": ["crystal", "hot", "mech", "energy"],
    "F7": ["gas", "energy", "mech", "sphere"],
    "F8": ["gas", "organic", "cold", "mech"],
    "F9": ["energy", "gas", "mech", "sphere", "crystal"],
}

# --- генерация 1000+ комбинаций ---
def _build_all():
    out = []
    for shape, cats in SHAPES:
        for struct in STRUCT:
            for char in CHARACTER:
                for part in PARTS:
                    # фраза: "a {struct} {char} {shape} of material, formed of {part}"
                    # НЕ «mineral» (тянет к кристаллам) — материал подставит промпт
                    phrase = f"a {struct} {char} {shape} of material, formed of {part}"
                    out.append((phrase, cats))
    return out

_FORMS_ALL = _build_all()


def forms_for(fam_id):
    """Формы, подходящие семейству (по категориям). Если пусто — все."""
    keys = CATEGORY_KEYS.get(fam_id, [])
    if not keys:
        return [f[0] for f in _FORMS_ALL]
    return [f[0] for f in _FORMS_ALL if any(k in f[1] for k in keys)]


if __name__ == "__main__":
    print("всего форм:", len(_FORMS_ALL))
    for f in ["F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9"]:
        print("%s: %d форм" % (f, len(forms_for(f))))
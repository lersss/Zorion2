// internal/handlers/studio_recipes_handlers.go
// HTTP-хендлеры рецептов /studio/api/recipes* и привязок рецептов к фабрикам
// /studio/api/producers/{id}/recipes* (спека
// 2026-09-21-рецепт-сущность-и-граф-фабрики §5). Вынесены из
// studio_handlers.go (файл > 300 строк, §4.9: имена файлов уникальны).
// Доступ — auth.AdminAuth (admin/skycomposer). Ошибки — {"error": "..."}:
// 400 невалидный вход/чужой выход · 404 не найдено · 409 дубликат/цикл/
// привязка есть.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Recipes — POST /studio/api/recipes {good_id, complexity?}: создать рецепт
// товара (штатно создаётся вместе с товаром; роут — для восстановления).
func (h *StudioHandlers) Recipes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		GoodID     int64 `json:"good_id"`
		Complexity *int  `json:"complexity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if body.GoodID <= 0 {
		studioErr(w, "good_id обязателен", http.StatusBadRequest)
		return
	}
	recipeID, err := h.repo.CreateRecipe(body.GoodID, body.Complexity)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusCreated, map[string]interface{}{"id": recipeID, "good_id": body.GoodID})
}

// RecipeByID — PUT /studio/api/recipes/{id} и под-пути
// (components, components/{pos}, components/{pos}/component,
// components/{pos}/allow_resource).
func (h *StudioHandlers) RecipeByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/studio/api/recipes/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	id, err := parseID(parts[0])
	if err != nil {
		studioErr(w, "не найдено", http.StatusNotFound)
		return
	}
	switch {
	case len(parts) == 1:
		h.recipeComplexity(w, r, id)
	case len(parts) == 2 && parts[1] == "components":
		h.recipeAddComponent(w, r, id)
	case len(parts) == 3 && parts[1] == "components":
		h.recipeComponent(w, r, id, parts[2])
	case len(parts) == 4 && parts[1] == "components" && parts[3] == "component":
		h.recipeClearComponent(w, r, id, parts[2])
	case len(parts) == 4 && parts[1] == "components" && parts[3] == "allow_resource":
		h.recipeComponentAllowResource(w, r, id, parts[2])
	default:
		studioErr(w, "не найдено", http.StatusNotFound)
	}
}

// recipeComplexity — PUT /studio/api/recipes/{id} {complexity: int|null}
// (null = вычисляемая по графу).
func (h *StudioHandlers) recipeComplexity(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPut {
		studioErr(w, "только PUT", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Complexity *int `json:"complexity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetRecipeComplexity(id, body.Complexity); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "complexity": body.Complexity})
}

// recipeAddComponent — POST /studio/api/recipes/{id}/components: добавить
// пустой компонент (pos = max+1, quantity 1).
func (h *StudioHandlers) recipeAddComponent(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	if err := h.repo.AddRecipeComponent(id); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// recipeComponent — PUT/DELETE /studio/api/recipes/{id}/components/{pos}:
// поставить составляющую/количество (цикл → 409) или удалить компонент.
func (h *StudioHandlers) recipeComponent(w http.ResponseWriter, r *http.Request, id int64, posStr string) {
	pos, err := parseID(posStr)
	if err != nil {
		studioErr(w, "невалидный номер компонента", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			GoodID   *int64 `json:"good_id"`
			Quantity *int   `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			studioErr(w, "невалидный JSON", http.StatusBadRequest)
			return
		}
		var compID int64
		if body.GoodID != nil {
			compID = *body.GoodID
		}
		if err := h.repo.PutRecipeComponent(id, int(pos), compID, body.Quantity); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
	case http.MethodDelete:
		if err := h.repo.DeleteRecipeComponent(id, int(pos)); err != nil {
			writeCatalogErr(w, err)
			return
		}
		studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
	default:
		studioErr(w, "только PUT/DELETE", http.StatusMethodNotAllowed)
	}
}

// recipeClearComponent — DELETE /studio/api/recipes/{id}/components/{pos}/component
// (товар остаётся в реестре).
func (h *StudioHandlers) recipeClearComponent(w http.ResponseWriter, r *http.Request, id int64, posStr string) {
	if r.Method != http.MethodDelete {
		studioErr(w, "только DELETE", http.StatusMethodNotAllowed)
		return
	}
	pos, err := parseID(posStr)
	if err != nil {
		studioErr(w, "невалидный номер компонента", http.StatusBadRequest)
		return
	}
	if err := h.repo.ClearRecipeComponent(id, int(pos)); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
}

// recipeComponentAllowResource — PUT
// /studio/api/recipes/{id}/components/{pos}/allow_resource {allow_resource}:
// галка «разрешить ресурс в этом компоненте (для ИИ)».
func (h *StudioHandlers) recipeComponentAllowResource(w http.ResponseWriter, r *http.Request, id int64, posStr string) {
	if r.Method != http.MethodPut {
		studioErr(w, "только PUT", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AllowResource bool `json:"allow_resource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	pos, err := parseID(posStr)
	if err != nil {
		studioErr(w, "невалидный номер компонента", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetRecipeComponentAllowResource(id, int(pos), body.AllowResource); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"id": id, "pos": pos})
}

// --- привязки рецептов к фабрикам (producer_recipes, спека §5) ---

// producerBindRecipe — POST /studio/api/producers/{id}/recipes {recipe_id}:
// привязать рецепт к конкретной фабрике (400: не конкретная фабрика / чужой
// выход; 409: повтор).
func (h *StudioHandlers) producerBindRecipe(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		RecipeID int64 `json:"recipe_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		studioErr(w, "невалидный JSON", http.StatusBadRequest)
		return
	}
	if body.RecipeID <= 0 {
		studioErr(w, "recipe_id обязателен", http.StatusBadRequest)
		return
	}
	if err := h.repo.BindRecipe(id, body.RecipeID); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"producer_type_id": id, "recipe_id": body.RecipeID})
}

// producerUnbindRecipe — DELETE /studio/api/producers/{id}/recipes/{recipe_id}.
func (h *StudioHandlers) producerUnbindRecipe(w http.ResponseWriter, r *http.Request, id, recipeID int64) {
	if r.Method != http.MethodDelete {
		studioErr(w, "только DELETE", http.StatusMethodNotAllowed)
		return
	}
	if err := h.repo.UnbindRecipe(id, recipeID); err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]interface{}{"producer_type_id": id, "recipe_id": recipeID})
}

// producerCopyUniversal — POST
// /studio/api/producers/{id}/recipes/copy-universal: скопировать набор
// рецептов в конкретную фабрику из универсальных фабрик её категории;
// 200 {added, skipped}; идемпотентно; пустой источник — {0,0}; 400 — не
// конкретная фабрика / нет категории / категория ресурсная.
func (h *StudioHandlers) producerCopyUniversal(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		studioErr(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	added, skipped, err := h.repo.CopyUniversalRecipes(id)
	if err != nil {
		writeCatalogErr(w, err)
		return
	}
	studioJSON(w, http.StatusOK, map[string]int{"added": added, "skipped": skipped})
}

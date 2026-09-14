// cmd/server/bootstrap.go
package main

import (
	"database/sql"
	"log"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// bootstrapSkycomposer создаёт первую учётку с ролью skycomposer
// (спека 99.2.14 §5). Срабатывает только пока в БД нет ни одного skycomposer:
//  1. skycomposer'ов ≥ 1 — ничего не делаем;
//  2. их 0 и заданы SKYCOMPOSER_BOOTSTRAP_USERNAME/PASSWORD — создаём учётку,
//     а если username занят — повышаем существующую;
//  3. их 0 и env не заданы — log warning, сервер продолжает работу.
//
// actingUserID = "" в UpdateRole: id пользователя — UUID, пустой строки не бывает,
// а проверка «последний skycomposer» не сработает (существующая учётка — player).
func bootstrapSkycomposer(db *sql.DB, username, password string) {
	userRepo := repository.NewUserRepository(db)

	count, err := userRepo.CountByRole(models.RoleSkycomposer)
	if err != nil {
		log.Fatalf("❌ Не удалось посчитать skycomposer'ов: %v", err)
	}
	if count >= 1 {
		return
	}

	if username == "" || password == "" {
		log.Println("⚠️ Skycomposer'ов нет и SKYCOMPOSER_BOOTSTRAP_USERNAME/PASSWORD не заданы — " +
			"админка недоступна, пока не создан Skycomposer")
		return
	}

	existing, err := userRepo.GetByUsername(username)
	if err != nil {
		log.Fatalf("❌ Bootstrap skycomposer: ошибка проверки username: %v", err)
	}
	if existing != nil {
		if err := userRepo.UpdateRole(existing.ID, models.RoleSkycomposer, ""); err != nil {
			log.Fatalf("❌ Bootstrap skycomposer: не удалось повысить %s: %v", username, err)
		}
		log.Printf("✅ Skycomposer: учётка %s повышена до skycomposer", username)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("❌ Bootstrap skycomposer: не удалось хэшировать пароль: %v", err)
	}
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hashed),
		ShipIcon:     "ship_strela.svg",
		Role:         models.RoleSkycomposer,
	}
	if err := userRepo.Create(user); err != nil {
		log.Fatalf("❌ Bootstrap skycomposer: не удалось создать учётку: %v", err)
	}
	log.Printf("✅ Skycomposer создан: %s", username)
}

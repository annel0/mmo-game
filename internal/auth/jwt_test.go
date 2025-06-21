package auth

import (
	"strings"
	"testing"
	"time"
)

// TestGenerateJWT тестирует создание JWT токена
func TestGenerateJWT(t *testing.T) {
	user := &User{
		ID:           1,
		Username:     "testuser",
		PasswordHash: "hashedpassword",
		IsAdmin:      false,
		CreatedAt:    time.Now(),
		LastLogin:    time.Now(),
	}

	token, err := GenerateJWT(user)
	if err != nil {
		t.Fatalf("Ошибка генерации JWT: %v", err)
	}

	if token == "" {
		t.Fatal("Пустой токен")
	}

	// Проверяем, что токен содержит точки (разделители частей JWT)
	if strings.Count(token, ".") != 2 {
		t.Errorf("Неверный формат JWT токена: %s", token)
	}
}

// TestValidateJWT тестирует валидацию JWT токена
func TestValidateJWT(t *testing.T) {
	user := &User{
		ID:           42,
		Username:     "validuser",
		PasswordHash: "hashedpassword",
		IsAdmin:      true,
		CreatedAt:    time.Now(),
		LastLogin:    time.Now(),
	}

	// Генерируем токен
	token, err := GenerateJWT(user)
	if err != nil {
		t.Fatalf("Ошибка генерации JWT: %v", err)
	}

	// Валидируем токен
	playerID, isValid, isAdmin := ValidateJWT(token)

	if !isValid {
		t.Error("Валидный токен определен как недействительный")
	}

	if playerID != user.ID {
		t.Errorf("Неверный playerID: ожидался %d, получен %d", user.ID, playerID)
	}

	if isAdmin != user.IsAdmin {
		t.Errorf("Неверный флаг администратора: ожидался %v, получен %v", user.IsAdmin, isAdmin)
	}
}

// TestValidateInvalidJWT тестирует валидацию недействительного JWT
func TestValidateInvalidJWT(t *testing.T) {
	// Тестируем различные случаи недействительных токенов
	testCases := []string{
		"invalid.token.here",
		"",
		"not.a.jwt",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature",
	}

	for _, invalidToken := range testCases {
		playerID, isValid, isAdmin := ValidateJWT(invalidToken)

		if isValid {
			t.Errorf("Недействительный токен '%s' прошел валидацию", invalidToken)
		}

		if playerID != 0 {
			t.Errorf("PlayerID должен быть 0 для недействительного токена, получен %d", playerID)
		}

		if isAdmin {
			t.Errorf("isAdmin должен быть false для недействительного токена")
		}
	}
}

// TestGenerateSecureSecret тестирует генерацию секретного ключа
func TestGenerateSecureSecret(t *testing.T) {
	secret1, err1 := GenerateSecureSecret()
	if err1 != nil {
		t.Fatalf("Ошибка генерации первого секрета: %v", err1)
	}

	secret2, err2 := GenerateSecureSecret()
	if err2 != nil {
		t.Fatalf("Ошибка генерации второго секрета: %v", err2)
	}

	// Проверяем, что секреты разные
	if secret1 == secret2 {
		t.Error("Два последовательных вызова GenerateSecureSecret вернули одинаковый результат")
	}

	// Проверяем, что секрет не пустой
	if secret1 == "" || secret2 == "" {
		t.Error("GenerateSecureSecret вернул пустой секрет")
	}

	// Проверяем минимальную длину (base64 от 32 байт = ~44 символа)
	if len(secret1) < 40 || len(secret2) < 40 {
		t.Error("Секрет слишком короткий")
	}
}

// TestSetJWTSecret тестирует установку пользовательского секретного ключа
func TestSetJWTSecret(t *testing.T) {
	// Генерируем действительный секрет
	validSecret, err := GenerateSecureSecret()
	if err != nil {
		t.Fatalf("Ошибка генерации валидного секрета: %v", err)
	}

	// Тестируем установку действительного секрета
	err = SetJWTSecret(validSecret)
	if err != nil {
		t.Errorf("Ошибка установки валидного секрета: %v", err)
	}

	// Тестируем недействительные секреты
	invalidSecrets := []string{
		"too-short",
		"invalid-base64-@#$%",
		"",
	}

	for _, invalidSecret := range invalidSecrets {
		err = SetJWTSecret(invalidSecret)
		if err == nil {
			t.Errorf("Недействительный секрет '%s' был принят", invalidSecret)
		}
	}
}

// TestJWTTokenLifecycle тестирует полный жизненный цикл токена
func TestJWTTokenLifecycle(t *testing.T) {
	// Создаем пользователя
	user := &User{
		ID:           123,
		Username:     "lifecycle_user",
		PasswordHash: "hashedpassword",
		IsAdmin:      false,
		CreatedAt:    time.Now(),
		LastLogin:    time.Now(),
	}

	// Генерируем токен
	token, err := GenerateJWT(user)
	if err != nil {
		t.Fatalf("Ошибка генерации токена: %v", err)
	}

	// Сразу проверяем токен
	playerID, isValid, isAdmin := ValidateJWT(token)
	if !isValid {
		t.Error("Свежеcозданный токен недействителен")
	}

	if playerID != user.ID {
		t.Errorf("Неверный playerID: ожидался %d, получен %d", user.ID, playerID)
	}

	if isAdmin != user.IsAdmin {
		t.Errorf("Неверный статус администратора: ожидался %v, получен %v", user.IsAdmin, isAdmin)
	}

	// Проверяем, что токен работает с разными пользователями
	adminUser := &User{
		ID:           456,
		Username:     "admin_user",
		PasswordHash: "adminhashedpassword",
		IsAdmin:      true,
		CreatedAt:    time.Now(),
		LastLogin:    time.Now(),
	}

	adminToken, err := GenerateJWT(adminUser)
	if err != nil {
		t.Fatalf("Ошибка генерации админского токена: %v", err)
	}

	adminPlayerID, adminIsValid, adminIsAdmin := ValidateJWT(adminToken)
	if !adminIsValid {
		t.Error("Админский токен недействителен")
	}

	if adminPlayerID != adminUser.ID {
		t.Errorf("Неверный adminPlayerID: ожидался %d, получен %d", adminUser.ID, adminPlayerID)
	}

	if !adminIsAdmin {
		t.Error("Админский флаг должен быть true для админского пользователя")
	}

	// Проверяем, что токены разные
	if token == adminToken {
		t.Error("Токены для разных пользователей одинаковые")
	}
}

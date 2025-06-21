package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT secret key - in production should be loaded from environment variable
var jwtSecret []byte

func init() {
	// Generate a secure random secret key
	jwtSecret = make([]byte, 32)
	if _, err := rand.Read(jwtSecret); err != nil {
		// Fallback to a hardcoded key only for development
		jwtSecret = []byte("development-secret-key-change-in-production")
	}
}

// Claims представляет структуру данных JWT токена.
// Содержит идентификацию пользователя и его права доступа.
type Claims struct {
	PlayerID             uint64 `json:"player_id"` // Уникальный идентификатор игрока
	Username             string `json:"username"`  // Имя пользователя
	IsAdmin              bool   `json:"is_admin"`  // Флаг администраторских прав
	jwt.RegisteredClaims        // Стандартные JWT claims
}

// GenerateJWT создает безопасный JWT токен для указанного пользователя.
// Токен содержит идентификатор игрока, имя пользователя и права доступа.
// Срок действия токена составляет 24 часа.
//
// Параметры:
//
//	user - пользователь, для которого создается токен
//
// Возвращает:
//
//	string - подписанный JWT токен
//	error - ошибка при создании или подписи токена
func GenerateJWT(user *User) (string, error) {
	claims := &Claims{
		PlayerID: user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "mmo-game",
			Subject:   user.Username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateJWT проверяет действительность JWT токена и извлекает информацию о пользователе.
// Выполняет валидацию подписи, срока действия и метода подписи.
//
// Параметры:
//
//	tokenString - строка JWT токена для валидации
//
// Возвращает:
//
//	playerID - идентификатор игрока (0 если токен недействителен)
//	isValid - флаг действительности токена
//	isAdmin - флаг администраторских прав (false если токен недействителен)
func ValidateJWT(tokenString string) (playerID uint64, isValid bool, isAdmin bool) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return 0, false, false
	}

	return claims.PlayerID, true, claims.IsAdmin
}

// GenerateSecureSecret генерирует новый криптографически безопасный секретный ключ.
// Ключ имеет длину 32 байта и кодируется в base64.
//
// Возвращает:
//
//	string - base64-кодированный секретный ключ
//	error - ошибка при генерации случайных данных
func GenerateSecureSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SetJWTSecret устанавливает пользовательский секретный ключ для подписи JWT токенов.
// Используется в продакшн среде для установки ключа из переменных окружения.
// Ключ должен быть base64-кодированным и иметь длину не менее 32 байт.
//
// Параметры:
//
//	secret - base64-кодированный секретный ключ
//
// Возвращает:
//
//	error - ошибка при декодировании или валидации ключа
func SetJWTSecret(secret string) error {
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return err
	}
	if len(decoded) < 32 {
		return errors.New("secret key must be at least 32 bytes")
	}
	jwtSecret = decoded
	return nil
}

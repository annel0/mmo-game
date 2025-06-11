package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/golang-jwt/jwt/v5"
)

// GameAuthenticator управляет аутентификацией игроков с поддержкой JWT
type GameAuthenticator struct {
	userRepo     UserRepository
	jwtSecret    []byte
	tokenExpiry  time.Duration
	serverInfo   *protocol.ServerInfo
	capabilities []string
}

// JWTClaims содержит данные JWT токена
type JWTClaims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// NewGameAuthenticator создает новый аутентификатор
func NewGameAuthenticator(repo UserRepository, jwtSecret []byte) *GameAuthenticator {
	if len(jwtSecret) == 0 {
		// Генерируем случайный секрет, если не предоставлен
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось сгенерировать JWT секрет: %v", err)
		}
	}

	return &GameAuthenticator{
		userRepo:     repo,
		jwtSecret:    jwtSecret,
		tokenExpiry:  24 * time.Hour, // 24 часа
		capabilities: []string{"jwt", "rest_api", "webhooks"},
		serverInfo: &protocol.ServerInfo{
			Version:          "v0.0.3a",
			Environment:      "development",
			RestApiAvailable: true,
			RestApiEndpoint:  "http://localhost:8088",
			Features:         []string{"webhooks", "analytics", "outbound_webhooks"},
		},
	}
}

// Authenticate выполняет аутентификацию игрока
func (ga *GameAuthenticator) Authenticate(req *protocol.AuthRequest) (*protocol.AuthResponse, error) {
	// Логируем попытку аутентификации
	log.Printf("🔐 Аутентификация: user=%s, has_password=%v, has_jwt=%v, capabilities=%v",
		req.Username, req.Password != nil, req.JwtToken != nil, req.Capabilities)

	// 1. Аутентификация по JWT токену
	if req.JwtToken != nil && *req.JwtToken != "" {
		return ga.authenticateByJWT(*req.JwtToken, req)
	}

	// 2. Аутентификация по username/password
	if req.Password != nil && *req.Password != "" {
		return ga.authenticateByCredentials(req.Username, *req.Password, req)
	}

	// 3. Аутентификация по старому токену (совместимость)
	if req.Token != nil && *req.Token != "" {
		return ga.authenticateByLegacyToken(*req.Token, req)
	}

	return &protocol.AuthResponse{
		Success: false,
		Message: "Требуется username/password или JWT токен",
	}, nil
}

// authenticateByCredentials аутентификация по логину/паролю
func (ga *GameAuthenticator) authenticateByCredentials(username, password string, req *protocol.AuthRequest) (*protocol.AuthResponse, error) {
	// Проверяем учетные данные
	user, err := ga.userRepo.ValidateCredentials(username, password)
	if err != nil {
		log.Printf("❌ Неудачная аутентификация для пользователя %s: %v", username, err)
		return &protocol.AuthResponse{
			Success: false,
			Message: "Неверные учетные данные",
		}, nil
	}

	log.Printf("✅ Успешная аутентификация пользователя %s (ID: %d)", user.Username, user.ID)

	// Создаем базовый ответ
	response := &protocol.AuthResponse{
		Success:            true,
		Message:            "Аутентификация успешна",
		PlayerId:           user.ID,
		WorldName:          "main_world",
		ServerCapabilities: ga.capabilities,
		ServerInfo:         ga.serverInfo,
	}

	// Генерируем JWT токен если запрошен
	if req.RequestJwt || ga.contains(req.Capabilities, "jwt") {
		jwtToken, expiresAt, err := ga.generateJWT(user)
		if err != nil {
			log.Printf("⚠️ Ошибка генерации JWT для %s: %v", username, err)
		} else {
			response.JwtToken = &jwtToken
			response.JwtExpiresAt = expiresAt
			log.Printf("🎫 JWT токен сгенерирован для %s, действителен до %s",
				username, time.Unix(expiresAt, 0).Format("2006-01-02 15:04:05"))
		}
	}

	// Генерируем legacy токен для совместимости
	legacyToken, err := ga.generateLegacyToken(user)
	if err != nil {
		log.Printf("⚠️ Ошибка генерации legacy токена для %s: %v", username, err)
	} else {
		response.Token = legacyToken
	}

	return response, nil
}

// authenticateByJWT аутентификация по JWT токену
func (ga *GameAuthenticator) authenticateByJWT(jwtToken string, req *protocol.AuthRequest) (*protocol.AuthResponse, error) {
	// Парсим и валидируем JWT
	token, err := jwt.ParseWithClaims(jwtToken, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Проверяем алгоритм подписи
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неожиданный алгоритм подписи: %v", token.Header["alg"])
		}
		return ga.jwtSecret, nil
	})

	if err != nil {
		log.Printf("❌ Ошибка валидации JWT: %v", err)
		return &protocol.AuthResponse{
			Success: false,
			Message: "Недействительный JWT токен",
		}, nil
	}

	if !token.Valid {
		log.Printf("❌ JWT токен недействителен")
		return &protocol.AuthResponse{
			Success: false,
			Message: "JWT токен недействителен",
		}, nil
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		log.Printf("❌ Некорректные claims в JWT токене")
		return &protocol.AuthResponse{
			Success: false,
			Message: "Некорректный формат JWT токена",
		}, nil
	}

	// Получаем пользователя
	user, err := ga.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		log.Printf("❌ Пользователь с ID %d не найден: %v", claims.UserID, err)
		return &protocol.AuthResponse{
			Success: false,
			Message: "Пользователь не найден",
		}, nil
	}

	log.Printf("✅ JWT аутентификация успешна для %s (ID: %d)", user.Username, user.ID)

	return &protocol.AuthResponse{
		Success:            true,
		Message:            "JWT аутентификация успешна",
		PlayerId:           user.ID,
		WorldName:          "main_world",
		ServerCapabilities: ga.capabilities,
		ServerInfo:         ga.serverInfo,
		JwtToken:           &jwtToken, // Возвращаем тот же токен
		JwtExpiresAt:       claims.ExpiresAt.Unix(),
	}, nil
}

// authenticateByLegacyToken аутентификация по старому токену (для совместимости)
func (ga *GameAuthenticator) authenticateByLegacyToken(token string, req *protocol.AuthRequest) (*protocol.AuthResponse, error) {
	// Здесь должна быть логика валидации старого токена
	// Пока просто возвращаем ошибку
	log.Printf("⚠️ Попытка аутентификации с legacy токеном: %s", token[:min(len(token), 10)]+"...")
	return &protocol.AuthResponse{
		Success: false,
		Message: "Legacy токены больше не поддерживаются, используйте username/password",
	}, nil
}

// generateJWT создает новый JWT токен
func (ga *GameAuthenticator) generateJWT(user *User) (string, int64, error) {
	expiresAt := time.Now().Add(ga.tokenExpiry)

	claims := &JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.GetRole(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.Username,
			Issuer:    "mmo-game-server",
			ID:        fmt.Sprintf("user_%d_%d", user.ID, time.Now().Unix()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(ga.jwtSecret)
	if err != nil {
		return "", 0, fmt.Errorf("ошибка подписи JWT токена: %w", err)
	}

	return tokenString, expiresAt.Unix(), nil
}

// generateLegacyToken создает простой токен для совместимости
func (ga *GameAuthenticator) generateLegacyToken(user *User) (string, error) {
	// Простой токен на основе base64
	tokenData := fmt.Sprintf("%d:%s:%d", user.ID, user.Username, time.Now().Unix())
	return base64.StdEncoding.EncodeToString([]byte(tokenData)), nil
}

// contains проверяет наличие строки в слайсе
func (ga *GameAuthenticator) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// SetJWTSecret устанавливает новый JWT секрет
func (ga *GameAuthenticator) SetJWTSecret(secret []byte) {
	ga.jwtSecret = secret
}

// GetServerInfo возвращает информацию о сервере
func (ga *GameAuthenticator) GetServerInfo() *protocol.ServerInfo {
	return ga.serverInfo
}

// min возвращает минимальное значение из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

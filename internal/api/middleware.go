package api

import (
	"net/http"
	"strings"

	"github.com/annel0/mmo-game/internal/auth"
	"github.com/gin-gonic/gin"
)

// jwtMiddleware проверяет JWT токен в заголовке Authorization
func (rs *RestServer) jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Получаем Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, GenericResponse{
				Success: false,
				Message: "Отсутствует токен авторизации",
			})
			c.Abort()
			return
		}

		// Проверяем формат "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, GenericResponse{
				Success: false,
				Message: "Неверный формат токена",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// Валидируем JWT токен
		playerID, isValid, isAdmin := auth.ValidateJWT(token)
		if !isValid {
			c.JSON(http.StatusUnauthorized, GenericResponse{
				Success: false,
				Message: "Недействительный токен",
			})
			c.Abort()
			return
		}

		// Сохраняем информацию о пользователе в контексте
		c.Set("player_id", playerID)
		c.Set("is_admin", isAdmin)
		c.Set("token", token)

		c.Next()
	}
}

// adminMiddleware проверяет, что пользователь является администратором
func (rs *RestServer) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Получаем информацию о пользователе из контекста (установлена в jwtMiddleware)
		isAdmin, exists := c.Get("is_admin")
		if !exists {
			c.JSON(http.StatusInternalServerError, GenericResponse{
				Success: false,
				Message: "Отсутствует информация о пользователе",
			})
			c.Abort()
			return
		}

		// Проверяем права администратора
		if !isAdmin.(bool) {
			c.JSON(http.StatusForbidden, GenericResponse{
				Success: false,
				Message: "Недостаточно прав доступа",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware ограничивает количество запросов (заглушка для будущего развития)
func (rs *RestServer) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Реализовать rate limiting
		c.Next()
	}
}

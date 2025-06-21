package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/middleware"
	"github.com/annel0/mmo-game/internal/world/entity"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// RestServer представляет REST API сервер
type RestServer struct {
	router           *gin.Engine
	userRepo         auth.UserRepository
	entityManager    *entity.EntityManager
	port             string
	metrics          *ServerMetrics
	webhookConfig    WebhookConfig
	outboundWebhooks *OutboundWebhookManager
}

// Config содержит конфигурацию для REST сервера
type Config struct {
	Port          string                // порт для запуска сервера
	UserRepo      auth.UserRepository   // репозиторий пользователей
	EntityManager *entity.EntityManager // менеджер сущностей
}

// NewRestServer создает новый REST API сервер
func NewRestServer(config Config) *RestServer {
	if config.Port == "" {
		config.Port = ":8080"
	}

	// Устанавливаем режим релиза для gin
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()        // без стандартного logger/recovery
	router.Use(gin.Recovery()) // добавим только recovery

	// === Observability middleware ===
	loggerMw := middleware.NewRequestLogger()
	router.Use(loggerMw.Handler())

	otelRouter := otelgin.Middleware("rest_api")
	router.Use(otelRouter)

	promMw := middleware.NewPrometheusMiddleware("rest_api")
	router.Use(promMw.Handler())
	promMw.RegisterMetricsEndpoint(router)

	server := &RestServer{
		router:        router,
		userRepo:      config.UserRepo,
		entityManager: config.EntityManager,
		port:          config.Port,
		metrics:       NewServerMetrics(),
		webhookConfig: WebhookConfig{
			SecretKey:        "", // Можно настроить через переменные окружения
			RequireSignature: false,
			EnableLogging:    true,
		},
		outboundWebhooks: NewOutboundWebhookManager("game_server_01", "development"),
	}

	// Настраиваем маршруты
	server.setupRoutes()

	return server
}

// setupRoutes настраивает маршруты REST API
func (rs *RestServer) setupRoutes() {
	// Middleware для CORS
	rs.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Группа API
	api := rs.router.Group("/api")

	// Эндпоинт для аутентификации (без JWT защиты)
	auth := api.Group("/auth")
	{
		auth.POST("/login", rs.handleLogin)
	}

	// Защищенные эндпоинты (требуют JWT)
	protected := api.Group("/")
	protected.Use(rs.jwtMiddleware())
	{
		// Статистика (доступна всем аутентифицированным пользователям)
		protected.GET("/stats", rs.handleStats)
		protected.GET("/server", rs.handleServerInfo)

		// Административные эндпоинты (только для админов)
		admin := protected.Group("/admin")
		admin.Use(rs.adminMiddleware())
		{
			admin.POST("/register", rs.handleAdminRegister)
			admin.GET("/users", rs.handleGetUsers)
			admin.POST("/ban", rs.handleBanUser)
			admin.POST("/unban", rs.handleUnbanUser)

			// Управление исходящими webhook'ами
			admin.GET("/webhooks", rs.handleGetOutboundWebhooks)
			admin.POST("/webhooks", rs.handleCreateOutboundWebhook)
			admin.GET("/webhooks/:id", rs.handleGetOutboundWebhook)
			admin.PUT("/webhooks/:id", rs.handleUpdateOutboundWebhook)
			admin.DELETE("/webhooks/:id", rs.handleDeleteOutboundWebhook)
			admin.POST("/webhooks/:id/test", rs.handleTestOutboundWebhook)
			admin.GET("/webhooks/events", rs.handleGetWebhookEventTypes)
			admin.POST("/events/send", rs.handleSendEvent)
		}
	}

	// Webhook (без аутентификации, но с валидацией)
	api.POST("/webhook", rs.HandleWebhook)

	// Health check
	rs.router.GET("/health", rs.handleHealth)
}

// LoginRequest представляет запрос на вход
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse представляет ответ на вход
type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message"`
	UserID  uint64 `json:"user_id,omitempty"`
	IsAdmin bool   `json:"is_admin,omitempty"`
}

// RegisterRequest представляет запрос на регистрацию
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	IsAdmin  bool   `json:"is_admin"`
}

// GenericResponse представляет общий ответ API
type GenericResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// handleLogin обрабатывает запрос на вход
func (rs *RestServer) handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, LoginResponse{
			Success: false,
			Message: "Неверный формат запроса",
		})
		return
	}

	// Получаем пользователя из БД
	user, err := rs.userRepo.GetUserByUsername(req.Username)
	if err == auth.ErrUserNotFound {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Неверное имя пользователя или пароль",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Внутренняя ошибка сервера",
		})
		return
	}

	// Проверяем пароль
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Неверное имя пользователя или пароль",
		})
		return
	}

	// Генерируем JWT токен
	token, err := auth.GenerateJWT(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Ошибка генерации токена",
		})
		return
	}

	// Обновляем время последнего входа (если поддерживается)
	if mariaRepo, ok := rs.userRepo.(*auth.MariaUserRepo); ok {
		_ = mariaRepo.UpdateUserLastLogin(user.ID)
	}

	c.JSON(http.StatusOK, LoginResponse{
		Success: true,
		Token:   token,
		Message: "Успешная авторизация",
		UserID:  user.ID,
		IsAdmin: user.IsAdmin,
	})
}

// handleAdminRegister обрабатывает регистрацию нового пользователя (только для админов)
func (rs *RestServer) handleAdminRegister(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный формат запроса",
		})
		return
	}

	// Валидация входных данных
	if len(req.Username) < 3 || len(req.Username) > 30 {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Имя пользователя должно быть от 3 до 30 символов",
		})
		return
	}

	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Пароль должен быть минимум 6 символов",
		})
		return
	}

	// Хешируем пароль
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{
			Success: false,
			Message: "Ошибка обработки пароля",
		})
		return
	}

	// Создаем пользователя
	user, err := rs.userRepo.CreateUser(req.Username, passwordHash, req.IsAdmin)
	if err == auth.ErrUserExists {
		c.JSON(http.StatusConflict, GenericResponse{
			Success: false,
			Message: "Пользователь уже существует",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, GenericResponse{
			Success: false,
			Message: "Ошибка создания пользователя",
		})
		return
	}

	c.JSON(http.StatusCreated, GenericResponse{
		Success: true,
		Message: "Пользователь успешно создан",
		Data: map[string]interface{}{
			"user_id":  user.ID,
			"username": user.Username,
			"is_admin": user.IsAdmin,
		},
	})
}

// handleStats возвращает статистику сервера
func (rs *RestServer) handleStats(c *gin.Context) {
	stats := make(map[string]interface{})

	// Статистика пользователей (если поддерживается)
	if mariaRepo, ok := rs.userRepo.(*auth.MariaUserRepo); ok {
		userStats, err := mariaRepo.GetUserStats()
		if err == nil {
			stats["users"] = userStats
		}
	}

	// Статистика сущностей
	if rs.entityManager != nil {
		entityStats := rs.entityManager.GetStats()
		stats["entities"] = entityStats
	}

	// Метрики сервера
	memoryMB, _ := rs.metrics.GetMemoryUsage()
	cpuPercent, _ := rs.metrics.GetCPUUsage()
	systemCPU, _ := rs.metrics.GetSystemCPUUsage()

	stats["server"] = map[string]interface{}{
		"uptime":      rs.metrics.GetUptime(),
		"memory_mb":   fmt.Sprintf("%.2f", memoryMB),
		"cpu_percent": fmt.Sprintf("%.2f", cpuPercent),
		"system_cpu":  fmt.Sprintf("%.2f", systemCPU),
		"server_time": time.Now().Unix(),
	}

	// Детальная статистика памяти
	stats["memory_details"] = rs.metrics.GetDetailedMemoryStats()

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Статистика получена",
		Data:    stats,
	})
}

// handleServerInfo возвращает информацию о сервере
func (rs *RestServer) handleServerInfo(c *gin.Context) {
	// Получаем реальные метрики
	memoryMB, _ := rs.metrics.GetMemoryUsage()
	cpuPercent, _ := rs.metrics.GetCPUUsage()

	info := map[string]interface{}{
		"version":     "v0.0.2a",
		"name":        "MMO Game Server",
		"status":      "running",
		"uptime":      rs.metrics.GetUptime(),
		"memory_mb":   fmt.Sprintf("%.1f", memoryMB),
		"cpu_percent": fmt.Sprintf("%.1f", cpuPercent),
	}

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Информация о сервере",
		Data:    info,
	})
}

// handleGetUsers возвращает список пользователей (только для админов)
func (rs *RestServer) handleGetUsers(c *gin.Context) {
	// Параметры пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// Пока возвращаем заглушку - в реальности нужно реализовать пагинацию в репозитории
	users := []map[string]interface{}{
		{
			"id":         1,
			"username":   "admin",
			"is_admin":   true,
			"created_at": time.Now().Add(-time.Hour * 24),
			"last_login": time.Now(),
		},
	}

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Список пользователей",
		Data: map[string]interface{}{
			"users": users,
			"page":  page,
			"limit": limit,
			"total": len(users),
		},
	})
}

// handleBanUser банит пользователя (заглушка)
func (rs *RestServer) handleBanUser(c *gin.Context) {
	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Пользователь забанен (заглушка)",
	})
}

// handleUnbanUser разбанивает пользователя (заглушка)
func (rs *RestServer) handleUnbanUser(c *gin.Context) {
	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Пользователь разбанен (заглушка)",
	})
}

// handleWebhook обрабатывает webhook запросы
func (rs *RestServer) handleWebhook(c *gin.Context) {
	// Проверяем Content-Type
	if !strings.Contains(c.GetHeader("Content-Type"), "application/json") {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный Content-Type",
		})
		return
	}

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный JSON",
		})
		return
	}

	// Простая обработка webhook (можно расширить)
	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Webhook обработан",
		Data:    payload,
	})
}

// handleHealth проверка состояния сервера
func (rs *RestServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

// Start запускает REST сервер
func (rs *RestServer) Start() error {
	return rs.router.Run(rs.port)
}

// Stop останавливает REST сервер (заглушка для graceful shutdown)
func (rs *RestServer) Stop() error {
	// В реальности здесь должен быть graceful shutdown
	return nil
}

// === ОБРАБОТЧИКИ ИСХОДЯЩИХ WEBHOOK'ОВ ===

// handleGetOutboundWebhooks возвращает список исходящих webhook'ов
func (rs *RestServer) handleGetOutboundWebhooks(c *gin.Context) {
	webhooks := rs.outboundWebhooks.GetWebhooks()

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Список webhook'ов получен",
		Data: map[string]interface{}{
			"webhooks": webhooks,
			"total":    len(webhooks),
		},
	})
}

// handleCreateOutboundWebhook создает новый исходящий webhook
func (rs *RestServer) handleCreateOutboundWebhook(c *gin.Context) {
	var webhook OutboundWebhook
	if err := c.ShouldBindJSON(&webhook); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный формат webhook'а: " + err.Error(),
		})
		return
	}

	// Валидация
	if webhook.Name == "" || webhook.URL == "" || len(webhook.Events) == 0 {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Обязательные поля: name, url, events",
		})
		return
	}

	createdWebhook := rs.outboundWebhooks.AddWebhook(webhook)

	c.JSON(http.StatusCreated, GenericResponse{
		Success: true,
		Message: "Webhook создан успешно",
		Data:    createdWebhook,
	})
}

// handleGetOutboundWebhook возвращает webhook по ID
func (rs *RestServer) handleGetOutboundWebhook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный ID webhook'а",
		})
		return
	}

	webhook := rs.outboundWebhooks.GetWebhook(id)
	if webhook == nil {
		c.JSON(http.StatusNotFound, GenericResponse{
			Success: false,
			Message: "Webhook не найден",
		})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Webhook найден",
		Data:    webhook,
	})
}

// handleUpdateOutboundWebhook обновляет webhook
func (rs *RestServer) handleUpdateOutboundWebhook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный ID webhook'а",
		})
		return
	}

	var updates OutboundWebhook
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный формат обновлений: " + err.Error(),
		})
		return
	}

	updatedWebhook := rs.outboundWebhooks.UpdateWebhook(id, updates)
	if updatedWebhook == nil {
		c.JSON(http.StatusNotFound, GenericResponse{
			Success: false,
			Message: "Webhook не найден",
		})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Webhook обновлен успешно",
		Data:    updatedWebhook,
	})
}

// handleDeleteOutboundWebhook удаляет webhook
func (rs *RestServer) handleDeleteOutboundWebhook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный ID webhook'а",
		})
		return
	}

	if !rs.outboundWebhooks.DeleteWebhook(id) {
		c.JSON(http.StatusNotFound, GenericResponse{
			Success: false,
			Message: "Webhook не найден",
		})
		return
	}

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Webhook удален успешно",
	})
}

// handleTestOutboundWebhook тестирует webhook отправкой тестового события
func (rs *RestServer) handleTestOutboundWebhook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный ID webhook'а",
		})
		return
	}

	webhook := rs.outboundWebhooks.GetWebhook(id)
	if webhook == nil {
		c.JSON(http.StatusNotFound, GenericResponse{
			Success: false,
			Message: "Webhook не найден",
		})
		return
	}

	// Отправляем тестовое событие
	rs.outboundWebhooks.SendEvent("webhook.test", map[string]interface{}{
		"webhook_id":   id,
		"webhook_name": webhook.Name,
		"test_time":    time.Now().Unix(),
		"message":      "Это тестовое сообщение от игрового сервера",
	})

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Тестовое событие отправлено",
		Data: map[string]interface{}{
			"webhook_id": id,
			"sent_at":    time.Now().Unix(),
		},
	})
}

// handleGetWebhookEventTypes возвращает доступные типы событий
func (rs *RestServer) handleGetWebhookEventTypes(c *gin.Context) {
	eventTypes := rs.outboundWebhooks.GetEventTypes()

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Типы событий получены",
		Data: map[string]interface{}{
			"event_types": eventTypes,
			"total":       len(eventTypes),
		},
	})
}

// handleSendEvent позволяет админам отправлять кастомные события
func (rs *RestServer) handleSendEvent(c *gin.Context) {
	var request struct {
		EventType string                 `json:"event_type" binding:"required"`
		Data      map[string]interface{} `json:"data"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, GenericResponse{
			Success: false,
			Message: "Неверный формат запроса: " + err.Error(),
		})
		return
	}

	// Отправляем событие через webhook менеджер
	rs.outboundWebhooks.SendEvent(request.EventType, request.Data)

	c.JSON(http.StatusOK, GenericResponse{
		Success: true,
		Message: "Событие отправлено",
		Data: map[string]interface{}{
			"event_type": request.EventType,
			"sent_at":    time.Now().Unix(),
		},
	})
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// WebhookEvent структура для получения webhook событий
type WebhookEvent struct {
	EventType   string                 `json:"event_type"`
	Timestamp   int64                  `json:"timestamp"`
	ServerID    string                 `json:"server_id"`
	Data        map[string]interface{} `json:"data"`
	Source      string                 `json:"source"`
	Environment string                 `json:"environment"`
}

func main() {
	log.Println("🔗 Запуск тестового Webhook приемника...")

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Middleware для логирования
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC3339),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))

	// Главная страница с информацией
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message":     "Webhook приемник запущен",
			"endpoints":   []string{"/webhook", "/webhooks/anticheat", "/webhooks/server", "/webhooks/player"},
			"server_time": time.Now().Unix(),
		})
	})

	// Основной webhook эндпоинт
	r.POST("/webhook", handleWebhook)

	// Специализированные эндпоинты
	r.POST("/webhooks/anticheat", handleAnticheatWebhook)
	r.POST("/webhooks/server", handleServerWebhook)
	r.POST("/webhooks/player", handlePlayerWebhook)

	log.Println("✅ Webhook приемник запущен на http://localhost:3000")
	log.Println("📋 Доступные эндпоинты:")
	log.Println("   GET  /                      - Информация о сервере")
	log.Println("   POST /webhook               - Основной webhook")
	log.Println("   POST /webhooks/anticheat    - Античит события")
	log.Println("   POST /webhooks/server       - Серверные события")
	log.Println("   POST /webhooks/player       - Игровые события")

	if err := r.Run(":3000"); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}

// handleWebhook обрабатывает общие webhook события
func handleWebhook(c *gin.Context) {
	var event WebhookEvent

	// Читаем тело запроса
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("❌ Ошибка чтения тела запроса: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка чтения запроса"})
		return
	}

	// Парсим JSON
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("❌ Ошибка парсинга JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный JSON"})
		return
	}

	// Логируем заголовки
	log.Printf("📧 Получен webhook:")
	log.Printf("   Event Type: %s", event.EventType)
	log.Printf("   Server ID: %s", event.ServerID)
	log.Printf("   Timestamp: %d (%s)", event.Timestamp, time.Unix(event.Timestamp, 0).Format("15:04:05"))
	log.Printf("   Headers:")
	for key, values := range c.Request.Header {
		for _, value := range values {
			log.Printf("     %s: %s", key, value)
		}
	}

	// Обрабатываем в зависимости от типа события
	switch event.EventType {
	case "server.started":
		log.Printf("Сервер %s запущен!", event.ServerID)
	case "server.stopped":
		log.Printf("⏹️  Сервер %s остановлен!", event.ServerID)
	case "server.error":
		log.Printf("❌ Ошибка на сервере %s: %v", event.ServerID, event.Data["error"])
	case "server.high_cpu":
		cpu := event.Data["cpu_percent"]
		log.Printf("⚠️  Высокая загрузка CPU на сервере %s: %v%%", event.ServerID, cpu)
	case "server.low_tps":
		tps := event.Data["tps"]
		log.Printf("⚠️  Низкий TPS на сервере %s: %v", event.ServerID, tps)
	case "player.joined":
		player := event.Data["username"]
		log.Printf("👤 Игрок %v подключился к серверу %s", player, event.ServerID)
	case "player.left":
		player := event.Data["username"]
		log.Printf("👋 Игрок %v покинул сервер %s", player, event.ServerID)
	case "anticheat.violation":
		player := event.Data["username"]
		violation := event.Data["violation_type"]
		log.Printf("🚨 АНТИЧИТ: Игрок %v нарушил правила (%v) на сервере %s", player, violation, event.ServerID)
	case "webhook.test":
		log.Printf("🧪 Тестовое событие от webhook ID %v", event.Data["webhook_id"])
	default:
		log.Printf("ℹ️  Неизвестное событие: %s", event.EventType)
	}

	// Выводим данные события
	if len(event.Data) > 0 {
		log.Printf("   Данные события:")
		for key, value := range event.Data {
			log.Printf("     %s: %v", key, value)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "received",
		"event_type":  event.EventType,
		"received_at": time.Now().Unix(),
	})
}

// handleAnticheatWebhook специализированный обработчик для античита
func handleAnticheatWebhook(c *gin.Context) {
	var event WebhookEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный JSON"})
		return
	}

	log.Printf("🛡️  АНТИЧИТ СОБЫТИЕ: %s", event.EventType)

	if event.EventType == "anticheat.violation" {
		player := event.Data["username"]
		violation := event.Data["violation_type"]
		severity := event.Data["severity"]

		log.Printf("🚨 НАРУШЕНИЕ АНТИЧИТА:")
		log.Printf("   Игрок: %v", player)
		log.Printf("   Тип: %v", violation)
		log.Printf("   Серьезность: %v", severity)
		log.Printf("   Сервер: %s", event.ServerID)

		// Здесь можно добавить логику для:
		// - Отправки в Discord/Telegram
		// - Записи в базу данных
		// - Автоматического бана
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// handleServerWebhook обработчик серверных событий
func handleServerWebhook(c *gin.Context) {
	var event WebhookEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный JSON"})
		return
	}

	log.Printf("🖥️  СЕРВЕРНОЕ СОБЫТИЕ: %s", event.EventType)

	switch event.EventType {
	case "server.high_cpu", "server.high_memory":
		// Алерт для мониторинга
		log.Printf("⚠️  МОНИТОРИНГ: Высокая нагрузка на сервере %s", event.ServerID)
	case "server.error":
		// Критическая ошибка
		log.Printf("🚨 КРИТИЧЕСКАЯ ОШИБКА на сервере %s", event.ServerID)
	}

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

// handlePlayerWebhook обработчик игровых событий
func handlePlayerWebhook(c *gin.Context) {
	var event WebhookEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный JSON"})
		return
	}

	log.Printf("🎮 ИГРОВОЕ СОБЫТИЕ: %s", event.EventType)

	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

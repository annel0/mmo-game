package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/annel0/mmo-game/internal/api"
	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/network"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// simulateGameEvents симулирует различные игровые события для демонстрации webhook'ов
func simulateGameEvents(webhookManager *api.OutboundWebhookManager) {
	ticker := time.NewTicker(30 * time.Second) // События каждые 30 секунд
	defer ticker.Stop()

	players := []string{"player1", "player2", "player3", "hacker_user", "speedrunner"}
	violations := []string{"speed_hack", "fly_hack", "x-ray", "auto_clicker", "kill_aura"}

	for {
		select {
		case <-ticker.C:
			// Случайно выбираем тип события
			eventType := rand.Intn(5)

			switch eventType {
			case 0:
				// Игрок подключился
				player := players[rand.Intn(len(players))]
				webhookManager.SendEvent("player.joined", map[string]interface{}{
					"username": player,
					"ip":       "192.168.1." + fmt.Sprintf("%d", rand.Intn(254)+1),
					"time":     time.Now().Unix(),
				})

			case 1:
				// Игрок покинул сервер
				player := players[rand.Intn(len(players))]
				webhookManager.SendEvent("player.left", map[string]interface{}{
					"username": player,
					"time":     time.Now().Unix(),
					"reason":   "disconnect",
				})

			case 2:
				// Нарушение античита
				player := players[rand.Intn(len(players))]
				violation := violations[rand.Intn(len(violations))]
				webhookManager.SendEvent("anticheat.violation", map[string]interface{}{
					"username":       player,
					"violation_type": violation,
					"severity":       rand.Intn(10) + 1,
					"details":        "Автоматически обнаружено системой античита",
					"time":           time.Now().Unix(),
				})

			case 3:
				// Высокая нагрузка CPU
				webhookManager.SendEvent("server.high_cpu", map[string]interface{}{
					"cpu_percent": rand.Float64()*20 + 80, // 80-100%
					"duration":    rand.Intn(300) + 60,    // 1-5 минут
					"alert_level": "warning",
				})

			case 4:
				// Сохранение мира
				webhookManager.SendEvent("world.saved", map[string]interface{}{
					"save_duration_ms": rand.Intn(5000) + 1000, // 1-6 секунд
					"chunks_saved":     rand.Intn(1000) + 500,
					"backup_created":   rand.Intn(2) == 1, // случайно true/false
				})
			}
		}
	}
}

// Пример интеграции REST API + игрового сервера
func main() {
	log.Println("🎮 Запуск MMO Game Server с REST API...")

	// Создаем менеджер сущностей
	entityManager := entity.NewEntityManager()
	entityManager.RegisterDefaultBehaviors()

	// Конфигурация для REST API + MariaDB
	apiConfig := api.IntegrationConfig{
		RestPort: ":8088",
		MariaConfig: auth.MariaConfig{
			Host:     "localhost",
			Port:     3306,
			Database: "blockverse",
			Username: "gameuser",    // Замените на реальные данные
			Password: "gamepass123", // Замените на реальные данные
		},
		EntityManager: entityManager,
		UseMariaDB:    true, // Установите true для использования MariaDB
	}

	// Создаем интеграцию REST API
	apiIntegration, err := api.NewServerIntegration(apiConfig)
	if err != nil {
		log.Fatalf("❌ Ошибка создания REST API интеграции: %v", err)
	}

	// Запускаем REST API сервер
	if err := apiIntegration.Start(); err != nil {
		log.Fatalf("❌ Ошибка запуска REST API: %v", err)
	}

	// Создаем игровой сервер (опционально)
	gameServer, err := network.NewGameServerPB(":7777", ":7778")
	if err != nil {
		log.Fatalf("❌ Ошибка создания игрового сервера: %v", err)
	}

	// Используем тот же репозиторий пользователей для игрового сервера
	_ = apiIntegration.GetUserRepository()
	log.Printf("✅ Используется общий репозиторий пользователей для игры и REST API")

	// Получаем доступ к менеджеру исходящих webhook'ов
	webhookManager := apiIntegration.GetOutboundWebhooks()

	// Отправляем событие о запуске сервера
	webhookManager.SendEvent("server.started", map[string]interface{}{
		"version":      "v0.0.2a",
		"environment":  "development",
		"startup_time": time.Now().Unix(),
		"features":     []string{"rest_api", "mariadb", "webhooks"},
	})

	// Запускаем симуляцию игровых событий
	go simulateGameEvents(webhookManager)

	// Запускаем игровой сервер
	go func() {
		if err := gameServer.Start(); err != nil {
			log.Printf("❌ Ошибка игрового сервера: %v", err)
		}
	}()

	log.Println("Сервер запущен успешно!")
	log.Println("📋 Доступные сервисы:")
	log.Println("   🎮 Игровой сервер: TCP :7777, UDP :7778")
	log.Println("   🌐 REST API: http://localhost:8088")
	log.Println("   ❤️  Health check: http://localhost:8088/health")

	// Примеры использования REST API
	log.Println("\n💡 Примеры использования REST API:")
	log.Println("   curl http://localhost:8088/health")
	log.Println("   curl -X POST http://localhost:8088/api/auth/login -H 'Content-Type: application/json' -d '{\"username\":\"admin\",\"password\":\"ChangeMe123!\"}'")
	log.Println("   curl -H 'Authorization: Bearer YOUR_JWT_TOKEN' http://localhost:8088/api/stats")

	// Ожидаем сигнал завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Получен сигнал завершения...")

	// Отправляем событие об остановке сервера
	webhookManager.SendEvent("server.stopped", map[string]interface{}{
		"shutdown_time": time.Now().Unix(),
		"reason":        "manual_shutdown",
	})

	// Останавливаем сервисы
	if err := apiIntegration.Stop(); err != nil {
		log.Printf("❌ Ошибка остановки REST API: %v", err)
	}

	gameServer.Stop()

	log.Println("👋 Сервер остановлен")
}

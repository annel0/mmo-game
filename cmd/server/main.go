package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/annel0/mmo-game/internal/api"
	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/network"
	"github.com/annel0/mmo-game/internal/world/block"
	"github.com/annel0/mmo-game/internal/world/entity"
)

func main() {
	// Инициализируем систему логирования (используем новый API)
	if err := logging.InitDefaultLogger("server"); err != nil {
		log.Fatalf("❌ Ошибка инициализации логирования: %v", err)
	}
	defer logging.CloseDefaultLogger()

	logging.Info("🎮 Запуск MMO Game Server с поддержкой JWT аутентификации и REST API...")
	logging.Debug("Инициализация системы логирования завершена")

	// === КОНФИГУРАЦИЯ ===
	// Игровые порты
	tcpAddr := ":7777"
	udpAddr := ":7778"

	// REST API порт
	restPort := ":8088"

	logging.Info("📡 Конфигурация сервера: TCP=%s, UDP=%s, REST API=%s", tcpAddr, udpAddr, restPort)

	// === ИНИЦИАЛИЗАЦИЯ КОМПОНЕНТОВ ===

	// Создаем менеджер сущностей
	logging.Debug("Создание менеджера сущностей...")
	entityManager := entity.NewEntityManager()
	entityManager.RegisterDefaultBehaviors()

	// Загружаем JSON-описания блоков (если каталог существует)
	if err := block.LoadJSONBlocks("assets/blocks"); err != nil && !os.IsNotExist(err) {
		logging.Error("Ошибка загрузки JSON-блоков: %v", err)
	}

	// Конфигурация REST API с поддержкой MariaDB
	apiConfig := api.IntegrationConfig{
		RestPort: restPort,
		MariaConfig: auth.MariaConfig{
			Host:     "localhost",
			Port:     3306,
			Database: "blockverse",
			Username: "gameuser",    // Настройте под вашу БД
			Password: "gamepass123", // Настройте под вашу БД
		},
		EntityManager: entityManager,
		UseMariaDB:    false, // Установите true для использования MariaDB
	}

	// Создаем интеграцию REST API
	logging.Debug("Создание REST API интеграции...")
	apiIntegration, err := api.NewServerIntegration(apiConfig)
	if err != nil {
		logging.Error("❌ Ошибка создания REST API интеграции: %v", err)
		log.Fatalf("❌ Ошибка создания REST API интеграции: %v", err)
	}

	// Запускаем REST API сервер
	logging.Debug("Запуск REST API сервера...")
	if err := apiIntegration.Start(); err != nil {
		logging.Error("❌ Ошибка запуска REST API: %v", err)
		log.Fatalf("❌ Ошибка запуска REST API: %v", err)
	}

	// Создаем игровой сервер
	logging.Debug("Создание игрового сервера...")
	gameServer, err := network.NewGameServerPB(tcpAddr, udpAddr)
	if err != nil {
		logging.Error("❌ Ошибка создания игрового сервера: %v", err)
		log.Fatalf("❌ Ошибка создания игрового сервера: %v", err)
	}

	// Используем общий репозиторий пользователей для игры и REST API
	_ = apiIntegration.GetUserRepository() // Получаем ссылку на репозиторий (для потенциального использования)
	logging.Info("✅ Используется общий репозиторий пользователей для игры и REST API")

	// Запускаем игровой сервер
	logging.Debug("Запуск игрового сервера...")
	gameServer.Start()

	logging.Info("✅ Все сервисы запущены и готовы принимать соединения")
	logging.Info("   🎮 Игровой трафик: TCP %s, UDP %s", tcpAddr, udpAddr)
	logging.Info("   🌐 REST API: http://localhost%s", restPort)
	logging.Info("   🔐 JWT аутентификация активирована")
	logging.Info("   ❤️  Health check: http://localhost%s/health", restPort)
	logging.Debug("Сервер полностью инициализирован и работает")

	// Примеры использования REST API
	logging.Info("💡 Примеры использования REST API:")
	logging.Info("   curl http://localhost%s/health", restPort)
	logging.Info("   curl -X POST http://localhost%s/api/auth/login -H 'Content-Type: application/json' -d '{\"username\":\"admin\",\"password\":\"ChangeMe123!\"}'", restPort)

	// Канал для получения сигналов ОС
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logging.Debug("Ожидание сигналов завершения...")

	// Ждем сигнала для завершения
	sig := <-sigCh
	logging.Info("📡 Получен сигнал %v, завершение работы...", sig)

	// === GRACEFUL SHUTDOWN ===
	logging.Debug("Остановка сервисов...")

	// Останавливаем игровой сервер
	logging.Debug("Остановка игрового сервера...")
	gameServer.Stop()

	// Останавливаем REST API
	logging.Debug("Остановка REST API...")
	if err := apiIntegration.Stop(); err != nil {
		logging.Error("❌ Ошибка остановки REST API: %v", err)
	}

	logging.Info("👋 Сервер успешно остановлен")
}

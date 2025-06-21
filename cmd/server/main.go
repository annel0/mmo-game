package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/annel0/mmo-game/internal/api"
	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/config"
	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/network"
	"github.com/annel0/mmo-game/internal/observability"
	"github.com/annel0/mmo-game/internal/regional"
	"github.com/annel0/mmo-game/internal/sync"
	"github.com/annel0/mmo-game/internal/world"
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

	// === TELEMETRY ===
	shutdownTel, err := observability.InitTelemetry(context.Background(), "mmo_server")
	if err != nil {
		logging.Warn("Не удалось инициализировать OpenTelemetry: %v", err)
	}

	// === КОНФИГУРАЦИЯ ===
	cfg, err := config.Load("")
	if err != nil {
		logging.Warn("Не удалось загрузить config: %v", err)
	}

	// Порты сервера с поддержкой конфигурации и fallback на environment variables
	var serverCfg config.ServerConfig
	if cfg != nil {
		serverCfg = cfg.Server
	}

	tcpPort := serverCfg.GetTCPPort()
	udpPort := serverCfg.GetUDPPort()
	restPort := serverCfg.GetRESTPort()
	metricsPort := serverCfg.GetMetricsPort()

	// Форматируем адреса
	tcpAddr := fmt.Sprintf(":%d", tcpPort)
	udpAddr := fmt.Sprintf(":%d", udpPort)
	restAddr := fmt.Sprintf(":%d", restPort)
	metricsAddr := fmt.Sprintf(":%d", metricsPort)

	// EventBus параметры из конфигурации (с дефолтами)
	natsURL := "nats://127.0.0.1:4222"
	streamName := "EVENTS"
	retention := 24
	if cfg != nil {
		if cfg.EventBus.URL != "" {
			natsURL = cfg.EventBus.URL
		}
		if cfg.EventBus.Stream != "" {
			streamName = cfg.EventBus.Stream
		}
		if cfg.EventBus.Retention > 0 {
			retention = cfg.EventBus.Retention
		}
	}

	logging.Info("📡 Конфигурация сервера: TCP=%s, UDP=%s, REST API=%s", tcpAddr, udpAddr, restAddr)

	// === ИНИЦИАЛИЗАЦИЯ EVENTBUS ===
	bus, err := eventbus.NewJetStreamBus(natsURL, streamName, time.Duration(retention)*time.Hour)
	if err != nil {
		logging.Error("❌ Не удалось инициализировать JetStreamBus: %v", err)
		log.Fatalf("EventBus init failed: %v", err)
	}

	eventbus.Init(bus)
	logging.Info("✅ JetStreamBus подключён %s", natsURL)

	// Запускаем internal listener и Prometheus metrics
	if err := eventbus.StartLoggingListener(bus); err != nil {
		logging.Warn("Не удалось запустить LoggingListener: %v", err)
	}

	exporter := eventbus.NewMetricsExporter(bus)
	exporter.StartHTTP(metricsAddr)

	// === ИНИЦИАЛИЗАЦИЯ SYNC ===
	syncCfg := sync.SyncConfig{
		RegionID:     "region-eu-west",
		Bus:          bus,
		BatchSize:    100,
		FlushEvery:   3 * time.Second,
		UseGzipCompr: true,
	}
	if cfg != nil && cfg.Sync.RegionID != "" {
		syncCfg.RegionID = cfg.Sync.RegionID
		if cfg.Sync.BatchSize > 0 {
			syncCfg.BatchSize = cfg.Sync.BatchSize
		}
		if cfg.Sync.FlushEvery > 0 {
			syncCfg.FlushEvery = time.Duration(cfg.Sync.FlushEvery) * time.Second
		}
		syncCfg.UseGzipCompr = cfg.Sync.UseGzipCompr
	}

	syncManager, err := sync.NewSyncManager(syncCfg)
	if err != nil {
		logging.Warn("Не удалось инициализировать SyncManager: %v", err)
	}

	// === ИНИЦИАЛИЗАЦИЯ REGIONAL NODE ===
	// Создаём локальный мир для регионального узла
	localWorld := world.NewWorldManager(time.Now().Unix()) // Используем timestamp как seed

	// Получаем BatchManager из SyncManager
	var batchManager *sync.BatchManager
	if syncManager != nil {
		// TODO: Добавить метод GetBatchManager в SyncManager
		// Пока создаём новый BatchManager напрямую
		batchManager = sync.NewBatchManager(bus, syncCfg.RegionID, syncCfg.BatchSize, syncCfg.FlushEvery, nil)
	}

	// Конфигурация регионального узла
	regionalCfg := regional.NodeConfig{
		RegionID:     syncCfg.RegionID,
		WorldManager: localWorld,
		EventBus:     bus,
		BatchManager: batchManager,
		Resolver:     nil, // Будет использован LWWResolver по умолчанию
	}

	// Создаём региональный узел
	regionalNode, err := regional.NewRegionalNode(regionalCfg)
	if err != nil {
		logging.Warn("Не удалось создать RegionalNode: %v", err)
	} else {
		// Запускаем региональный узел
		if err := regionalNode.Start(context.Background()); err != nil {
			logging.Warn("Не удалось запустить RegionalNode: %v", err)
		} else {
			logging.Info("✅ RegionalNode %s запущен", syncCfg.RegionID)
		}
	}

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
		RestPort: restAddr,
		MariaConfig: auth.MariaConfig{
			Host:     "localhost",
			Port:     3306,
			Database: "blockverse",
			Username: "gameuser",    // Настройте под вашу БД
			Password: "gamepass123", // Настройте под вашу БД
		},
		EntityManager: entityManager,
		UseMariaDB:    false, // Установите true для использования MariaDB

		// Конфигурация хранилища позиций игроков
		PositionStorage: api.PositionStorageConfig{
			Type:             "memory", // "memory" или "mariadb"
			MariaDBDSN:       "gameuser:gamepass123@tcp(localhost:3306)/blockverse",
			FallbackToMemory: true, // Fallback к памяти, если MariaDB недоступна
		},
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

	// Создаем KCP игровой сервер (вместо TCP)
	logging.Debug("Создание KCP игрового сервера...")
	kcpAddr := fmt.Sprintf(":%d", tcpPort) // Используем тот же порт что был для TCP
	gameServer, err := network.NewKCPGameServer(kcpAddr, udpAddr)
	if err != nil {
		logging.Error("❌ Ошибка создания KCP игрового сервера: %v", err)
		log.Fatalf("❌ Ошибка создания KCP игрового сервера: %v", err)
	}

	// Получаем репозиторий позиций из интеграции API
	positionRepo := apiIntegration.GetPositionRepository()
	logging.Info("✅ Инициализирован репозиторий позиций игроков")
	// TODO: В будущем также интегрировать общий userRepo для игры и REST API

	// Устанавливаем репозиторий позиций в игровой сервер
	gameServer.SetPositionRepo(positionRepo)
	logging.Debug("Репозиторий позиций передан в игровой сервер")

	// Запускаем игровой сервер
	logging.Debug("Запуск игрового сервера...")
	gameServer.Start()

	logging.Info("✅ Все сервисы запущены и готовы принимать соединения")
	logging.Info("   🎮 Игровой трафик: KCP %s, UDP %s (fallback)", kcpAddr, udpAddr)
	logging.Info("   🌐 REST API: http://localhost%s", restAddr)
	logging.Info("   🔐 JWT аутентификация активирована")
	logging.Info("   ❤️  Health check: http://localhost%s/health", restAddr)
	logging.Debug("KCP игровой сервер полностью инициализирован и работает")

	// Примеры использования REST API
	logging.Info("💡 Примеры использования REST API:")
	logging.Info("   curl http://localhost%s/health", restAddr)
	logging.Info("   curl -X POST http://localhost%s/api/auth/login -H 'Content-Type: application/json' -d '{\"username\":\"admin\",\"password\":\"ChangeMe123!\"}'", restAddr)

	// Канал для получения сигналов ОС
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logging.Debug("Ожидание сигналов завершения...")

	// Ждем сигнала для завершения
	sig := <-sigCh
	logging.Info("📡 Получен сигнал %v, завершение работы...", sig)

	// === GRACEFUL SHUTDOWN ===
	logging.Debug("Остановка сервисов...")

	// Останавливаем KCP игровой сервер
	logging.Debug("Остановка KCP игрового сервера...")
	gameServer.Stop()

	// Останавливаем REST API
	logging.Debug("Остановка REST API...")
	if err := apiIntegration.Stop(); err != nil {
		logging.Error("❌ Ошибка остановки REST API: %v", err)
	}

	if shutdownTel != nil {
		_ = shutdownTel(context.Background())
	}

	if syncManager != nil {
		syncManager.Stop()
	}

	// Останавливаем региональный узел
	if regionalNode != nil {
		if err := regionalNode.Stop(); err != nil {
			logging.Error("❌ Ошибка остановки RegionalNode: %v", err)
		}
	}

	logging.Info("👋 Сервер успешно остановлен")
}

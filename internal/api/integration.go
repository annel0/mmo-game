package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// ServerIntegration управляет интеграцией REST API с игровым сервером
type ServerIntegration struct {
	restServer    *RestServer
	userRepo      auth.UserRepository
	entityManager *entity.EntityManager
	httpServer    *http.Server
	ctx           context.Context
	cancel        context.CancelFunc
}

// IntegrationConfig содержит конфигурацию для интеграции
type IntegrationConfig struct {
	// REST API настройки
	RestPort string

	// MariaDB настройки
	MariaConfig auth.MariaConfig

	// Менеджер сущностей
	EntityManager *entity.EntityManager

	// Использовать ли MariaDB вместо in-memory репозитория
	UseMariaDB bool
}

// NewServerIntegration создает новую интеграцию REST API с игровым сервером
func NewServerIntegration(config IntegrationConfig) (*ServerIntegration, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Инициализируем репозиторий пользователей
	var userRepo auth.UserRepository
	var err error

	if config.UseMariaDB {
		// Используем MariaDB
		userRepo, err = auth.NewMariaUserRepo(config.MariaConfig)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("не удалось подключиться к MariaDB: %w", err)
		}
		log.Println("✅ MariaDB подключена успешно")
	} else {
		// Используем in-memory репозиторий для разработки/тестирования
		userRepo, err = auth.NewMemoryUserRepo()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("не удалось создать in-memory репозиторий: %w", err)
		}
		log.Println("⚠️  Используется in-memory репозиторий пользователей")
	}

	// Создаем REST сервер
	restServer := NewRestServer(Config{
		Port:          config.RestPort,
		UserRepo:      userRepo,
		EntityManager: config.EntityManager,
	})

	integration := &ServerIntegration{
		restServer:    restServer,
		userRepo:      userRepo,
		entityManager: config.EntityManager,
		ctx:           ctx,
		cancel:        cancel,
	}

	return integration, nil
}

// Start запускает REST API сервер
func (si *ServerIntegration) Start() error {
	log.Printf("Запуск REST API сервера на порту %s", si.restServer.port)

	// Создаем HTTP сервер для graceful shutdown
	si.httpServer = &http.Server{
		Addr:    si.restServer.port,
		Handler: si.restServer.router,
	}

	// Запускаем сервер в отдельной горутине
	go func() {
		if err := si.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Ошибка REST API сервера: %v", err)
		}
	}()

	log.Printf("✅ REST API сервер запущен на http://localhost%s", si.restServer.port)
	log.Printf("📋 Доступные эндпоинты:")
	log.Printf("   GET  /health           - Проверка состояния")
	log.Printf("   POST /api/auth/login   - Вход в систему")
	log.Printf("   GET  /api/stats        - Статистика сервера (требует JWT)")
	log.Printf("   GET  /api/server       - Информация о сервере (требует JWT)")
	log.Printf("   POST /api/admin/register - Регистрация пользователя (только админы)")
	log.Printf("   GET  /api/admin/users  - Список пользователей (только админы)")
	log.Printf("   POST /api/webhook      - Webhook эндпоинт")

	return nil
}

// Stop останавливает REST API сервер
func (si *ServerIntegration) Stop() error {
	log.Println("🛑 Остановка REST API сервера...")

	// Устанавливаем таймаут для graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Останавливаем HTTP сервер
	if si.httpServer != nil {
		if err := si.httpServer.Shutdown(ctx); err != nil {
			log.Printf("❌ Ошибка при остановке HTTP сервера: %v", err)
			return err
		}
	}

	// Закрываем репозиторий пользователей
	if si.userRepo != nil {
		if closer, ok := si.userRepo.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				log.Printf("❌ Ошибка при закрытии репозитория: %v", err)
			}
		}
	}

	// Отменяем контекст
	si.cancel()

	log.Println("✅ REST API сервер остановлен")
	return nil
}

// GetUserRepository возвращает репозиторий пользователей (для использования в игровом сервере)
func (si *ServerIntegration) GetUserRepository() auth.UserRepository {
	return si.userRepo
}

// GetRestServer возвращает REST сервер (для дополнительной настройки)
func (si *ServerIntegration) GetRestServer() *RestServer {
	return si.restServer
}

// GetOutboundWebhooks возвращает менеджер исходящих webhook'ов
func (si *ServerIntegration) GetOutboundWebhooks() *OutboundWebhookManager {
	return si.restServer.outboundWebhooks
}

// IsHealthy проверяет состояние интеграции
func (si *ServerIntegration) IsHealthy() bool {
	// Проверяем, что контекст не отменен
	select {
	case <-si.ctx.Done():
		return false
	default:
	}

	// Проверяем подключение к БД (если MariaDB)
	if mariaRepo, ok := si.userRepo.(*auth.MariaUserRepo); ok {
		// Простая проверка - попытка получить статистику
		_, err := mariaRepo.GetUserStats()
		return err == nil
	}

	return true
}

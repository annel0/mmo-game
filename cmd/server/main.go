package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/network"
)

func main() {
	// === ИНИЦИАЛИЗАЦИЯ ЛОГИРОВАНИЯ ===
	if err := logging.InitLogger(); err != nil {
		log.Fatalf("❌ Ошибка инициализации логгера: %v", err)
	}
	defer logging.CloseLogger()

	logging.LogInfo("🎮 Запуск игрового сервера с поддержкой JWT аутентификации...")

	// === ОБНОВЛЕННЫЕ АДРЕСА ===
	// Новые порты для совместимости с обновленным клиентом
	tcpAddr := ":7777"
	udpAddr := ":7778"

	// Создаем игровой сервер
	server, err := network.NewGameServerPB(tcpAddr, udpAddr)
	if err != nil {
		logging.LogError("❌ Ошибка создания сервера: %v", err)
		log.Fatalf("❌ Ошибка создания сервера: %v", err)
	}

	// Запускаем сервер
	server.Start()
	logging.LogInfo("✅ Сервер запущен и готов принимать соединения")
	logging.LogInfo("   🎮 Игровой трафик: TCP %s, UDP %s", tcpAddr, udpAddr)
	logging.LogInfo("   🔐 JWT аутентификация активирована")

	log.Println("✅ Сервер запущен и готов принимать соединения")
	log.Printf("   🎮 Игровой трафик: TCP %s, UDP %s", tcpAddr, udpAddr)
	log.Printf("   🔐 JWT аутентификация активирована")

	// Канал для получения сигналов ОС
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Ждем сигнала для завершения
	sig := <-sigCh
	logging.LogInfo("📡 Получен сигнал %v, завершение работы...", sig)
	log.Printf("📡 Получен сигнал %v, завершение работы...", sig)

	// Останавливаем сервер
	server.Stop()
	logging.LogInfo("👋 Сервер успешно остановлен")
	log.Println("👋 Сервер успешно остановлен")
}

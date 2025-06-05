package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/annel0/mmo-game/internal/network"
)

func main() {
	log.Println("Запуск игрового сервера с поддержкой Protocol Buffers...")

	// Адреса для TCP и UDP серверов
	tcpAddr := ":9090"
	udpAddr := ":9091"

	// Создаем игровой сервер
	server, err := network.NewGameServerPB(tcpAddr, udpAddr)
	if err != nil {
		log.Fatalf("Ошибка создания сервера: %v", err)
	}

	// Запускаем сервер
	server.Start()
	log.Println("Сервер запущен и готов принимать соединения")

	// Канал для получения сигналов ОС
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Ждем сигнала для завершения
	sig := <-sigCh
	log.Printf("Получен сигнал %v, завершение работы...", sig)

	// Останавливаем сервер
	server.Stop()
	log.Println("Сервер успешно остановлен")
}

package logging

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// LogLevel определяет уровни логирования
type LogLevel int

const (
	TRACE LogLevel = iota
	DEBUG
	INFO
	WARN
	ERROR
)

// String возвращает строковое представление уровня логирования
func (l LogLevel) String() string {
	switch l {
	case TRACE:
		return "TRACE"
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger представляет систему логирования
type Logger struct {
	consoleLogger *log.Logger
	fileLogger    *log.Logger
	file          *os.File
}

// Глобальный экземпляр логгера
var globalLogger *Logger

// InitLogger инициализирует систему логирования
func InitLogger() error {
	// Создаем директорию для логов
	if err := os.MkdirAll("logs", 0755); err != nil {
		return fmt.Errorf("ошибка создания директории logs: %w", err)
	}

	// Создаем файл для логов с временной меткой
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := filepath.Join("logs", fmt.Sprintf("server_%s.log", timestamp))

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("ошибка создания файла логов: %w", err)
	}

	// Создаем логгеры
	consoleLogger := log.New(os.Stdout, "", log.LstdFlags)
	fileLogger := log.New(file, "", log.LstdFlags)

	globalLogger = &Logger{
		consoleLogger: consoleLogger,
		fileLogger:    fileLogger,
		file:          file,
	}

	return nil
}

// CloseLogger закрывает систему логирования
func CloseLogger() {
	if globalLogger != nil && globalLogger.file != nil {
		globalLogger.file.Close()
	}
}

// LogTrace логирует сообщение уровня TRACE
func LogTrace(format string, args ...interface{}) {
	logMessage(TRACE, format, args...)
}

// LogDebug логирует сообщение уровня DEBUG
func LogDebug(format string, args ...interface{}) {
	logMessage(DEBUG, format, args...)
}

// LogInfo логирует сообщение уровня INFO
func LogInfo(format string, args ...interface{}) {
	logMessage(INFO, format, args...)
}

// LogWarn логирует сообщение уровня WARN
func LogWarn(format string, args ...interface{}) {
	logMessage(WARN, format, args...)
}

// LogError логирует сообщение уровня ERROR
func LogError(format string, args ...interface{}) {
	logMessage(ERROR, format, args...)
}

// logMessage внутренняя функция для логирования
func logMessage(level LogLevel, format string, args ...interface{}) {
	if globalLogger == nil {
		return
	}

	message := fmt.Sprintf("[%s] %s", level.String(), fmt.Sprintf(format, args...))

	// Логируем в файл все уровни
	globalLogger.fileLogger.Println(message)

	// Логируем в консоль только INFO и выше
	if level >= INFO {
		globalLogger.consoleLogger.Println(message)
	}
}

// LogMessage логирует детали protobuf сообщения с hex дампом
func LogMessage(connID string, direction string, msgType interface{}, payload []byte) {
	LogDebug("=== %s MESSAGE %s ===", direction, connID)
	LogDebug("Type: %v", msgType)
	LogDebug("Size: %d bytes", len(payload))

	if len(payload) > 0 {
		LogDebug("Hex dump:")
		LogDebug("%s", HexDump(payload))
	}
}

// HexDump создает hex дамп данных
func HexDump(data []byte) string {
	if len(data) == 0 {
		return "No data"
	}

	// Ограничиваем размер дампа до 256 байт
	size := len(data)
	if size > 256 {
		size = 256
	}

	return hex.Dump(data[:size])
}

// LogProtocolError логирует ошибки десериализации протокола
func LogProtocolError(connID string, err error, data []byte) {
	LogError("Protocol error from %s: %v", connID, err)
	if len(data) > 0 {
		LogError("Raw data (%d bytes):", len(data))
		LogError("%s", HexDump(data))
	}
}

// LogEntityMovement логирует движение сущности
func LogEntityMovement(entityID uint64, fromX, fromY, toX, toY float64, direction int) {
	LogTrace("Entity %d movement: (%.2f,%.2f) -> (%.2f,%.2f) dir:%d",
		entityID, fromX, fromY, toX, toY, direction)
}

// LogChunkRequest логирует запрос чанка
func LogChunkRequest(connID string, chunkX, chunkY int) {
	LogDebug("Chunk request from %s: chunk(%d,%d)", connID, chunkX, chunkY)
}

// LogChunkData логирует отправку данных чанка
func LogChunkData(connID string, chunkX, chunkY int, blockCount int) {
	LogDebug("Chunk data sent to %s: chunk(%d,%d) with %d blocks",
		connID, chunkX, chunkY, blockCount)
}

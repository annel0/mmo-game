package logging

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
)

type LogLevel int

const (
	TRACE LogLevel = iota
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[LogLevel]string{
	TRACE: "TRACE",
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

type Logger struct {
	consoleLogger   *log.Logger
	fileLogger      *log.Logger
	fileHandle      *os.File
	minConsoleLevel LogLevel
	minFileLevel    LogLevel
}

// NewLogger создает новый логгер с dual-системой
func NewLogger(component string) (*Logger, error) {
	// Создаем папку logs если не существует
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("ошибка создания папки logs: %w", err)
	}

	// Генерируем имя файла с timestamp
	now := time.Now()
	filename := fmt.Sprintf("logs/%s_%02d-%02d_%02d-%02d-%02d.log",
		component,
		now.Hour(), now.Minute(),
		now.Day(), now.Month(), now.Year()%100)

	// Открываем файл для записи
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия лог-файла %s: %w", filename, err)
	}

	logger := &Logger{
		consoleLogger:   log.New(os.Stdout, "", log.LstdFlags),
		fileLogger:      log.New(file, "", log.LstdFlags|log.Lmicroseconds),
		fileHandle:      file,
		minConsoleLevel: INFO,
		minFileLevel:    TRACE,
	}

	logger.Info("=== %s LOGGING STARTED ===", component)
	logger.Debug("Лог-файл: %s", filename)

	return logger, nil
}

// Close закрывает файловый логгер
func (l *Logger) Close() error {
	if l.fileHandle != nil {
		return l.fileHandle.Close()
	}
	return nil
}

// Базовый метод логирования
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	levelStr := levelNames[level]

	// Логируем в консоль если уровень достаточен
	if level >= l.minConsoleLevel {
		l.consoleLogger.Printf("[%s] %s", levelStr, message)
	}

	// Логируем в файл если уровень достаточен
	if level >= l.minFileLevel {
		l.fileLogger.Printf("[%s] %s", levelStr, message)
	}
}

// Методы для разных уровней логирования
func (l *Logger) Trace(format string, args ...interface{}) {
	l.log(TRACE, format, args...)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
	os.Exit(1)
}

// LogMessage логирует protobuf сообщение с деталями
func (l *Logger) LogMessage(direction string, msgType protocol.MessageType, data []byte, details string) {
	l.Info("%s: %s (%d байт) - %s", direction, msgType.String(), len(data), details)

	// Детальная информация в файл
	l.Debug("=== %s MESSAGE DETAILS ===", direction)
	l.Debug("Type: %s (%d)", msgType.String(), int32(msgType))
	l.Debug("Size: %d bytes", len(data))
	l.Debug("Details: %s", details)

	// Hex-дамп в файл для детального анализа
	if len(data) > 0 {
		l.Debug("Hex dump:")
		l.LogHexDump(data)
	}
	l.Debug("=== END MESSAGE ===")
}

// LogHexDump выводит hex-дамп данных
func (l *Logger) LogHexDump(data []byte) {
	if len(data) == 0 {
		l.Debug("  [empty data]")
		return
	}

	const bytesPerLine = 16
	for i := 0; i < len(data); i += bytesPerLine {
		end := i + bytesPerLine
		if end > len(data) {
			end = len(data)
		}

		// Offset
		line := fmt.Sprintf("  %08x: ", i)

		// Hex bytes
		hexPart := ""
		asciiPart := ""
		for j := i; j < end; j++ {
			hexPart += fmt.Sprintf("%02x ", data[j])
			if data[j] >= 32 && data[j] < 127 {
				asciiPart += string(data[j])
			} else {
				asciiPart += "."
			}
		}

		// Выравнивание
		for j := end; j < i+bytesPerLine; j++ {
			hexPart += "   "
		}

		line += hexPart + " |" + asciiPart + "|"
		l.fileLogger.Println(line)
	}
}

// LogProtocolError специальный метод для логирования ошибок протокола
func (l *Logger) LogProtocolError(operation string, err error, data []byte) {
	l.Error("PROTOCOL ERROR in %s: %v", operation, err)

	if len(data) > 0 {
		l.Debug("=== PROTOCOL ERROR DATA ===")
		l.Debug("Operation: %s", operation)
		l.Debug("Error: %v", err)
		l.Debug("Data size: %d bytes", len(data))
		l.Debug("Raw data:")
		l.LogHexDump(data)
		l.Debug("=== END ERROR DATA ===")
	}
}

// LogEntityMovement логирует движение сущности
func (l *Logger) LogEntityMovement(entityID uint64, x, y, vx, vy float32) {
	l.Debug("ENTITY_MOVE: ID=%d pos=(%.2f,%.2f) vel=(%.2f,%.2f)", entityID, x, y, vx, vy)
}

// LogChunkRequest логирует запрос чанка
func (l *Logger) LogChunkRequest(chunkX, chunkY int32, source string) {
	l.Info("CHUNK_REQUEST: (%d,%d) from %s", chunkX, chunkY, source)
	l.Debug("CHUNK_REQUEST details: chunk=(%d,%d) source=%s", chunkX, chunkY, source)
}

// LogChunkData логирует данные чанка
func (l *Logger) LogChunkData(chunkX, chunkY int32, blockCount int, entityCount int, target string) {
	l.Info("CHUNK_DATA: (%d,%d) %d blocks, %d entities -> %s", chunkX, chunkY, blockCount, entityCount, target)
	l.Debug("CHUNK_DATA details: chunk=(%d,%d) blocks=%d entities=%d target=%s", chunkX, chunkY, blockCount, entityCount, target)
}

// Глобальный логгер по умолчанию
var defaultLogger *Logger

// InitDefaultLogger инициализирует глобальный логгер
func InitDefaultLogger(component string) error {
	var err error
	defaultLogger, err = NewLogger(component)
	return err
}

// GetDefaultLogger возвращает глобальный логгер
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// CloseDefaultLogger закрывает глобальный логгер
func CloseDefaultLogger() error {
	if defaultLogger != nil {
		return defaultLogger.Close()
	}
	return nil
}

// Глобальные методы для удобства
func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, args...)
	}
}

func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, args...)
	}
}

func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, args...)
	}
}

func LogMessage(direction string, msgType protocol.MessageType, data []byte, details string) {
	if defaultLogger != nil {
		defaultLogger.LogMessage(direction, msgType, data, details)
	}
}

func LogProtocolError(operation string, err error, data []byte) {
	if defaultLogger != nil {
		defaultLogger.LogProtocolError(operation, err, data)
	}
}

func LogChunkRequest(chunkX, chunkY int32, source string) {
	if defaultLogger != nil {
		defaultLogger.LogChunkRequest(chunkX, chunkY, source)
	}
}

func LogChunkData(chunkX, chunkY int32, blockCount int, entityCount int, target string) {
	if defaultLogger != nil {
		defaultLogger.LogChunkData(chunkX, chunkY, blockCount, entityCount, target)
	}
}

func LogEntityMovement(entityID uint64, x, y, vx, vy float32) {
	if defaultLogger != nil {
		defaultLogger.LogEntityMovement(entityID, x, y, vx, vy)
	}
}

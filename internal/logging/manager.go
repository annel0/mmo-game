package logging

import (
	"fmt"
	"sync"
)

// LoggerManager управляет множественными логгерами для разных компонентов
type LoggerManager struct {
	mu      sync.RWMutex
	loggers map[string]*Logger
}

var (
	globalManager *LoggerManager
	managerOnce   sync.Once
)

// GetLoggerManager возвращает глобальный менеджер логгеров
func GetLoggerManager() *LoggerManager {
	managerOnce.Do(func() {
		globalManager = &LoggerManager{
			loggers: make(map[string]*Logger),
		}
	})
	return globalManager
}

// GetLogger возвращает логгер для компонента, создавая его при необходимости
func (lm *LoggerManager) GetLogger(component string) (*Logger, error) {
	lm.mu.RLock()
	if logger, exists := lm.loggers[component]; exists {
		lm.mu.RUnlock()
		return logger, nil
	}
	lm.mu.RUnlock()

	// Создаем новый логгер под write lock
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Проверяем еще раз на случай race condition
	if logger, exists := lm.loggers[component]; exists {
		return logger, nil
	}

	logger, err := NewLogger(component)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger for %s: %w", component, err)
	}

	lm.loggers[component] = logger
	return logger, nil
}

// MustGetLogger возвращает логгер или создает fallback при ошибке
func (lm *LoggerManager) MustGetLogger(component string) *Logger {
	logger, err := lm.GetLogger(component)
	if err != nil {
		// Fallback: создаем простой логгер в stdout
		return &Logger{
			consoleLogger:   defaultLogger.consoleLogger,
			minConsoleLevel: INFO,
			minFileLevel:    ERROR,
		}
	}
	return logger
}

// CloseAll закрывает все логгеры
func (lm *LoggerManager) CloseAll() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var lastErr error
	for component, logger := range lm.loggers {
		if err := logger.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close logger for %s: %w", component, err)
		}
	}

	// Очищаем карту
	lm.loggers = make(map[string]*Logger)
	return lastErr
}

// ListComponents возвращает список всех зарегистрированных компонентов
func (lm *LoggerManager) ListComponents() []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	components := make([]string, 0, len(lm.loggers))
	for component := range lm.loggers {
		components = append(components, component)
	}
	return components
}

// SetLogLevel устанавливает уровень логирования для компонента
func (lm *LoggerManager) SetLogLevel(component string, consoleLevel, fileLevel LogLevel) error {
	lm.mu.RLock()
	logger, exists := lm.loggers[component]
	lm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("logger for component %s not found", component)
	}

	logger.minConsoleLevel = consoleLevel
	logger.minFileLevel = fileLevel
	return nil
}

// Удобные функции для получения логгеров
func GetComponentLogger(component string) *Logger {
	return GetLoggerManager().MustGetLogger(component)
}

func GetNetworkLogger() *Logger {
	return GetComponentLogger("network")
}

func GetServerLogger() *Logger {
	return GetComponentLogger("server")
}

func GetGameLogger() *Logger {
	return GetComponentLogger("game")
}

func GetRegionalLogger() *Logger {
	return GetComponentLogger("regional")
}

func GetSyncLogger() *Logger {
	return GetComponentLogger("sync")
}

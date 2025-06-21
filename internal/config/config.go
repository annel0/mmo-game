package config

import (
	"io/ioutil"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config корневая структура конфигурации приложения.
// Пока содержит только EventBus; может расширяться.

type Config struct {
	EventBus EventBusConfig `yaml:"eventbus"`
	Sync     SyncConfig     `yaml:"sync"`
	Server   ServerConfig   `yaml:"server"`
}

type EventBusConfig struct {
	URL       string `yaml:"url"`
	Stream    string `yaml:"stream"`
	Retention int    `yaml:"retention_hours"`
}

type SyncConfig struct {
	RegionID     string `yaml:"region_id"`
	BatchSize    int    `yaml:"batch_size"`
	FlushEvery   int    `yaml:"flush_every_seconds"`
	UseGzipCompr bool   `yaml:"use_gzip_compression"`
}

type ServerConfig struct {
	TCPPort     int `yaml:"tcp_port"`
	UDPPort     int `yaml:"udp_port"`
	RESTPort    int `yaml:"rest_port"`
	MetricsPort int `yaml:"metrics_port"`
}

// GetTCPPort возвращает TCP порт с поддержкой fallback значений
func (s *ServerConfig) GetTCPPort() int {
	return getPortWithEnvFallback(s.TCPPort, "GAME_TCP_PORT", 7777)
}

// GetUDPPort возвращает UDP порт с поддержкой fallback значений
func (s *ServerConfig) GetUDPPort() int {
	return getPortWithEnvFallback(s.UDPPort, "GAME_UDP_PORT", 7778)
}

// GetRESTPort возвращает REST API порт с поддержкой fallback значений
func (s *ServerConfig) GetRESTPort() int {
	return getPortWithEnvFallback(s.RESTPort, "GAME_REST_PORT", 8088)
}

// GetMetricsPort возвращает Prometheus метрики порт с поддержкой fallback значений
func (s *ServerConfig) GetMetricsPort() int {
	return getPortWithEnvFallback(s.MetricsPort, "GAME_METRICS_PORT", 2112)
}

// getPortWithEnvFallback возвращает порт с приоритетом: config -> env -> default
func getPortWithEnvFallback(configPort int, envVar string, defaultPort int) int {
	// Если порт задан в конфиге и больше 0, используем его
	if configPort > 0 {
		return configPort
	}

	// Пробуем прочитать из environment variable
	if envVal := os.Getenv(envVar); envVal != "" {
		if port, err := strconv.Atoi(envVal); err == nil && port > 0 {
			return port
		}
	}

	// Используем дефолтное значение
	return defaultPort
}

// Load читает YAML файл конфигурации.
// Если path == "", пытается прочитать из ENV GAME_CONFIG или возвращает nil, nil.
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("GAME_CONFIG")
		if path == "" {
			return nil, nil // конфиг не задан — использовать дефолты
		}
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

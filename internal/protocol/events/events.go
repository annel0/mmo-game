// Package events содержит типы и утилиты для работы с событиями
package events

// EventType представляет тип события
type EventType string

const (
	// EventTypeSystem - системные события
	EventTypeSystem EventType = "system"
	// EventTypeWorld - события мира
	EventTypeWorld EventType = "world"
	// EventTypeBlock - события блоков
	EventTypeBlock EventType = "block"
	// EventTypeChat - события чата
	EventTypeChat EventType = "chat"
)

// Event представляет базовое событие
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

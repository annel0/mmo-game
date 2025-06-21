// Package replay содержит типы и утилиты для воспроизведения событий
package replay

import (
	"time"

	"github.com/annel0/mmo-game/internal/protocol/events"
)

// ReplaySession представляет сессию воспроизведения
type ReplaySession struct {
	ID        string            `json:"id"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time"`
	Events    []events.Event    `json:"events"`
	Metadata  map[string]string `json:"metadata"`
}

// ReplayFilter определяет фильтры для воспроизведения
type ReplayFilter struct {
	EventTypes []events.EventType `json:"event_types"`
	StartTime  *time.Time         `json:"start_time,omitempty"`
	EndTime    *time.Time         `json:"end_time,omitempty"`
	Region     string             `json:"region,omitempty"`
	PlayerID   uint64             `json:"player_id,omitempty"`
}

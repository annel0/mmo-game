package regional

import (
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/sync"
)

// Conflict представляет конфликт между локальными и удалёнными изменениями
type Conflict struct {
	LocalChange  *sync.Change // Локальное изменение
	RemoteChange *sync.Change // Удалённое изменение
	DetectedAt   time.Time    // Время обнаружения конфликта
}

// ConflictResolver интерфейс для разрешения конфликтов между региональными узлами
type ConflictResolver interface {
	// Resolve разрешает конфликт и возвращает финальное изменение
	Resolve(conflict *Conflict) (*sync.Change, error)
}

// LWWResolver реализует Last-Write-Wins стратегию разрешения конфликтов
type LWWResolver struct{}

// NewLWWResolver создаёт новый Last-Write-Wins resolver
func NewLWWResolver() ConflictResolver {
	return &LWWResolver{}
}

// Resolve реализует ConflictResolver для LWW стратегии
func (r *LWWResolver) Resolve(conflict *Conflict) (*sync.Change, error) {
	logging.Debug("LWW Resolver: разрешение конфликта между local и remote изменениями")

	// Last-Write-Wins: выбираем изменение с более поздним timestamp
	localTime := conflict.LocalChange.Timestamp
	remoteTime := conflict.RemoteChange.Timestamp

	if remoteTime.After(localTime) {
		logging.Debug("LWW Resolver: выбираем remote изменение (newer)")
		return conflict.RemoteChange, nil
	} else {
		logging.Debug("LWW Resolver: выбираем local изменение (newer)")
		return conflict.LocalChange, nil
	}
}

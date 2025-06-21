package network

import (
	"log"

	"github.com/annel0/mmo-game/internal/protocol"
)

// createMessageSerializer создаёт новый MessageSerializer для обратной совместимости
func createMessageSerializer() *protocol.MessageSerializer {
	serializer, err := protocol.NewMessageSerializer()
	if err != nil {
		log.Printf("❌ Ошибка создания MessageSerializer: %v", err)
		// Возвращаем nil - старый код должен обработать этот случай
		return nil
	}
	return serializer
}

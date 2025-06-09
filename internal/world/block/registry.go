package block

var registry = make(map[BlockID]BlockBehavior)

// Register добавляет поведение блока в регистр
func Register(id BlockID, behavior BlockBehavior) {
	registry[id] = behavior
}

// Get возвращает поведение для указанного ID
func Get(id BlockID) (BlockBehavior, bool) {
	behavior, exists := registry[id]
	return behavior, exists
}

// IsValidBlockID проверяет, является ли ID допустимым идентификатором блока
func IsValidBlockID(id BlockID) bool {
	_, exists := registry[id]
	return exists
}

// BlockID представляет идентификатор блока
type BlockID uint16

// Константы ID блоков
const (
	// Базовые типы блоков
	AirBlockID   BlockID = iota // 0
	StoneBlockID                // 1
	GrassBlockID                // 2
	WaterBlockID                // 3
	SandBlockID                 // 4
	DirtBlockID                 // 5 - Новый блок земли/грязи

	// Для возможности расширения, оставляем большие промежутки между категориями

	// Декоративные блоки (начиная с 100)
	FlowerBlockID BlockID = 100 // Цветок
	TreeBlockID   BlockID = 101 // Дерево
	CactusBlockID BlockID = 102 // Кактус, 2-слойный

	// Интерактивные блоки (начиная с 200)
	ChestBlockID BlockID = 200 // Сундук
	DoorBlockID  BlockID = 201 // Дверь

	// Специальные блоки (начиная с 1000)
	PortalBlockID  BlockID = 1000 // Портал
	SpawnerBlockID BlockID = 1001 // Спаунер
)

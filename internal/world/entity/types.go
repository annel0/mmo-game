package entity

// Player представляет игрока в игре
type Player struct {
	Entity
	Name string
}

// NPC представляет неигрового персонажа
type NPC struct {
	Entity
	DialogueID string
}

// Animal представляет животное
type Animal struct {
	Entity
	Species string
}

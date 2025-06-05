package physics

import (
	"github.com/annel0/mmo-game/internal/vec"
)

// BoxCollider представляет простой прямоугольный коллайдер
type BoxCollider struct {
	Width  int // Ширина в блоках
	Height int // Высота в блоках
}

// NewBoxCollider создаёт новый коллайдер с указанными размерами
func NewBoxCollider(width, height int) *BoxCollider {
	return &BoxCollider{
		Width:  width,
		Height: height,
	}
}

// IsPointInside проверяет, находится ли точка внутри коллайдера
func (bc *BoxCollider) IsPointInside(colliderPos, point vec.Vec2) bool {
	halfWidth := bc.Width / 2
	halfHeight := bc.Height / 2

	return point.X >= colliderPos.X-halfWidth &&
		point.X < colliderPos.X+halfWidth &&
		point.Y >= colliderPos.Y-halfHeight &&
		point.Y < colliderPos.Y+halfHeight
}

// CheckCollision проверяет столкновение двух коллайдеров
func CheckBoxCollision(pos1 vec.Vec2, collider1 *BoxCollider, pos2 vec.Vec2, collider2 *BoxCollider) bool {
	halfWidth1 := collider1.Width / 2
	halfHeight1 := collider1.Height / 2
	halfWidth2 := collider2.Width / 2
	halfHeight2 := collider2.Height / 2

	return pos1.X+halfWidth1 > pos2.X-halfWidth2 &&
		pos1.X-halfWidth1 < pos2.X+halfWidth2 &&
		pos1.Y+halfHeight1 > pos2.Y-halfHeight2 &&
		pos1.Y-halfHeight1 < pos2.Y+halfHeight2
}

// GetCollisionPoints возвращает точки для проверки коллизий с блоками
// Например, для сущности 2x2 блока вернёт 4 точки (углы)
func GetCollisionPoints(pos vec.Vec2, collider *BoxCollider) []vec.Vec2 {
	halfWidth := collider.Width / 2
	halfHeight := collider.Height / 2

	// Для коллайдера 1x1 вернём только центральную точку
	if collider.Width <= 1 && collider.Height <= 1 {
		return []vec.Vec2{pos}
	}

	// Для больших коллайдеров вернём 4 угла и центр
	points := []vec.Vec2{
		{X: pos.X - halfWidth, Y: pos.Y - halfHeight},         // Левый верхний
		{X: pos.X + halfWidth - 1, Y: pos.Y - halfHeight},     // Правый верхний
		{X: pos.X - halfWidth, Y: pos.Y + halfHeight - 1},     // Левый нижний
		{X: pos.X + halfWidth - 1, Y: pos.Y + halfHeight - 1}, // Правый нижний
		{X: pos.X, Y: pos.Y},                                  // Центр
	}

	return points
}

// CanMoveToPosition проверяет, может ли сущность с указанным коллайдером переместиться в указанную позицию
// blockChecker - функция, которая проверяет, является ли блок в указанной позиции проходимым
func CanMoveToPosition(newPos vec.Vec2, collider *BoxCollider, blockChecker func(vec.Vec2) bool) bool {
	// Получаем точки для проверки коллизий
	points := GetCollisionPoints(newPos, collider)

	// Проверяем каждую точку
	for _, point := range points {
		if !blockChecker(point) {
			// Если хотя бы одна точка находится в непроходимом блоке, движение невозможно
			return false
		}
	}

	return true
}

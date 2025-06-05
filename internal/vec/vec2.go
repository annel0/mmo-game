package vec

import "math"

// Vec2 представляет 2D координаты
type Vec2 struct {
    X, Y int
}

// ToChunkCoords преобразует глобальные координаты в координаты чанка
func (v Vec2) ToChunkCoords() Vec2 {
    return Vec2{X: v.X >> 4, Y: v.Y >> 4} // Деление на 16
}

// ToBigChunkCoords преобразует в координаты BigChunk
func (v Vec2) ToBigChunkCoords() Vec2 {
    return Vec2{X: v.X >> 9, Y: v.Y >> 9} // Деление на 512 (16*32)
}

// LocalInChunk возвращает локальные координаты внутри чанка
func (v Vec2) LocalInChunk() Vec2 {
    return Vec2{X: v.X & 0xF, Y: v.Y & 0xF} // Модуль 16
}

// DistanceTo вычисляет расстояние до другой точки
func (v Vec2) DistanceTo(other Vec2) float64 {
    dx := float64(v.X - other.X)
    dy := float64(v.Y - other.Y)
    return math.Sqrt(dx*dx + dy*dy)
}

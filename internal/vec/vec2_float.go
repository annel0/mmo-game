package vec

import "math"

// Vec2Float представляет 2D координаты с плавающей точкой
type Vec2Float struct {
	X, Y float64
}

// ToVec2 преобразует в целочисленные координаты
func (v Vec2Float) ToVec2() Vec2 {
	return Vec2{X: int(v.X), Y: int(v.Y)}
}

// FromVec2 создает Vec2Float из Vec2
func FromVec2(v Vec2) Vec2Float {
	return Vec2Float{X: float64(v.X), Y: float64(v.Y)}
}

// Add складывает два вектора
func (v Vec2Float) Add(other Vec2Float) Vec2Float {
	return Vec2Float{X: v.X + other.X, Y: v.Y + other.Y}
}

// Sub вычитает вектор
func (v Vec2Float) Sub(other Vec2Float) Vec2Float {
	return Vec2Float{X: v.X - other.X, Y: v.Y - other.Y}
}

// Mul умножает вектор на скаляр
func (v Vec2Float) Mul(scalar float64) Vec2Float {
	return Vec2Float{X: v.X * scalar, Y: v.Y * scalar}
}

// Normalized возвращает нормализованный вектор
func (v Vec2Float) Normalized() Vec2Float {
	length := v.Length()
	if length == 0 {
		return Vec2Float{X: 0, Y: 0}
	}
	return Vec2Float{X: v.X / length, Y: v.Y / length}
}

// Length возвращает длину вектора
func (v Vec2Float) Length() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}

// DistanceTo вычисляет расстояние до другой точки
func (v Vec2Float) DistanceTo(other Vec2Float) float64 {
	dx := v.X - other.X
	dy := v.Y - other.Y
	return math.Sqrt(dx*dx + dy*dy)
}

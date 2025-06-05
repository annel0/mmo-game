package vec

// Vec3 представляет трехмерный вектор с целочисленными координатами
type Vec3 struct {
	X int
	Y int
	Z int
}

// Vec3Float представляет трехмерный вектор с плавающими координатами
type Vec3Float struct {
	X float64
	Y float64
	Z float64
}

// ToVec2 преобразует Vec3 в Vec2, игнорируя координату Z
func (v Vec3) ToVec2() Vec2 {
	return Vec2{
		X: v.X,
		Y: v.Y,
	}
}

// FromVec2 создает Vec3 из Vec2, используя заданную Z координату
// func FromVec2(v Vec2, z int) Vec3 {
// 	return Vec3{
// 		X: v.X,
// 		Y: v.Y,
// 		Z: z,
// 	}
// }

// DistanceTo возвращает расстояние до другого вектора
func (v Vec3) DistanceTo(other Vec3) float64 {
	dx := v.X - other.X
	dy := v.Y - other.Y
	dz := v.Z - other.Z
	return float64(dx*dx + dy*dy + dz*dz)
}

// Equals проверяет равенство векторов
func (v Vec3) Equals(other Vec3) bool {
	return v.X == other.X && v.Y == other.Y && v.Z == other.Z
}

// Add складывает два вектора
func (v Vec3) Add(other Vec3) Vec3 {
	return Vec3{
		X: v.X + other.X,
		Y: v.Y + other.Y,
		Z: v.Z + other.Z,
	}
}
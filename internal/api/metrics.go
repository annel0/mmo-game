package api

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/process"
)

// ServerMetrics содержит метрики сервера
type ServerMetrics struct {
	StartTime time.Time
}

// NewServerMetrics создает новый экземпляр метрик
func NewServerMetrics() *ServerMetrics {
	return &ServerMetrics{
		StartTime: time.Now(),
	}
}

// GetUptime возвращает время работы сервера
func (sm *ServerMetrics) GetUptime() string {
	uptime := time.Since(sm.StartTime)

	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60
	seconds := int(uptime.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dд %dч %dм %dс", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("%dч %dм %dс", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dм %dс", minutes, seconds)
	} else {
		return fmt.Sprintf("%dс", seconds)
	}
}

// GetMemoryUsage возвращает использование памяти в MB
func (sm *ServerMetrics) GetMemoryUsage() (float64, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Преобразуем байты в мегабайты
	memoryMB := float64(m.Alloc) / 1024 / 1024
	return memoryMB, nil
}

// GetCPUUsage возвращает использование CPU процессом в процентах
func (sm *ServerMetrics) GetCPUUsage() (float64, error) {
	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		return 0, err
	}

	// Получаем процент использования CPU за последний интервал
	cpuPercent, err := proc.CPUPercent()
	if err != nil {
		// Если не удалось получить метрику процесса, попробуем системную
		cpuPercents, err := cpu.Percent(100*time.Millisecond, false)
		if err != nil || len(cpuPercents) == 0 {
			return 0, err
		}
		return cpuPercents[0], nil
	}

	return cpuPercent, nil
}

// GetSystemCPUUsage возвращает общее использование CPU системы
func (sm *ServerMetrics) GetSystemCPUUsage() (float64, error) {
	cpuPercents, err := cpu.Percent(time.Second, false)
	if err != nil || len(cpuPercents) == 0 {
		return 0, err
	}
	return cpuPercents[0], nil
}

// GetDetailedMemoryStats возвращает детальную статистику памяти
func (sm *ServerMetrics) GetDetailedMemoryStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"alloc_mb":       float64(m.Alloc) / 1024 / 1024,
		"total_alloc_mb": float64(m.TotalAlloc) / 1024 / 1024,
		"sys_mb":         float64(m.Sys) / 1024 / 1024,
		"heap_alloc_mb":  float64(m.HeapAlloc) / 1024 / 1024,
		"heap_sys_mb":    float64(m.HeapSys) / 1024 / 1024,
		"num_gc":         m.NumGC,
		"goroutines":     runtime.NumGoroutine(),
	}
}

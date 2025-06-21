package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusMiddleware_BasicMetrics(t *testing.T) {
	// Создаём новый регистр для изоляции тестов
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	gin.SetMode(gin.TestMode)
	r := gin.New()

	promMw := middleware.NewPrometheusMiddleware("test")
	r.Use(promMw.Handler())

	r.GET("/test", func(c *gin.Context) {
		time.Sleep(10 * time.Millisecond) // Симулируем задержку
		c.JSON(200, gin.H{"ok": true})
	})

	r.GET("/error", func(c *gin.Context) {
		c.JSON(500, gin.H{"error": "test error"})
	})

	// Выполняем успешный запрос
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// Выполняем запрос с ошибкой
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/error", nil)
	r.ServeHTTP(w2, req2)
	assert.Equal(t, 500, w2.Code)

	// Проверяем метрики
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Ищем наши метрики
	var durationFound, errorsFound bool
	for _, mf := range metricFamilies {
		switch *mf.Name {
		case "test_http_request_duration_seconds":
			durationFound = true
			assert.Equal(t, "Длительность HTTP-запросов.", *mf.Help)
			// Должно быть 2 запроса
			assert.Len(t, mf.Metric, 2)
		case "test_http_request_errors_total":
			errorsFound = true
			// Должна быть 1 ошибка (500 статус)
			assert.Len(t, mf.Metric, 1)
			assert.Equal(t, float64(1), *mf.Metric[0].Counter.Value)
		}
	}

	assert.True(t, durationFound, "Duration metric not found")
	assert.True(t, errorsFound, "Errors metric not found")
}

func TestPrometheusMiddleware_InflightRequests(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	gin.SetMode(gin.TestMode)
	r := gin.New()

	promMw := middleware.NewPrometheusMiddleware("test")
	r.Use(promMw.Handler())

	// Endpoint с искусственной задержкой
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(50 * time.Millisecond)
		c.JSON(200, gin.H{"ok": true})
	})

	// Запускаем запрос в горутине
	done := make(chan bool)
	go func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/slow", nil)
		r.ServeHTTP(w, req)
		done <- true
	}()

	// Небольшая пауза, чтобы middleware зарегистрировал inflight запрос
	time.Sleep(10 * time.Millisecond)

	// Проверяем inflight метрику
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	var inflightFound bool
	for _, mf := range metricFamilies {
		if *mf.Name == "test_http_requests_inflight" {
			inflightFound = true
			// Должен быть 1 активный запрос
			assert.Equal(t, float64(1), *mf.Metric[0].Gauge.Value)
			break
		}
	}

	assert.True(t, inflightFound, "Inflight metric not found")

	// Ждём завершения запроса
	<-done

	// Проверяем что inflight сбросился
	time.Sleep(10 * time.Millisecond)
	metricFamilies, err = registry.Gather()
	require.NoError(t, err)

	for _, mf := range metricFamilies {
		if *mf.Name == "test_http_requests_inflight" {
			assert.Equal(t, float64(0), *mf.Metric[0].Gauge.Value)
			break
		}
	}
}

func TestRequestLogger_TraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	loggerMw := middleware.NewRequestLogger()
	r.Use(loggerMw.Handler())

	var capturedTraceID string
	r.GET("/test", func(c *gin.Context) {
		traceID, exists := c.Get("trace_id")
		require.True(t, exists, "trace_id should be set in context")
		capturedTraceID = traceID.(string)
		c.JSON(200, gin.H{"trace_id": capturedTraceID})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.NotEmpty(t, capturedTraceID, "trace_id should not be empty")

	// Проверяем что trace_id в ответе
	assert.Contains(t, w.Body.String(), capturedTraceID)
}

func TestRequestLogger_LogFormat(t *testing.T) {
	// Для этого теста нужно было бы перехватывать логи
	// Пока оставляем как заглушку для будущей реализации
	t.Skip("Log format testing requires log capture mechanism")
}

func TestMiddleware_Integration(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Интегрируем оба middleware
	loggerMw := middleware.NewRequestLogger()
	r.Use(loggerMw.Handler())

	promMw := middleware.NewPrometheusMiddleware("integration_test")
	r.Use(promMw.Handler())

	r.GET("/api/v1/test", func(c *gin.Context) {
		traceID, _ := c.Get("trace_id")
		c.JSON(200, gin.H{
			"status":   "ok",
			"trace_id": traceID,
		})
	})

	// Выполняем несколько запросов
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
	}

	// Проверяем что метрики собираются
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	var requestsCount int
	for _, mf := range metricFamilies {
		if *mf.Name == "integration_test_http_request_duration_seconds" {
			for _, metric := range mf.Metric {
				requestsCount += int(*metric.Histogram.SampleCount)
			}
		}
	}

	assert.Equal(t, 5, requestsCount, "Should have recorded 5 requests")
}

func TestPrometheusMiddleware_MetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	promMw := middleware.NewPrometheusMiddleware("test")
	r.Use(promMw.Handler())
	promMw.RegisterMetricsEndpoint(r)

	r.GET("/api/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Выполняем обычный запрос для генерации метрик
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/api/test", nil)
	r.ServeHTTP(w1, req1)
	assert.Equal(t, 200, w1.Code)

	// Проверяем endpoint метрик
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	assert.Contains(t, w2.Header().Get("Content-Type"), "text/plain")

	body := w2.Body.String()
	// Проверяем что endpoint метрик работает и возвращает данные
	assert.Greater(t, len(body), 0, "Metrics endpoint should return data")
	assert.Contains(t, body, "# HELP", "Should contain Prometheus help text")
}

func TestPrometheusMiddleware_ErrorCounting(t *testing.T) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	gin.SetMode(gin.TestMode)
	r := gin.New()

	promMw := middleware.NewPrometheusMiddleware("error_test")
	r.Use(promMw.Handler())

	r.GET("/400", func(c *gin.Context) { c.JSON(400, gin.H{"error": "bad request"}) })
	r.GET("/401", func(c *gin.Context) { c.JSON(401, gin.H{"error": "unauthorized"}) })
	r.GET("/404", func(c *gin.Context) { c.JSON(404, gin.H{"error": "not found"}) })
	r.GET("/500", func(c *gin.Context) { c.JSON(500, gin.H{"error": "internal error"}) })
	r.GET("/200", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// Выполняем запросы с разными статусами
	endpoints := []string{"/400", "/401", "/404", "/500", "/200", "/200"}
	for _, endpoint := range endpoints {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", endpoint, nil)
		r.ServeHTTP(w, req)
	}

	// Проверяем метрики ошибок
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	var totalErrors float64
	for _, mf := range metricFamilies {
		if *mf.Name == "error_test_http_request_errors_total" {
			for _, metric := range mf.Metric {
				totalErrors += *metric.Counter.Value
			}
		}
	}

	// Должно быть 4 ошибки (400, 401, 404, 500)
	assert.Equal(t, float64(4), totalErrors)
}

// BenchmarkPrometheusMiddleware измеряет overhead middleware
func BenchmarkPrometheusMiddleware(b *testing.B) {
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	promMw := middleware.NewPrometheusMiddleware("bench")
	r.Use(promMw.Handler())

	r.GET("/bench", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/bench", nil)
			r.ServeHTTP(w, req)
		}
	})
}

// BenchmarkRequestLogger измеряет overhead логирования
func BenchmarkRequestLogger(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	loggerMw := middleware.NewRequestLogger()
	r.Use(loggerMw.Handler())

	r.GET("/bench", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/bench", nil)
			r.ServeHTTP(w, req)
		}
	})
}

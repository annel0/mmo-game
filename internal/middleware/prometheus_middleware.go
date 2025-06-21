package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMiddleware регистрирует базовые HTTP-метрики для Gin.
// Маршрут /metrics добавляется отдельно с помощью метода RegisterMetricsEndpoint.
// Использование:
//   mw := middleware.NewPrometheusMiddleware("rest_api")
//   r.Use(mw.Handler())
//   mw.RegisterMetricsEndpoint(r)
//
// Метрики:
// * http_request_duration_seconds{method,path,status} — histogram
// * http_requests_inflight — gauge
// * http_request_errors_total{method,path,status} — counter (4xx/5xx)

type PrometheusMiddleware struct {
	reqDuration *prometheus.HistogramVec
	reqInflight prometheus.Gauge
	reqErrors   *prometheus.CounterVec
}

// NewPrometheusMiddleware создаёт middleware и регистрирует метрики в дефолтном регистре.
func NewPrometheusMiddleware(service string) *PrometheusMiddleware {
	pm := &PrometheusMiddleware{
		reqDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: service,
			Name:      "http_request_duration_seconds",
			Help:      "Длительность HTTP-запросов.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"method", "path", "status"}),
		reqInflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: service,
			Name:      "http_requests_inflight",
			Help:      "Текущее количество обрабатываемых HTTP-запросов.",
		}),
		reqErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: service,
			Name:      "http_request_errors_total",
			Help:      "Общее число запросов, завершившихся ошибкой (4xx/5xx).",
		}, []string{"method", "path", "status"}),
	}

	prometheus.MustRegister(pm.reqDuration, pm.reqInflight, pm.reqErrors)
	return pm
}

// Handler возвращает gin.HandlerFunc, которую нужно добавить через router.Use().
func (pm *PrometheusMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		pm.reqInflight.Inc()
		// Обработать запрос.
		c.Next()
		pm.reqInflight.Dec()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path // для не-матченных маршрутов
		}
		method := c.Request.Method

		pm.reqDuration.WithLabelValues(method, path, status).Observe(duration)

		// Ошибочные статусы >=400
		if c.Writer.Status() >= 400 {
			pm.reqErrors.WithLabelValues(method, path, status).Inc()
		}
	}
}

// RegisterMetricsEndpoint добавляет GET /metrics в указанный router.
func (pm *PrometheusMiddleware) RegisterMetricsEndpoint(r *gin.Engine) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

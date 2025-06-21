package eventbus

import (
	"net/http"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsExporter инкапсулирует Prometheus-метрики для EventBus и периодически обновляет их.
// Экспортер не делает предположений о конкретной реализации шины –
// он опирается исключительно на интерфейс StatsProvider.
// Интерфейс объявлен ниже, чтобы избежать жёсткой зависимости от private-type методов.

// MetricsExporter управляет HTTP-эндпоинтом Prometheus и периодически обновляет Gauge/Counter.
type MetricsExporter struct {
	bus  EventBus
	quit chan struct{}
	done chan struct{}
	// Prometheus metrics
	published prometheus.Counter
	consumed  prometheus.Counter
	dropped   prometheus.Counter
	inflight  prometheus.Gauge
}

// NewMetricsExporter создаёт экспортер, но не запускает HTTP-сервер.
func NewMetricsExporter(bus EventBus) *MetricsExporter {
	me := &MetricsExporter{
		bus:  bus,
		quit: make(chan struct{}),
		done: make(chan struct{}),
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "eventbus",
			Name:      "messages_published_total",
			Help:      "Общее число опубликованных сообщений.",
		}),
		consumed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "eventbus",
			Name:      "messages_consumed_total",
			Help:      "Общее число доставленных сообщений подписчикам.",
		}),
		dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "eventbus",
			Name:      "messages_dropped_total",
			Help:      "Сообщений, отброшенных из-за ошибок или ограничения back-pressure.",
		}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "eventbus",
			Name:      "messages_inflight",
			Help:      "Количество сообщений, находящихся в очереди (не доставленных).",
		}),
	}

	// Регистрируем метрики в глобальном регистре Prometheus.
	prometheus.MustRegister(me.published, me.consumed, me.dropped, me.inflight)
	return me
}

// StartHTTP запускает HTTP-эндпоинт Prometheus на указанном адресе (например, ":2112").
// Метод неблокирующий: HTTP-сервер стартует в отдельной горутине.
func (m *MetricsExporter) StartHTTP(addr string) {
	go func() {
		logging.Info("📈 Prometheus /metrics доступен по адресу %s", addr)
		if err := http.ListenAndServe(addr, promhttp.Handler()); err != nil {
			logging.Error("Ошибка Prometheus HTTP сервера: %v", err)
		}
	}()
	go m.loop()
}

// Stop останавливает обновление метрик. HTTP-сервер при этом не завершается
// (для упрощения – можно запустить на отдельном порте и убить процесс целиком).
func (m *MetricsExporter) Stop() {
	close(m.quit)
	<-m.done
}

func (m *MetricsExporter) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer close(m.done)

	// Для коррекции Counter нужно хранить прошлое значение и прибавлять дельту.
	var prev Stats

	for {
		select {
		case <-ticker.C:
			// Пытаемся получить Stats.
			stats := m.bus.Metrics()

			// Вычисляем приращение и обновляем counters.
			deltaPub := stats.Published - prev.Published
			deltaCons := stats.Consumed - prev.Consumed
			deltaDrop := stats.Dropped - prev.Dropped

			if deltaPub > 0 {
				m.published.Add(float64(deltaPub))
			}
			if deltaCons > 0 {
				m.consumed.Add(float64(deltaCons))
			}
			if deltaDrop > 0 {
				m.dropped.Add(float64(deltaDrop))
			}

			m.inflight.Set(float64(stats.InFlight))

			prev = stats
		case <-m.quit:
			return
		}
	}
}

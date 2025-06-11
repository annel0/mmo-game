package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// OutboundWebhook представляет исходящий webhook
type OutboundWebhook struct {
	ID           uint64     `json:"id"`
	Name         string     `json:"name" binding:"required"`
	URL          string     `json:"url" binding:"required"`
	Secret       string     `json:"secret,omitempty"`
	Events       []string   `json:"events" binding:"required"` // События, на которые подписан
	Active       bool       `json:"active"`
	Timeout      int        `json:"timeout"` // Таймаут в секундах
	RetryCount   int        `json:"retry_count"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsed     *time.Time `json:"last_used,omitempty"`
	FailureCount int        `json:"failure_count"`
}

// OutboundWebhookEvent представляет событие для отправки
type OutboundWebhookEvent struct {
	EventType   string                 `json:"event_type"`
	Timestamp   int64                  `json:"timestamp"`
	ServerID    string                 `json:"server_id"`
	Data        map[string]interface{} `json:"data"`
	Source      string                 `json:"source"`
	Environment string                 `json:"environment"`
}

// OutboundWebhookManager управляет исходящими webhook'ами
type OutboundWebhookManager struct {
	webhooks    map[uint64]*OutboundWebhook
	eventQueue  chan OutboundWebhookEvent
	mu          sync.RWMutex
	nextID      uint64
	httpClient  *http.Client
	serverID    string
	environment string
}

// NewOutboundWebhookManager создает новый менеджер исходящих webhook'ов
func NewOutboundWebhookManager(serverID, environment string) *OutboundWebhookManager {
	manager := &OutboundWebhookManager{
		webhooks:    make(map[uint64]*OutboundWebhook),
		eventQueue:  make(chan OutboundWebhookEvent, 1000), // Буфер для событий
		nextID:      1,
		serverID:    serverID,
		environment: environment,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Запускаем воркера для обработки событий
	go manager.eventWorker()

	return manager
}

// AddWebhook добавляет новый webhook
func (owm *OutboundWebhookManager) AddWebhook(webhook OutboundWebhook) *OutboundWebhook {
	owm.mu.Lock()
	defer owm.mu.Unlock()

	webhook.ID = owm.nextID
	owm.nextID++
	webhook.CreatedAt = time.Now()
	webhook.Active = true

	if webhook.Timeout == 0 {
		webhook.Timeout = 30
	}
	if webhook.RetryCount == 0 {
		webhook.RetryCount = 3
	}

	owm.webhooks[webhook.ID] = &webhook
	return &webhook
}

// GetWebhooks возвращает список всех webhook'ов
func (owm *OutboundWebhookManager) GetWebhooks() []*OutboundWebhook {
	owm.mu.RLock()
	defer owm.mu.RUnlock()

	webhooks := make([]*OutboundWebhook, 0, len(owm.webhooks))
	for _, webhook := range owm.webhooks {
		webhooks = append(webhooks, webhook)
	}
	return webhooks
}

// GetWebhook возвращает webhook по ID
func (owm *OutboundWebhookManager) GetWebhook(id uint64) *OutboundWebhook {
	owm.mu.RLock()
	defer owm.mu.RUnlock()

	webhook, exists := owm.webhooks[id]
	if !exists {
		return nil
	}
	return webhook
}

// UpdateWebhook обновляет webhook
func (owm *OutboundWebhookManager) UpdateWebhook(id uint64, updates OutboundWebhook) *OutboundWebhook {
	owm.mu.Lock()
	defer owm.mu.Unlock()

	webhook, exists := owm.webhooks[id]
	if !exists {
		return nil
	}

	// Обновляем поля
	if updates.Name != "" {
		webhook.Name = updates.Name
	}
	if updates.URL != "" {
		webhook.URL = updates.URL
	}
	if updates.Secret != "" {
		webhook.Secret = updates.Secret
	}
	if len(updates.Events) > 0 {
		webhook.Events = updates.Events
	}
	if updates.Timeout > 0 {
		webhook.Timeout = updates.Timeout
	}
	if updates.RetryCount >= 0 {
		webhook.RetryCount = updates.RetryCount
	}
	webhook.Active = updates.Active

	return webhook
}

// DeleteWebhook удаляет webhook
func (owm *OutboundWebhookManager) DeleteWebhook(id uint64) bool {
	owm.mu.Lock()
	defer owm.mu.Unlock()

	_, exists := owm.webhooks[id]
	if !exists {
		return false
	}

	delete(owm.webhooks, id)
	return true
}

// SendEvent отправляет событие всем подписанным webhook'ам
func (owm *OutboundWebhookManager) SendEvent(eventType string, data map[string]interface{}) {
	event := OutboundWebhookEvent{
		EventType:   eventType,
		Timestamp:   time.Now().Unix(),
		ServerID:    owm.serverID,
		Data:        data,
		Source:      "game_server",
		Environment: owm.environment,
	}

	// Добавляем событие в очередь
	select {
	case owm.eventQueue <- event:
		log.Printf("📤 Событие %s добавлено в очередь webhook'ов", eventType)
	default:
		log.Printf("⚠️  Очередь webhook'ов переполнена, событие %s пропущено", eventType)
	}
}

// eventWorker обрабатывает события из очереди
func (owm *OutboundWebhookManager) eventWorker() {
	for event := range owm.eventQueue {
		owm.processEvent(event)
	}
}

// processEvent обрабатывает одно событие
func (owm *OutboundWebhookManager) processEvent(event OutboundWebhookEvent) {
	owm.mu.RLock()
	webhooks := make([]*OutboundWebhook, 0)

	// Находим webhook'и, подписанные на это событие
	for _, webhook := range owm.webhooks {
		if webhook.Active && owm.isSubscribedToEvent(webhook, event.EventType) {
			webhooks = append(webhooks, webhook)
		}
	}
	owm.mu.RUnlock()

	// Отправляем событие каждому подписанному webhook'у
	for _, webhook := range webhooks {
		go owm.sendToWebhook(webhook, event)
	}
}

// isSubscribedToEvent проверяет, подписан ли webhook на событие
func (owm *OutboundWebhookManager) isSubscribedToEvent(webhook *OutboundWebhook, eventType string) bool {
	for _, subscribedEvent := range webhook.Events {
		if subscribedEvent == eventType || subscribedEvent == "*" {
			return true
		}
	}
	return false
}

// sendToWebhook отправляет событие конкретному webhook'у
func (owm *OutboundWebhookManager) sendToWebhook(webhook *OutboundWebhook, event OutboundWebhookEvent) {
	// Подготавливаем данные
	jsonData, err := json.Marshal(event)
	if err != nil {
		log.Printf("❌ Ошибка маршалинга события для webhook %s: %v", webhook.Name, err)
		return
	}

	// Создаем HTTP запрос
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(webhook.Timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ Ошибка создания запроса для webhook %s: %v", webhook.Name, err)
		return
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MMO-Game-Server/1.0")
	req.Header.Set("X-Event-Type", event.EventType)
	req.Header.Set("X-Server-ID", event.ServerID)

	// Добавляем подпись, если есть секрет
	if webhook.Secret != "" {
		signature := owm.generateSignature(jsonData, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	// Отправляем с retry логикой
	success := false
	for attempt := 0; attempt <= webhook.RetryCount; attempt++ {
		resp, err := owm.httpClient.Do(req)
		if err != nil {
			log.Printf("⚠️  Попытка %d/%d для webhook %s: %v", attempt+1, webhook.RetryCount+1, webhook.Name, err)
			time.Sleep(time.Duration(attempt+1) * time.Second) // Экспоненциальная задержка
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			success = true
			log.Printf("✅ Событие %s успешно отправлено в webhook %s", event.EventType, webhook.Name)
			resp.Body.Close()
			break
		} else {
			log.Printf("⚠️  Webhook %s вернул статус %d на попытке %d", webhook.Name, resp.StatusCode, attempt+1)
			resp.Body.Close()
			if attempt < webhook.RetryCount {
				time.Sleep(time.Duration(attempt+1) * time.Second)
			}
		}
	}

	// Обновляем статистику
	owm.mu.Lock()
	now := time.Now()
	webhook.LastUsed = &now
	if !success {
		webhook.FailureCount++
	}
	owm.mu.Unlock()
}

// generateSignature генерирует HMAC подпись
func (owm *OutboundWebhookManager) generateSignature(data []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// GetEventTypes возвращает доступные типы событий
func (owm *OutboundWebhookManager) GetEventTypes() []string {
	return []string{
		"server.started",
		"server.stopped",
		"server.error",
		"server.high_cpu",
		"server.high_memory",
		"server.low_tps",
		"player.joined",
		"player.left",
		"player.banned",
		"player.kicked",
		"anticheat.violation",
		"anticheat.ban",
		"world.saved",
		"world.load_error",
		"chat.message",
		"admin.command",
		"security.alert",
		"backup.completed",
		"backup.failed",
	}
}

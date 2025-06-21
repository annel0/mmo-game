package network

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
)

// PredictionVisualizer предоставляет веб-интерфейс для отладки prediction
type PredictionVisualizer struct {
	logger        *logging.Logger
	httpServer    *http.Server
	predictionSvc *PredictionService
	mu            sync.RWMutex

	// Данные для визуализации
	errorHistory    map[uint64][]*PredictionErrorPoint // playerID -> error points
	snapshotHistory map[uint64][]*SnapshotPoint        // playerID -> snapshot points
	maxHistorySize  int

	// Настройки
	webUIPort int
	enabled   bool
}

// PredictionErrorPoint точка ошибки prediction
type PredictionErrorPoint struct {
	Timestamp time.Time      `json:"timestamp"`
	Error     float32        `json:"error"`
	PlayerID  uint64         `json:"player_id"`
	ServerPos *protocol.Vec2 `json:"server_pos"`
	ClientPos *protocol.Vec2 `json:"client_pos"`
	InputID   uint32         `json:"input_id"`
	Corrected bool           `json:"corrected"`
}

// SnapshotPoint точка снимка
type SnapshotPoint struct {
	Timestamp       time.Time      `json:"timestamp"`
	SnapshotID      uint32         `json:"snapshot_id"`
	PlayerID        uint64         `json:"player_id"`
	Position        *protocol.Vec2 `json:"position"`
	EntitiesCount   int            `json:"entities_count"`
	ProcessedInputs uint32         `json:"processed_inputs"`
}

// VisualizationData данные для визуализации
type VisualizationData struct {
	Players          []PlayerVisualizationData `json:"players"`
	Timestamp        time.Time                 `json:"timestamp"`
	TotalPlayers     int                       `json:"total_players"`
	AvgError         float32                   `json:"avg_error"`
	MaxError         float32                   `json:"max_error"`
	TotalCorrections uint64                    `json:"total_corrections"`
}

// PlayerVisualizationData данные визуализации для игрока
type PlayerVisualizationData struct {
	PlayerID        uint64                           `json:"player_id"`
	CurrentPosition *protocol.Vec2                   `json:"current_position"`
	ErrorHistory    []*PredictionErrorPoint          `json:"error_history"`
	SnapshotHistory []*SnapshotPoint                 `json:"snapshot_history"`
	Stats           *protocol.PredictionStatsMessage `json:"stats"`
	LastErrorTime   time.Time                        `json:"last_error_time"`
	AvgError        float32                          `json:"avg_error"`
	MaxError        float32                          `json:"max_error"`
}

// NewPredictionVisualizer создаёт новый визуализатор
func NewPredictionVisualizer(logger *logging.Logger, predictionSvc *PredictionService, webUIPort int) *PredictionVisualizer {
	return &PredictionVisualizer{
		logger:          logger,
		predictionSvc:   predictionSvc,
		webUIPort:       webUIPort,
		enabled:         true,
		errorHistory:    make(map[uint64][]*PredictionErrorPoint),
		snapshotHistory: make(map[uint64][]*SnapshotPoint),
		maxHistorySize:  200, // Храним последние 200 точек
	}
}

// Start запускает визуализатор
func (pv *PredictionVisualizer) Start(ctx context.Context) error {
	if !pv.enabled {
		return nil
	}

	pv.logger.Info("🎨 Запуск визуализатора prediction на порту %d", pv.webUIPort)

	// Настройка HTTP роутов
	mux := http.NewServeMux()
	mux.HandleFunc("/", pv.handleWebUI)
	mux.HandleFunc("/api/data", pv.handleAPIData)
	mux.HandleFunc("/api/player/", pv.handleAPIPlayer)
	mux.HandleFunc("/api/reset", pv.handleAPIReset)

	pv.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", pv.webUIPort),
		Handler: mux,
	}

	go func() {
		if err := pv.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			pv.logger.Error("Ошибка запуска веб-сервера визуализатора: %v", err)
		}
	}()

	return nil
}

// Stop останавливает визуализатор
func (pv *PredictionVisualizer) Stop() error {
	if pv.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return pv.httpServer.Shutdown(ctx)
}

// LogPredictionError логирует ошибку prediction
func (pv *PredictionVisualizer) LogPredictionError(playerID uint64, serverPos, clientPos *protocol.Vec2, error float32, inputID uint32, corrected bool) {
	if !pv.enabled {
		return
	}

	pv.mu.Lock()
	defer pv.mu.Unlock()

	point := &PredictionErrorPoint{
		Timestamp: time.Now(),
		Error:     error,
		PlayerID:  playerID,
		ServerPos: serverPos,
		ClientPos: clientPos,
		InputID:   inputID,
		Corrected: corrected,
	}

	// Добавляем в историю
	if _, exists := pv.errorHistory[playerID]; !exists {
		pv.errorHistory[playerID] = make([]*PredictionErrorPoint, 0, pv.maxHistorySize)
	}

	pv.errorHistory[playerID] = append(pv.errorHistory[playerID], point)

	// Ограничиваем размер истории
	if len(pv.errorHistory[playerID]) > pv.maxHistorySize {
		pv.errorHistory[playerID] = pv.errorHistory[playerID][1:]
	}
}

// LogSnapshot логирует снимок
func (pv *PredictionVisualizer) LogSnapshot(playerID uint64, snapshot *protocol.WorldSnapshotMessage) {
	if !pv.enabled {
		return
	}

	pv.mu.Lock()
	defer pv.mu.Unlock()

	entitiesCount := len(snapshot.VisibleEntities)
	if snapshot.PlayerState != nil {
		entitiesCount++ // Считаем игрока
	}

	point := &SnapshotPoint{
		Timestamp:       time.Now(),
		SnapshotID:      snapshot.SnapshotId,
		PlayerID:        playerID,
		Position:        snapshot.PlayerState.Position,
		EntitiesCount:   entitiesCount,
		ProcessedInputs: snapshot.LastProcessedInput,
	}

	// Добавляем в историю
	if _, exists := pv.snapshotHistory[playerID]; !exists {
		pv.snapshotHistory[playerID] = make([]*SnapshotPoint, 0, pv.maxHistorySize)
	}

	pv.snapshotHistory[playerID] = append(pv.snapshotHistory[playerID], point)

	// Ограничиваем размер истории
	if len(pv.snapshotHistory[playerID]) > pv.maxHistorySize {
		pv.snapshotHistory[playerID] = pv.snapshotHistory[playerID][1:]
	}
}

// GetVisualizationData возвращает данные для визуализации
func (pv *PredictionVisualizer) GetVisualizationData() *VisualizationData {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	data := &VisualizationData{
		Timestamp:    time.Now(),
		TotalPlayers: len(pv.errorHistory),
		Players:      make([]PlayerVisualizationData, 0, len(pv.errorHistory)),
	}

	var totalError float32
	var maxError float32
	var totalCorrections uint64
	var playerCount int

	// Собираем все уникальные ID игроков
	playerIDs := make(map[uint64]bool)
	for playerID := range pv.errorHistory {
		playerIDs[playerID] = true
	}
	for playerID := range pv.snapshotHistory {
		playerIDs[playerID] = true
	}

	// Создаём данные для каждого игрока
	for playerID := range playerIDs {
		playerData := PlayerVisualizationData{
			PlayerID:        playerID,
			ErrorHistory:    pv.errorHistory[playerID],
			SnapshotHistory: pv.snapshotHistory[playerID],
		}

		// Получаем статистику от prediction service
		if pv.predictionSvc != nil {
			playerData.Stats = pv.predictionSvc.GetPredictionStats(playerID)
		}

		// Вычисляем метрики игрока
		if len(playerData.ErrorHistory) > 0 {
			var sum float32
			for _, point := range playerData.ErrorHistory {
				sum += point.Error
				if point.Error > playerData.MaxError {
					playerData.MaxError = point.Error
				}
				if point.Corrected {
					totalCorrections++
				}
			}
			playerData.AvgError = sum / float32(len(playerData.ErrorHistory))
			playerData.LastErrorTime = playerData.ErrorHistory[len(playerData.ErrorHistory)-1].Timestamp

			totalError += playerData.AvgError
			if playerData.MaxError > maxError {
				maxError = playerData.MaxError
			}
			playerCount++
		}

		// Получаем текущую позицию из последнего снимка
		if len(playerData.SnapshotHistory) > 0 {
			lastSnapshot := playerData.SnapshotHistory[len(playerData.SnapshotHistory)-1]
			playerData.CurrentPosition = lastSnapshot.Position
		}

		data.Players = append(data.Players, playerData)
	}

	// Вычисляем общие метрики
	if playerCount > 0 {
		data.AvgError = totalError / float32(playerCount)
	}
	data.MaxError = maxError
	data.TotalCorrections = totalCorrections

	// Сортируем игроков по ID
	sort.Slice(data.Players, func(i, j int) bool {
		return data.Players[i].PlayerID < data.Players[j].PlayerID
	})

	return data
}

// handleWebUI обслуживает веб-интерфейс
func (pv *PredictionVisualizer) handleWebUI(w http.ResponseWriter, r *http.Request) {
	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
    <title>Prediction Visualizer</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .stat-card { background: #f5f5f5; padding: 15px; border-radius: 8px; }
        .refresh-btn { background: #007bff; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎯 Prediction Visualizer</h1>
        <button class="refresh-btn" onclick="loadData()">Обновить данные</button>
        
        <div class="stats">
            <div class="stat-card">
                <h3>Общая статистика</h3>
                <p>Игроков: <span id="total-players">0</span></p>
                <p>Средняя ошибка: <span id="avg-error">0</span> px</p>
                <p>Макс ошибка: <span id="max-error">0</span> px</p>
                <p>Коррекций: <span id="total-corrections">0</span></p>
            </div>
        </div>
        
        <div id="players-container"></div>
    </div>

    <script>
        async function loadData() {
            try {
                const response = await fetch('/api/data');
                const data = await response.json();
                updateUI(data);
            } catch (error) {
                console.error('Ошибка загрузки данных:', error);
            }
        }

        function updateUI(data) {
            document.getElementById('total-players').textContent = data.total_players;
            document.getElementById('avg-error').textContent = data.avg_error.toFixed(2);
            document.getElementById('max-error').textContent = data.max_error.toFixed(2);
            document.getElementById('total-corrections').textContent = data.total_corrections;

            const container = document.getElementById('players-container');
            container.innerHTML = '<h2>Игроки: ' + data.players.length + '</h2>';
        }

        setInterval(loadData, 2000);
        loadData();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	tmpl, _ := template.New("visualizer").Parse(htmlTemplate)
	tmpl.Execute(w, nil)
}

// handleAPIData возвращает данные в JSON формате
func (pv *PredictionVisualizer) handleAPIData(w http.ResponseWriter, r *http.Request) {
	data := pv.GetVisualizationData()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleAPIPlayer возвращает данные конкретного игрока
func (pv *PredictionVisualizer) handleAPIPlayer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "TODO: implement player details"})
}

// handleAPIReset сбрасывает историю данных
func (pv *PredictionVisualizer) handleAPIReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pv.mu.Lock()
	pv.errorHistory = make(map[uint64][]*PredictionErrorPoint)
	pv.snapshotHistory = make(map[uint64][]*SnapshotPoint)
	pv.mu.Unlock()

	pv.logger.Info("История prediction сброшена")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reset successful"})
}

// SetEnabled включает/выключает визуализатор
func (pv *PredictionVisualizer) SetEnabled(enabled bool) {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	pv.enabled = enabled
}

// ClearHistory очищает историю данных
func (pv *PredictionVisualizer) ClearHistory() {
	pv.mu.Lock()
	defer pv.mu.Unlock()
	pv.errorHistory = make(map[uint64][]*PredictionErrorPoint)
	pv.snapshotHistory = make(map[uint64][]*SnapshotPoint)
}

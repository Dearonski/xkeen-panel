package xkeen

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xkeen-panel/internal/models"
)

type SubscriptionManager struct {
	dataDir string
	data    *models.SubscriptionData
	mu      sync.RWMutex
}

func NewSubscriptionManager(dataDir string) *SubscriptionManager {
	return &SubscriptionManager{
		dataDir: dataDir,
		data:    &models.SubscriptionData{},
	}
}

func (sm *SubscriptionManager) filePath() string {
	return filepath.Join(sm.dataDir, "subscription.json")
}

// Load загружает данные подписки из файла
func (sm *SubscriptionManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, sm.data)
}

// Save сохраняет данные подписки в файл
func (sm *SubscriptionManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if err := os.MkdirAll(sm.dataDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(sm.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.filePath(), data, 0600)
}

// UpdateURL обновляет URL подписки, скачивает и парсит серверы
func (sm *SubscriptionManager) UpdateURL(url string) ([]models.Server, error) {
	servers, err := sm.downloadAndParse(url)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	sm.data.URL = url
	sm.data.LastUpdated = time.Now()
	sm.data.Servers = servers

	// Если активный сервер вне диапазона — сбросить
	if sm.data.ActiveID >= len(servers) {
		sm.data.ActiveID = 0
	}

	// Пометить активный сервер
	for i := range sm.data.Servers {
		sm.data.Servers[i].Active = i == sm.data.ActiveID
	}
	sm.mu.Unlock()

	return servers, sm.Save()
}

// Refresh перезагружает серверы по текущему URL
func (sm *SubscriptionManager) Refresh() ([]models.Server, error) {
	sm.mu.RLock()
	url := sm.data.URL
	sm.mu.RUnlock()

	if url == "" {
		return nil, fmt.Errorf("URL подписки не задан")
	}

	servers, err := sm.downloadAndParse(url)
	if err != nil {
		return nil, err
	}

	sm.mu.Lock()
	sm.data.LastUpdated = time.Now()
	sm.data.Servers = servers
	if sm.data.ActiveID >= len(servers) {
		sm.data.ActiveID = 0
	}
	for i := range sm.data.Servers {
		sm.data.Servers[i].Active = i == sm.data.ActiveID
	}
	sm.mu.Unlock()

	return servers, sm.Save()
}

// GetData возвращает копию данных подписки
func (sm *SubscriptionManager) GetData() models.SubscriptionData {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return *sm.data
}

// GetServers возвращает список серверов
func (sm *SubscriptionManager) GetServers() []models.Server {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]models.Server, len(sm.data.Servers))
	copy(result, sm.data.Servers)
	return result
}

// SetActive устанавливает активный сервер по ID
func (sm *SubscriptionManager) SetActive(id int) (*models.Server, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if id < 0 || id >= len(sm.data.Servers) {
		return nil, fmt.Errorf("сервер с id %d не найден", id)
	}

	sm.data.ActiveID = id
	for i := range sm.data.Servers {
		sm.data.Servers[i].Active = i == id
	}

	server := sm.data.Servers[id]

	// Сохранение в горутине нежелательно — сохраним синхронно
	data, err := json.MarshalIndent(sm.data, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(sm.dataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(sm.filePath(), data, 0600); err != nil {
		return nil, err
	}

	return &server, nil
}

// GetActiveServer возвращает текущий активный сервер
func (sm *SubscriptionManager) GetActiveServer() *models.Server {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.data.Servers) == 0 {
		return nil
	}

	id := sm.data.ActiveID
	if id < 0 || id >= len(sm.data.Servers) {
		return nil
	}

	s := sm.data.Servers[id]
	return &s
}

// SelectNext переключает на следующий сервер
func (sm *SubscriptionManager) SelectNext() (*models.Server, error) {
	sm.mu.RLock()
	count := len(sm.data.Servers)
	current := sm.data.ActiveID
	sm.mu.RUnlock()

	if count == 0 {
		return nil, fmt.Errorf("нет доступных серверов")
	}

	next := (current + 1) % count
	return sm.SetActive(next)
}

func (sm *SubscriptionManager) downloadAndParse(url string) ([]models.Server, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("ошибка загрузки подписки: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("сервер вернул код %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	return ParseSubscription(string(body))
}

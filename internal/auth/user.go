package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xkeen-panel/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type UserManager struct {
	dataDir string
	user    *models.User
	mu      sync.RWMutex
	// Временное хранение данных setup до подтверждения TOTP
	pendingSetup *models.User
}

func NewUserManager(dataDir string) *UserManager {
	return &UserManager{dataDir: dataDir}
}

func (um *UserManager) userFilePath() string {
	return filepath.Join(um.dataDir, "user.json")
}

// Load загружает пользователя из файла
func (um *UserManager) Load() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	data, err := os.ReadFile(um.userFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return err
	}
	um.user = &user
	return nil
}

// SetupRequired возвращает true, если пользователь ещё не создан
func (um *UserManager) SetupRequired() bool {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.user == nil
}

// CreatePendingUser создаёт пользователя в памяти (до подтверждения TOTP)
func (um *UserManager) CreatePendingUser(username, password, totpSecret string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	jwtSecret, err := generateRandomKey(32)
	if err != nil {
		return err
	}

	um.pendingSetup = &models.User{
		Username:     username,
		PasswordHash: string(hash),
		TOTPSecret:   totpSecret,
		JWTSecret:    jwtSecret,
		CreatedAt:    time.Now(),
	}
	return nil
}

// ConfirmSetup сохраняет pending-пользователя на диск
func (um *UserManager) ConfirmSetup() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.pendingSetup == nil {
		return os.ErrNotExist
	}

	if err := os.MkdirAll(um.dataDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(um.pendingSetup, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(um.userFilePath(), data, 0600); err != nil {
		return err
	}

	um.user = um.pendingSetup
	um.pendingSetup = nil
	return nil
}

// GetPendingTOTPSecret возвращает TOTP-секрет из pending setup
func (um *UserManager) GetPendingTOTPSecret() string {
	um.mu.RLock()
	defer um.mu.RUnlock()
	if um.pendingSetup == nil {
		return ""
	}
	return um.pendingSetup.TOTPSecret
}

// HasPendingSetup возвращает true, если setup начат но не завершён
func (um *UserManager) HasPendingSetup() bool {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.pendingSetup != nil
}

// CheckPassword проверяет пароль пользователя
func (um *UserManager) CheckPassword(username, password string) bool {
	um.mu.RLock()
	defer um.mu.RUnlock()

	if um.user == nil || um.user.Username != username {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(um.user.PasswordHash), []byte(password)) == nil
}

// GetUser возвращает копию пользователя
func (um *UserManager) GetUser() *models.User {
	um.mu.RLock()
	defer um.mu.RUnlock()
	if um.user == nil {
		return nil
	}
	u := *um.user
	return &u
}

func generateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

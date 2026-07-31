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
	// Holds the setup data until TOTP is confirmed
	pendingSetup *models.User
}

func NewUserManager(dataDir string) *UserManager {
	return &UserManager{dataDir: dataDir}
}

func (um *UserManager) userFilePath() string {
	return filepath.Join(um.dataDir, "user.json")
}

// Load reads the stored user.
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

// SetupRequired reports whether no account exists yet.
func (um *UserManager) SetupRequired() bool {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.user == nil
}

// CreatePendingUser builds the account in memory, before TOTP is confirmed.
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

// ConfirmSetup persists the pending account.
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

// GetPendingTOTPSecret returns the TOTP secret of the pending setup.
func (um *UserManager) GetPendingTOTPSecret() string {
	um.mu.RLock()
	defer um.mu.RUnlock()
	if um.pendingSetup == nil {
		return ""
	}
	return um.pendingSetup.TOTPSecret
}

// HasPendingSetup reports whether setup was started but not finished.
func (um *UserManager) HasPendingSetup() bool {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.pendingSetup != nil
}

// CheckPassword verifies the account password.
func (um *UserManager) CheckPassword(username, password string) bool {
	um.mu.RLock()
	defer um.mu.RUnlock()

	if um.user == nil || um.user.Username != username {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(um.user.PasswordHash), []byte(password)) == nil
}

// GetUser returns a copy of the account.
func (um *UserManager) GetUser() *models.User {
	um.mu.RLock()
	defer um.mu.RUnlock()
	if um.user == nil {
		return nil
	}
	u := *um.user
	return &u
}

// persistLocked writes the current account to disk. Call with um.mu held.
func (um *UserManager) persistLocked() error {
	if um.user == nil {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(um.dataDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(um.user, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(um.userFilePath(), data, 0600)
}

func generateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

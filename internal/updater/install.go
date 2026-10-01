package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxBinarySize = 64 << 20

// The new binary removes this to prove it came up; if the guard still finds it, the update is rolled back.
type pendingMarker struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (u *Updater) pendingPath() string { return filepath.Join(u.dataDir, "update-pending.json") }
func (u *Updater) failedPath() string  { return filepath.Join(u.dataDir, "update-failed.json") }
func (u *Updater) prevPath() string    { return u.binPath + ".prev" }
func (u *Updater) newPath() string     { return u.binPath + ".new" }

func (u *Updater) install(ctx context.Context, release Release) error {
	if release.assetURL == "" {
		return fmt.Errorf("в релизе %s нет файла %s", release.Tag, u.asset)
	}

	sum, err := u.expectedSum(ctx, release)
	if err != nil {
		return err
	}

	if err := u.download(ctx, release.assetURL, u.newPath(), sum); err != nil {
		os.Remove(u.newPath())
		return err
	}

	if err := u.smokeTest(ctx, u.newPath(), release.Tag); err != nil {
		os.Remove(u.newPath())
		return err
	}

	if err := os.Rename(u.binPath, u.prevPath()); err != nil {
		os.Remove(u.newPath())
		return fmt.Errorf("не удалось сохранить текущую версию: %w", err)
	}
	if err := os.Rename(u.newPath(), u.binPath); err != nil {
		os.Rename(u.prevPath(), u.binPath)
		return fmt.Errorf("не удалось установить новую версию: %w", err)
	}

	if err := writeJSON(u.pendingPath(), pendingMarker{From: u.current, To: release.Tag}); err != nil {
		u.log("[UPDATE] Не удалось записать маркер обновления: %v — откат при сбое запуска не сработает", err)
	} else if err := u.startGuard(); err != nil {
		u.log("[UPDATE] Сторож отката не запущен: %v", err)
	}

	return nil
}

func (u *Updater) download(ctx context.Context, url, path, sum string) error {
	resp, err := u.get(ctx, url, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}

	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, maxBinarySize+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("загрузка прервана: %w", err)
	}
	if n > maxBinarySize {
		return fmt.Errorf("файл больше %d МБ — это не бинарь панели", maxBinarySize>>20)
	}

	if got := hex.EncodeToString(hash.Sum(nil)); got != sum {
		return fmt.Errorf("контрольная сумма не совпала (%s…, ожидалась %s…)", got[:12], sum[:min(12, len(sum))])
	}

	return os.Chmod(path, 0755)
}

// A wrong-arch or truncated binary fails here instead of leaving the router without a panel.
func (u *Updater) smokeTest(ctx context.Context, path, tag string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("новая версия не запускается на этом роутере: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	if got := strings.TrimSpace(string(out)); got != tag {
		return fmt.Errorf("новый бинарь сообщает версию %q, ожидалась %s", got, tag)
	}
	return nil
}

// Runs detached: the process starting it is about to be replaced and may never come back.
const guardScript = `sleep "$GUARD_DELAY"
[ -f "$GUARD_MARKER" ] || exit 0
[ -f "$GUARD_PREV" ] || exit 0
mv -f "$GUARD_PREV" "$GUARD_BIN"
mv -f "$GUARD_MARKER" "$GUARD_FAILED"
if [ -x "$GUARD_INIT" ]; then
	"$GUARD_INIT" restart
fi`

func (u *Updater) startGuard() error {
	// The outer shell exits at once, so the guard is orphaned to init and nothing reaps it after exec
	cmd := exec.Command("/bin/sh", "-c", `(`+guardScript+`) </dev/null >/dev/null 2>&1 &`)
	cmd.Env = append(os.Environ(),
		"GUARD_DELAY="+strconv.Itoa(int(u.guardDelay.Seconds())),
		"GUARD_MARKER="+u.pendingPath(),
		"GUARD_FAILED="+u.failedPath(),
		"GUARD_PREV="+u.prevPath(),
		"GUARD_BIN="+u.binPath,
		"GUARD_INIT="+u.initScript,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	return cmd.Run()
}

func (u *Updater) rollbackBinary() error {
	if _, err := os.Stat(u.prevPath()); err != nil {
		return fmt.Errorf("предыдущей версии нет на диске")
	}
	if err := os.Rename(u.prevPath(), u.binPath); err != nil {
		return fmt.Errorf("откат не выполнен: %w", err)
	}
	os.Remove(u.pendingPath())
	return nil
}

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

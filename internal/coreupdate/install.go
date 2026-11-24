package coreupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxArchive = 96 << 20
	maxBinary  = 160 << 20
)

// XKeen ships the softfloat build to these models only
var softfloatModels = []string{"KN-1212", "KN-2310", "KN-2311", "KN-2910"}

func prevPath(bin string) string { return bin + ".prev" }
func newPath(bin string) string  { return bin + ".new" }

func (u *Updater) install(ctx context.Context, release Release) error {
	rt, reason := u.supported()
	if reason != "" {
		return fmt.Errorf("%s", reason)
	}
	bin := rt.CoreBin

	u.mu.Lock()
	from := u.current
	u.mu.Unlock()

	u.setStep("Скачиваю " + release.Tag)
	sum, err := u.expectedSum(ctx, release)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(u.dataDir, 0700); err != nil {
		return err
	}
	archive := filepath.Join(u.dataDir, "xray-update.zip")
	defer os.Remove(archive)
	if err := u.download(ctx, release.assetURL, archive, sum); err != nil {
		return err
	}

	u.setStep("Проверяю " + release.Tag)
	staged := newPath(bin)
	if err := extractBinary(archive, u.binaryMember(), staged); err != nil {
		os.Remove(staged)
		return err
	}
	if err := u.vet(ctx, staged, release.Tag, rt.XrayConfDir); err != nil {
		os.Remove(staged)
		return err
	}

	u.setStep("Перезапускаю ядро")
	os.Remove(prevPath(bin))
	if err := os.Rename(bin, prevPath(bin)); err != nil {
		os.Remove(staged)
		return fmt.Errorf("не удалось сохранить текущую версию: %w", err)
	}
	if err := os.Rename(staged, bin); err != nil {
		os.Rename(prevPath(bin), bin)
		return fmt.Errorf("не удалось поставить новую версию: %w", err)
	}

	u.restart()
	if err := u.waitRunning(ctx, bin); err != nil {
		if restoreErr := u.restorePrevious(bin); restoreErr != nil {
			return fmt.Errorf("Xray %s не запустился (%v), и вернуть %s не удалось: %v", release.Tag, err, from, restoreErr)
		}
		u.restart()
		return fmt.Errorf("Xray %s не запустился (%v) — ядро возвращено на %s", release.Tag, err, from)
	}

	u.log("[XRAY] Ядро обновлено: %s → %s", from, release.Tag)
	return nil
}

func (u *Updater) binaryMember() string {
	if u.arch != "mipsle" {
		return "xray"
	}
	model := u.model()
	for _, m := range softfloatModels {
		if strings.Contains(model, m) {
			return "xray_softfloat"
		}
	}
	return "xray"
}

func (u *Updater) download(ctx context.Context, url, path, sum string) error {
	resp, err := u.get(ctx, url, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, maxArchive+1))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("загрузка прервана: %w", err)
	}
	if n > maxArchive {
		return fmt.Errorf("архив больше %d МБ — это не сборка Xray", maxArchive>>20)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != sum {
		return fmt.Errorf("контрольная сумма архива не совпала (%s…, ожидалась %s…)", got[:12], sum[:min(12, len(sum))])
	}
	return nil
}

func extractBinary(archive, member, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("архив не открылся: %w", err)
	}
	defer zr.Close()

	var file *zip.File
	for _, f := range zr.File {
		if f.Name == member {
			file = f
		}
	}
	if file == nil {
		return fmt.Errorf("в архиве нет %s", member)
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(src, maxBinary+1))
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("распаковка прервана: %w", err)
	}
	if n > maxBinary {
		return fmt.Errorf("%s больше %d МБ", member, maxBinary>>20)
	}
	return os.Chmod(dest, 0755)
}

// The running configs are tested before the swap, so a core that would reject them never replaces the working one
func (u *Updater) vet(ctx context.Context, bin, tag, confDir string) error {
	got, err := binaryVersion(ctx, bin)
	if err != nil {
		return fmt.Errorf("новая версия не запускается на этом роутере: %v", err)
	}
	if !sameVersion(got, tag) {
		return fmt.Errorf("новый бинарь сообщает версию %s, ожидалась %s", got, tag)
	}
	if confDir == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "run", "-test", "-confdir", confDir)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Join(filepath.Dir(confDir), "dat"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Xray %s не принимает текущие конфиги: %s", tag, lastLine(out))
	}
	return nil
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func (u *Updater) waitRunning(ctx context.Context, bin string) error {
	deadline := time.Now().Add(u.startTimeout)
	for !u.running(bin) {
		if time.Now().After(deadline) {
			return fmt.Errorf("процесс не появился за %s", u.startTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(u.settleFor):
	}
	if !u.running(bin) {
		return fmt.Errorf("процесс завершился сразу после запуска")
	}
	return nil
}

func (u *Updater) restorePrevious(bin string) error {
	if _, err := os.Stat(prevPath(bin)); err != nil {
		return fmt.Errorf("прежней версии нет на диске")
	}
	if err := os.Rename(prevPath(bin), bin); err != nil {
		return fmt.Errorf("прежняя версия не восстановлена: %w", err)
	}
	return nil
}

func binaryVersion(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v (%s)", err, lastLine(out))
	}
	m := xrayVersionRe.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("в выводе нет версии Xray")
	}
	return string(m[1]), nil
}

// A replaced binary shows up in /proc under its new name, so only a process started from bin itself counts
func processRunning(bin string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if exe, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil && exe == bin {
			return true
		}
	}
	return false
}

func routerModel() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:79/rci/show/version")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return string(bytes.TrimSpace(body))
}

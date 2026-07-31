package xkeen

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	restartMu            sync.Mutex
	restarting           bool
	OnRestartStateChange func(restarting bool)

	runningMu        sync.Mutex
	runningCached    bool
	runningCore      string
	runningCheckedAt time.Time
)

const (
	restartTimeout = 180 * time.Second
	quickTimeout   = 60 * time.Second
)

// runXkeen runs the dispatcher and captures its output.
//
// XKEEN_FOREGROUND=1 disables the fork's self-detach: without a TTY (which is
// always our case) `xkeen -start|-stop|-restart` re-execs itself through
// start-stop-daemon -b and returns immediately, so Wait() would report success
// while the restart is still running.
//
// Output goes to a temp file rather than a pipe because the proxy core inherits
// our descriptors — with a pipe Wait() blocks until the core exits.
func runXkeen(dispatcher string, timeout time.Duration, args ...string) (string, error) {
	if dispatcher == "" {
		return "", fmt.Errorf("путь к xkeen не задан")
	}

	out, err := os.CreateTemp("", "xkeen-out-*")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	defer os.Remove(out.Name())
	defer out.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, dispatcher, args...)
	cmd.Env = append(os.Environ(), "XKEEN_FOREGROUND=1")
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	runErr := cmd.Run()

	body, _ := os.ReadFile(out.Name())
	text := strings.TrimSpace(string(ansiRe.ReplaceAll(body, nil)))

	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("команда xkeen %s не завершилась за %s", strings.Join(args, " "), timeout)
	}

	return text, runErr
}

// Restart restarts the proxy core via `xkeen -restart`. Concurrent restarts are
// skipped, and the busy flag now spans the whole operation.
func Restart(dispatcher string) (string, error) {
	restartMu.Lock()
	if restarting {
		restartMu.Unlock()
		log.Printf("[RESTART] Пропущен — уже идёт рестарт")
		return "restart already in progress", nil
	}
	restarting = true
	restartMu.Unlock()

	if OnRestartStateChange != nil {
		OnRestartStateChange(true)
	}

	log.Printf("[RESTART] Запуск: %s -restart", dispatcher)

	go func() {
		output, err := runXkeen(dispatcher, restartTimeout, "-restart")

		restartMu.Lock()
		restarting = false
		restartMu.Unlock()

		if OnRestartStateChange != nil {
			OnRestartStateChange(false)
		}

		if err != nil {
			log.Printf("[RESTART] Ошибка: %v (%s)", err, output)
		} else {
			log.Printf("[RESTART] Завершён успешно")
		}
	}()

	return "restart initiated", nil
}

// Start starts the proxy core. The `on` argument mirrors what `xkeen -start`
// passes to the init script, so autostart state is not consulted.
func Start(dispatcher string) (string, error) {
	return runXkeen(dispatcher, restartTimeout, "-start")
}

// Stop stops the proxy core.
func Stop(dispatcher string) (string, error) {
	return runXkeen(dispatcher, quickTimeout, "-stop")
}

// Status returns the human-readable output of `xkeen -status`.
func Status(dispatcher string) (string, error) {
	return runXkeen(dispatcher, quickTimeout, "-status")
}

// TestConfig validates the config of the active core: `-xtest` runs
// `xray -format=json -test`, `-mtest` does the same for Mihomo. Both propagate
// the core's exit code, so a non-nil error means the config is broken.
func TestConfig(dispatcher, core string) (string, error) {
	flag := "-xtest"
	if core == CoreMihomo {
		flag = "-mtest"
	}
	return runXkeen(dispatcher, quickTimeout, flag)
}

// IsRestarting reports whether a restart is in flight.
func IsRestarting() bool {
	restartMu.Lock()
	defer restartMu.Unlock()
	return restarting
}

// IsRunning reports whether the proxy core process is alive. Cached for 2s —
// polling /proc on every SSE push is expensive on a router.
func IsRunning(core string) bool {
	if core == "" {
		core = CoreXray
	}

	runningMu.Lock()
	defer runningMu.Unlock()

	if core == runningCore && time.Since(runningCheckedAt) < 2*time.Second {
		return runningCached
	}

	runningCached = coreProcessRunning(core)
	runningCore = core
	runningCheckedAt = time.Now()

	return runningCached
}

// coreProcessRunning looks for the core process. The PID file XKeen maintains is
// checked first; /proc is the fallback because a stale PID file outlives a crash.
func coreProcessRunning(core string) bool {
	if pid, ok := readPIDFile("/opt/var/run/" + core + ".pid"); ok && processAlive(pid, core) {
		return true
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		// No /proc (macOS during development) — fall back to ps
		out, _ := exec.Command("sh", "-c", "ps | grep -v grep | grep "+core).Output()
		return len(bytes.TrimSpace(out)) > 0
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		cmd := string(bytes.ReplaceAll(data, []byte{0}, []byte{' '}))
		if strings.Contains(cmd, "xkeen-panel") {
			continue // do not count the panel itself
		}
		if strings.Contains(cmd, core) {
			return true
		}
	}

	return false
}

func readPIDFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// processAlive verifies the PID is both alive and actually the core — PIDs get
// recycled, so the cmdline is checked too.
func processAlive(pid int, core string) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return false
	}
	return strings.Contains(string(bytes.ReplaceAll(data, []byte{0}, []byte{' '})), core)
}

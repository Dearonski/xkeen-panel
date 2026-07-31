package xkeen

import (
	"fmt"
	"log"
	"strings"

	"xkeen-panel/internal/models"
)

// ApplyServer writes the server into the core config and validates the result
// before anyone restarts on top of it.
//
// Without the check a bad outbound leaves XKeen unable to start, and the panel
// would report success: `xkeen -restart` returns long before the core fails.
// Validation runs the core's own parser (`xray -test` / `mihomo -t`), so a
// failure means the config really is broken — the previous one is restored.
func ApplyServer(rt Runtime, outboundsPath string, server *models.Server) error {
	if err := UpdateOutbound(outboundsPath, server); err != nil {
		return err
	}

	if !rt.Installed || rt.Dispatcher == "" {
		return nil
	}

	output, err := TestConfig(rt.Dispatcher, rt.Core)
	if err == nil {
		return nil
	}

	if rbErr := RestoreBackup(outboundsPath); rbErr != nil {
		log.Printf("[APPLY] Конфиг не прошёл проверку и откат не удался: %v", rbErr)
		return fmt.Errorf("конфигурация %s не прошла проверку, откат не удался: %v (%s)", rt.Core, rbErr, output)
	}

	log.Printf("[APPLY] Конфиг не прошёл проверку — выполнен откат")

	return fmt.Errorf("конфигурация %s не прошла проверку, изменения отменены: %s", rt.Core, TailLines(output, 4))
}

// TailLines trims validator output to something a UI can show. The tail is what
// matters: `xray -test` logs a line per config file it reads and only then
// prints the failure, so the head is noise.
func TailLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")

	kept := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(kept) < n; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			kept = append([]string{line}, kept...)
		}
	}

	if len(kept) == 0 {
		return "вывод пуст"
	}

	return strings.Join(kept, "; ")
}

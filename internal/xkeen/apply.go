package xkeen

import (
	"fmt"
	"log"

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

	return fmt.Errorf("конфигурация %s не прошла проверку, изменения отменены: %s", rt.Core, firstLines(output, 5))
}

// firstLines trims validator output to something a UI can show.
func firstLines(s string, n int) string {
	if s == "" {
		return "вывод пуст"
	}

	lines := 0
	for i, c := range s {
		if c != '\n' {
			continue
		}
		lines++
		if lines == n {
			return s[:i]
		}
	}

	return s
}

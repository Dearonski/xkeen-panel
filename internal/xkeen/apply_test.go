package xkeen

import "testing"

// Real `xray -test` output: one Info line per config file, verdict at the end.
// Trimming from the head would show the reader log and hide the failure.
func TestTailLinesKeepsTheVerdict(t *testing.T) {
	output := `Xray 26.7.28 (Xray, Penetrates Everything.) 5ca6f4b
2026/07/31 19:49:44 Using confdir from env: /opt/etc/xray/configs
2026/07/31 19:49:44 [Info] infra/conf/serial: Reading config: 01_log.json
2026/07/31 19:49:44 [Info] infra/conf/serial: Reading config: 04_outbounds.json
Failed to start: main: failed to load config files: [04_outbounds.json]
> infra/conf: unknown protocol "vles"`

	got := TailLines(output, 2)
	want := `Failed to start: main: failed to load config files: [04_outbounds.json]; > infra/conf: unknown protocol "vles"`

	if got != want {
		t.Errorf("TailLines =\n%q\nwant\n%q", got, want)
	}
}

func TestTailLinesEmpty(t *testing.T) {
	if got := TailLines("  \n\n ", 3); got != "вывод пуст" {
		t.Errorf("TailLines = %q, want the empty-output placeholder", got)
	}
}

func TestTailLinesShorterThanLimit(t *testing.T) {
	if got := TailLines("only one line", 5); got != "only one line" {
		t.Errorf("TailLines = %q, want the single line", got)
	}
}

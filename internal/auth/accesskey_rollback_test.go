package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// Регресс: если запись на диск падает, in-memory состояние ключа откатывается,
// чтобы оно не разошлось с диском (ключ не должен числиться включённым).
func TestAccessKeyGenerateRollbackOnPersistError(t *testing.T) {
	dir := t.TempDir()
	um := newConfirmedUserIn(t, dir)

	// Саботаж: делаем user.json директорией — os.WriteFile в неё не сможет.
	p := filepath.Join(dir, "user.json")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}

	if _, err := um.GenerateAccessKey(); err == nil {
		t.Fatal("ожидалась ошибка записи")
	}
	if um.HasAccessKey() {
		t.Error("откат не сработал: HasAccessKey=true после ошибки записи")
	}
}

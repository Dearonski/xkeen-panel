package geoip

import (
	"os"
	"path/filepath"
	"testing"
)

// Регресс: битая длина length-delimited поля не должна валить парсер паникой.
func TestWalkOverflowNoPanic(t *testing.T) {
	// field 1, wire 2, length varint = 0x7FFFFFFFFFFFFFFF (max int64)
	bad := []byte{0x0A, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F}
	p := filepath.Join(t.TempDir(), "bad.dat")
	if err := os.WriteFile(p, bad, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p, []string{"RU"}); err == nil {
		t.Error("ожидалась ошибка на битом geoip.dat (а не паника)")
	}
}

package xkeen

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"xkeen-panel/internal/models"
)

const (
	uriA = "vless://aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa@1.1.1.1:443?type=tcp&security=reality&sni=a.com&fp=chrome#NL-1"
	uriB = "vless://bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb@2.2.2.2:443?type=tcp&security=reality&sni=b.com&fp=chrome#DE-2"
	uriC = "vless://cccccccc-cccc-cccc-cccc-cccccccccccc@3.3.3.3:443?type=tcp&security=reality&sni=c.com&fp=chrome#FI-3"
)

func body(uris ...string) string {
	return strings.Join(uris, "\n")
}

// subServer отдаёт изменяемое тело подписки; setter меняет его между запросами.
func subServer(t *testing.T, initial string) (*httptest.Server, func(string)) {
	t.Helper()
	var mu sync.Mutex
	current := initial
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		b := current
		mu.Unlock()
		w.Write([]byte(b))
	}))
	t.Cleanup(srv.Close)
	return srv, func(s string) {
		mu.Lock()
		current = s
		mu.Unlock()
	}
}

func findByURI(servers []models.Server, uri string) *models.Server {
	for i := range servers {
		if servers[i].RawURI == uri {
			return &servers[i]
		}
	}
	return nil
}

func TestRefreshPreservesActiveByURI(t *testing.T) {
	srv, set := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(t.TempDir())
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}
	if _, err := sm.SetActive(1); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	set(body(uriC, uriA, uriB))
	if _, err := sm.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	active := sm.GetActiveServer()
	if active == nil || active.RawURI != uriB {
		t.Fatalf("активный сервер сохранён по индексу, а не по URI: %+v", active)
	}
	if got := sm.GetData().ActiveID; got != 2 {
		t.Errorf("ActiveID = %d, want 2 (новый индекс B)", got)
	}
}

func TestRefreshActiveRemoved(t *testing.T) {
	srv, set := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(t.TempDir())
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}
	if _, err := sm.SetActive(1); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	set(body(uriA, uriC))
	if _, err := sm.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if got := sm.GetData().ActiveID; got != 0 {
		t.Errorf("ActiveID = %d, want 0 (fallback)", got)
	}
	active := sm.GetActiveServer()
	if active == nil || active.RawURI != uriA {
		t.Fatalf("после удаления активного ожидался первый сервер, got %+v", active)
	}
}

func TestCarryOverrides(t *testing.T) {
	srv, _ := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(t.TempDir())
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}

	if err := sm.SetCountryOverride(1, "NL"); err != nil {
		t.Fatalf("SetCountryOverride: %v", err)
	}

	if _, err := sm.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	s := findByURI(sm.GetServers(), uriB)
	if s == nil {
		t.Fatal("сервер B не найден после refresh")
	}
	if s.CountryOverride != "NL" {
		t.Errorf("CountryOverride = %q, want NL (перенос по RawURI)", s.CountryOverride)
	}
}

func TestUpdateLatencies(t *testing.T) {
	dir := t.TempDir()
	srv, _ := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(dir)
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}

	servers := sm.GetServers()
	c0 := servers[0]
	c0.Latency = 11
	c2 := servers[2]
	c2.Latency = 33
	sm.UpdateLatencies([]models.Server{c0, c2})

	reloaded := NewSubscriptionManager(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := reloaded.GetServers()

	a := findByURI(got, uriA)
	if a == nil || a.Latency != 11 || a.LastChecked.IsZero() {
		t.Errorf("A: latency/lastChecked не сохранены: %+v", a)
	}
	c := findByURI(got, uriC)
	if c == nil || c.Latency != 33 || c.LastChecked.IsZero() {
		t.Errorf("C: latency/lastChecked не сохранены: %+v", c)
	}
	b := findByURI(got, uriB)
	if b == nil || !b.LastChecked.IsZero() {
		t.Errorf("B не должен был обновиться: %+v", b)
	}
}

func TestSetCountryOverride(t *testing.T) {
	srv, _ := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(t.TempDir())
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}

	if err := sm.SetCountryOverride(0, "nl "); err != nil {
		t.Fatalf("SetCountryOverride: %v", err)
	}
	if got := sm.GetServers()[0].CountryOverride; got != "NL" {
		t.Errorf("CountryOverride = %q, want NL (uppercase+trim)", got)
	}

	if err := sm.SetCountryOverride(99, "x"); err == nil {
		t.Error("ожидалась ошибка для id вне диапазона")
	}
	if err := sm.SetCountryOverride(-1, "x"); err == nil {
		t.Error("ожидалась ошибка для отрицательного id")
	}
}

func TestSelectNext(t *testing.T) {
	srv, _ := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(t.TempDir())
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}

	next, err := sm.SelectNext()
	if err != nil {
		t.Fatalf("SelectNext: %v", err)
	}
	if next.RawURI != uriB || sm.GetData().ActiveID != 1 {
		t.Errorf("после первого SelectNext ожидался B (id 1), got id %d", sm.GetData().ActiveID)
	}

	if _, err := sm.SelectNext(); err != nil {
		t.Fatalf("SelectNext: %v", err)
	}
	if sm.GetData().ActiveID != 2 {
		t.Errorf("ActiveID = %d, want 2", sm.GetData().ActiveID)
	}

	if _, err := sm.SelectNext(); err != nil {
		t.Fatalf("SelectNext: %v", err)
	}
	if sm.GetData().ActiveID != 0 {
		t.Errorf("ActiveID = %d, want 0 (wrap)", sm.GetData().ActiveID)
	}
}

func TestSetActiveBounds(t *testing.T) {
	srv, _ := subServer(t, body(uriA, uriB, uriC))
	sm := NewSubscriptionManager(t.TempDir())
	if _, err := sm.UpdateURL(srv.URL); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}

	if _, err := sm.SetActive(-1); err == nil {
		t.Error("ожидалась ошибка для id -1")
	}
	if _, err := sm.SetActive(3); err == nil {
		t.Error("ожидалась ошибка для id 3 (всего 3 сервера)")
	}
	if _, err := sm.SetActive(0); err != nil {
		t.Errorf("SetActive(0): неожиданная ошибка %v", err)
	}
}

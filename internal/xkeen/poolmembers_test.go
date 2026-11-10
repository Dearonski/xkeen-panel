package xkeen

import (
	"fmt"
	"testing"
	"time"

	"xkeen-panel/internal/models"
)

func manyServers(n int) []models.Server {
	var servers []models.Server
	for i := range n {
		server := vlessAt(fmt.Sprintf("s%d", i), "NL", fmt.Sprintf("h%d.example", i), 443)
		server.ID = i
		servers = append(servers, server)
	}
	return servers
}

func TestSelectPoolServersKeepsMembersWithinCap(t *testing.T) {
	servers := manyServers(30)
	member := NodeKeyOfServer(servers[25])

	got := SelectPoolServers(servers, PoolSelection{MaxNodes: 5, ProbeTimeout: 10 * time.Millisecond, Members: map[string]bool{member: true}})

	if len(got) != 5 {
		t.Fatalf("selected %d, want the cap of 5", len(got))
	}
	if NodeKeyOfServer(got[0]) != member {
		t.Errorf("first slot is %s, want the hand-picked member", got[0].Name)
	}
}

func TestSelectPoolServersGrowsForMembersBeyondCap(t *testing.T) {
	servers := manyServers(10)
	members := map[string]bool{}
	for _, server := range servers[:4] {
		members[NodeKeyOfServer(server)] = true
	}

	got := SelectPoolServers(servers, PoolSelection{MaxNodes: 2, ProbeTimeout: 10 * time.Millisecond, Members: members})

	if len(got) != 4 {
		t.Fatalf("selected %d, want all 4 members even past a cap of 2", len(got))
	}
	for _, server := range got {
		if !members[NodeKeyOfServer(server)] {
			t.Errorf("%s took a slot the members needed", server.Name)
		}
	}
}

func TestSelectPoolServersLetsMemberPastCountryFilter(t *testing.T) {
	ru := vlessAt("RU-1", "RU", "ru.example", 443)
	servers := []models.Server{vlessAt("NL-1", "NL", "nl.example", 443), ru}

	got := SelectPoolServers(servers, PoolSelection{AvoidCountries: []string{"RU"}, Members: map[string]bool{NodeKeyOfServer(ru): true}})

	if len(got) != 2 {
		t.Errorf("selected %d, want the owner's RU pick kept alongside NL", len(got))
	}
}

func TestMemberStorePersists(t *testing.T) {
	dir := t.TempDir()
	store := NewMemberStore(dir)
	if err := store.Set("a", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("b", true); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("a", false); err != nil {
		t.Fatal(err)
	}

	reloaded := NewMemberStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if keys := reloaded.Keys(); len(keys) != 1 || !keys["b"] {
		t.Errorf("reloaded %v, want only b", keys)
	}

	var missing *MemberStore
	if missing.Has("b") || missing.Keys() != nil {
		t.Error("a nil store must read as empty")
	}
}

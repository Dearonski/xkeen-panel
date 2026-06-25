// Package geoip читает geoip.dat формата Xray/V2Ray (protobuf) и отвечает на
// вопрос «находится ли IP в одной из избегаемых стран». Чтобы не держать в памяти
// всю базу (~18 МБ) на роутере, загружаются CIDR только нужных стран.
package geoip

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type v4range struct {
	start, end uint32
	cc         string
}

type v6entry struct {
	net *net.IPNet
	cc  string
}

type Matcher struct {
	v4 []v4range
	v6 []v6entry

	resolveMu    sync.Mutex
	resolveCache map[string]resolveEntry
}

type resolveEntry struct {
	ips []net.IP
	at  time.Time
}

const (
	resolveTTL    = time.Hour
	resolveNegTTL = time.Minute
)

// FindDat возвращает путь к существующему geoip.dat: сначала заданный в конфиге,
// затем стандартные кандидаты. "" если ничего не найдено.
func FindDat(configured string) string {
	candidates := []string{configured}
	if configured != "" {
		dir := filepath.Dir(configured)
		candidates = append(candidates,
			filepath.Join(dir, "geoip_v2fly.dat"),
			filepath.Join(dir, "geoip.dat"),
		)
	}
	candidates = append(candidates,
		"/opt/etc/xray/dat/geoip_v2fly.dat",
		"/opt/etc/xray/dat/geoip.dat",
	)
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// Load парсит geoip.dat и сохраняет диапазоны только для стран из avoidCodes.
func Load(path string, avoidCodes []string) (*Matcher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("чтение geoip.dat: %w", err)
	}

	want := make(map[string]bool, len(avoidCodes))
	for _, c := range avoidCodes {
		want[strings.ToUpper(strings.TrimSpace(c))] = true
	}

	m := &Matcher{resolveCache: make(map[string]resolveEntry)}

	// GeoIPList: repeated GeoIP entry = 1
	ok := walk(data, func(field, wire int, _ uint64, ld []byte) bool {
		if field == 1 && wire == 2 {
			cc := countryCodeOf(ld)
			if cc != "" && want[cc] {
				m.addEntry(ld, cc)
			}
		}
		return true
	})
	if !ok {
		return nil, fmt.Errorf("повреждённый geoip.dat")
	}

	sort.Slice(m.v4, func(i, j int) bool { return m.v4[i].start < m.v4[j].start })
	return m, nil
}

// countryCodeOf достаёт country_code (поле 1) из сообщения GeoIP, не парся CIDR.
func countryCodeOf(geoip []byte) string {
	var cc string
	walk(geoip, func(field, wire int, _ uint64, ld []byte) bool {
		if field == 1 && wire == 2 {
			cc = strings.ToUpper(string(ld))
			return false
		}
		return true
	})
	return cc
}

// addEntry парсит CIDR (поле 2) сообщения GeoIP и добавляет диапазоны.
func (m *Matcher) addEntry(geoip []byte, cc string) {
	walk(geoip, func(field, wire int, _ uint64, ld []byte) bool {
		if field == 2 && wire == 2 {
			m.addCIDR(ld, cc)
		}
		return true
	})
}

// addCIDR парсит CIDR{ ip=1 bytes, prefix=2 varint }.
func (m *Matcher) addCIDR(cidr []byte, cc string) {
	var ip []byte
	var prefix uint64
	walk(cidr, func(field, wire int, v uint64, ld []byte) bool {
		switch {
		case field == 1 && wire == 2:
			ip = ld
		case field == 2 && wire == 0:
			prefix = v
		}
		return true
	})

	switch len(ip) {
	case 4:
		p := int(prefix)
		if p > 32 {
			return
		}
		base := binary.BigEndian.Uint32(ip)
		var mask uint32
		if p == 0 {
			mask = 0
		} else {
			mask = ^uint32(0) << (32 - p)
		}
		start := base & mask
		m.v4 = append(m.v4, v4range{start: start, end: start | ^mask, cc: cc})
	case 16:
		p := int(prefix)
		if p > 128 {
			return
		}
		ipCopy := append([]byte(nil), ip...)
		n := &net.IPNet{IP: net.IP(ipCopy), Mask: net.CIDRMask(p, 128)}
		m.v6 = append(m.v6, v6entry{net: n, cc: cc})
	}
}

// Match возвращает код избегаемой страны, если IP в неё попадает.
func (m *Matcher) Match(ip net.IP) (string, bool) {
	if m == nil {
		return "", false
	}
	if ip4 := ip.To4(); ip4 != nil {
		v := binary.BigEndian.Uint32(ip4)
		idx := sort.Search(len(m.v4), func(i int) bool { return m.v4[i].start > v })
		if idx > 0 {
			r := m.v4[idx-1]
			if v >= r.start && v <= r.end {
				return r.cc, true
			}
		}
		return "", false
	}
	for _, e := range m.v6 {
		if e.net.Contains(ip) {
			return e.cc, true
		}
	}
	return "", false
}

// Inspect резолвит адрес (домен или IP) и проверяет на избегаемые страны.
// resolved=false означает «не удалось определить» (DNS не отвечает) — повод
// откатиться на эвристику по имени.
func (m *Matcher) Inspect(address string) (avoidCC string, resolved bool) {
	if m == nil {
		return "", false
	}
	ips := m.resolve(address)
	if len(ips) == 0 {
		return "", false
	}
	for _, ip := range ips {
		if cc, ok := m.Match(ip); ok {
			return cc, true
		}
	}
	return "", true
}

func (m *Matcher) resolve(address string) []net.IP {
	if ip := net.ParseIP(address); ip != nil {
		return []net.IP{ip}
	}

	m.resolveMu.Lock()
	if e, ok := m.resolveCache[address]; ok {
		ttl := resolveTTL
		if len(e.ips) == 0 {
			ttl = resolveNegTTL // негатив кэшируем коротко, чтобы не залипнуть на час
		}
		if time.Since(e.at) < ttl {
			m.resolveMu.Unlock()
			return e.ips
		}
	}
	m.resolveMu.Unlock()

	// Таймаут обязателен: resolve вызывается из watchdog-горутины во время
	// фейловера, а он часто срабатывает именно когда сеть/DNS недоступны.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addrs, _ := net.DefaultResolver.LookupIPAddr(ctx, address)
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}

	m.resolveMu.Lock()
	m.resolveCache[address] = resolveEntry{ips: ips, at: time.Now()}
	m.resolveMu.Unlock()
	return ips
}

// walk проходит protobuf-сообщение, вызывая fn для каждого поля. Для wire 0
// передаётся распарсенный varint, для wire 2 — байты значения.
func walk(b []byte, fn func(field, wire int, varint uint64, ld []byte) bool) bool {
	i := 0
	for i < len(b) {
		key, n := binary.Uvarint(b[i:])
		if n <= 0 {
			return false
		}
		i += n
		field := int(key >> 3)
		wire := int(key & 7)

		switch wire {
		case 0:
			v, n := binary.Uvarint(b[i:])
			if n <= 0 {
				return false
			}
			i += n
			if !fn(field, wire, v, nil) {
				return true
			}
		case 2:
			l, n := binary.Uvarint(b[i:])
			if n <= 0 {
				return false
			}
			i += n
			// Сравниваем как uint64 до конвертации в int — иначе i+int(l) может
			// переполнить знаковый int (особенно 32-битный на mipsle) и уйти в минус.
			if l > uint64(len(b)-i) {
				return false
			}
			end := i + int(l)
			if !fn(field, wire, 0, b[i:end]) {
				return true
			}
			i = end
		case 1:
			if i+8 > len(b) {
				return false
			}
			i += 8
		case 5:
			if i+4 > len(b) {
				return false
			}
			i += 4
		default:
			return false
		}
	}
	return true
}

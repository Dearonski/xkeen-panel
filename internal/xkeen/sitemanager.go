package xkeen

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// A node passing today can be banned tomorrow, so results expire even when the pool holds still
const siteRecheckEvery = 12 * time.Hour

// Keeps a check that keeps failing, say on a missing xray binary, from running every minute
const siteRetryAfter = 10 * time.Minute

type SiteManager struct {
	store         *SiteStore
	detector      *Detector
	subscription  *SubscriptionManager
	outboundsPath string
	apiAddr       string
	timeout       time.Duration

	Restart func()
	Notify  func()
	Logf    func(format string, args ...interface{})

	work      sync.Mutex
	mu        sync.Mutex
	checking  map[int]bool
	attempted map[int]time.Time
}

func NewSiteManager(store *SiteStore, det *Detector, sub *SubscriptionManager, outboundsPath, apiAddr string, timeout time.Duration) *SiteManager {
	return &SiteManager{
		store:         store,
		detector:      det,
		subscription:  sub,
		outboundsPath: outboundsPath,
		apiAddr:       apiAddr,
		timeout:       timeout,
		Restart:       func() {},
		Notify:        func() {},
		Logf:          func(string, ...interface{}) {},
		checking:      map[int]bool{},
		attempted:     map[int]time.Time{},
	}
}

// Unavailable explains why sites cannot work here, empty when they can.
func (m *SiteManager) Unavailable() string {
	if m.detector.Runtime().Core != CoreXray {
		return "Сайты работают только с ядром Xray"
	}
	if m.detector.Topology().Mode != TopologyPool {
		return "Сайты работают только в режиме пула"
	}
	return ""
}

func (m *SiteManager) Create(domainsInput []string, checkURL string) (Site, error) {
	if reason := m.Unavailable(); reason != "" {
		return Site{}, fmt.Errorf("%s", reason)
	}

	domains, err := ParseDomains(domainsInput)
	if err != nil {
		return Site{}, err
	}
	checkURL, err = NormalizeCheckURL(checkURL, domains)
	if err != nil {
		return Site{}, err
	}

	for _, existing := range m.store.List() {
		for _, d := range existing.Domains {
			for _, wanted := range domains {
				if d == wanted {
					return Site{}, fmt.Errorf("%s уже есть в списке сайтов", d)
				}
			}
		}
	}

	site, err := m.store.Add(Site{Domains: domains, CheckURL: checkURL})
	if err != nil {
		return Site{}, err
	}

	m.Logf("[SITES] Добавлен сайт %s, проверяю ноды пула", domains[0])
	m.CheckAsync(site.ID)

	return site, nil
}

func (m *SiteManager) Delete(id int) error {
	m.work.Lock()
	defer m.work.Unlock()

	site, ok := m.store.Get(id)
	if !ok {
		return fmt.Errorf("сайт не найден")
	}
	if err := m.store.Remove(id); err != nil {
		return err
	}
	if err := m.applyLocked(); err != nil {
		return err
	}

	m.Logf("[SITES] Сайт %s удалён", site.Domains[0])
	m.Notify()
	return nil
}

func (m *SiteManager) CheckAsync(id int) {
	m.setChecking(id, true)
	go m.check(id)
}

func (m *SiteManager) Checking(id int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checking[id]
}

func (m *SiteManager) setChecking(id int, on bool) {
	m.mu.Lock()
	if on {
		m.checking[id] = true
	} else {
		delete(m.checking, id)
	}
	m.mu.Unlock()
	m.Notify()
}

func (m *SiteManager) check(id int) {
	defer m.setChecking(id, false)

	m.work.Lock()
	defer m.work.Unlock()

	site, ok := m.store.Get(id)
	if !ok || m.Unavailable() != "" {
		return
	}

	top := m.detector.Topology()
	nodes, err := poolNodesInFile(m.outboundsPath, top.Selector())
	if err != nil {
		m.Logf("[SITES] Не удалось прочитать пул: %v", err)
		return
	}

	results, err := CheckSite(m.detector.Runtime(), nodes, site.CheckURL, m.timeout)
	if err != nil {
		m.Logf("[SITES] Проверка %s не выполнена: %v", site.Domains[0], err)
		return
	}

	site.Results = results
	site.CheckedAt = time.Now()
	if err := m.store.Put(site); err != nil {
		m.Logf("[SITES] Не удалось сохранить результат: %v", err)
		return
	}

	passing := len(site.Passing())
	if passing == 0 {
		m.Logf("[SITES] %s не открывается ни через одну ноду — оставляю весь пул", site.Domains[0])
	} else {
		m.Logf("[SITES] %s открывается через %d из %d нод", site.Domains[0], passing, len(results))
	}

	if err := m.applyLocked(); err != nil {
		m.Logf("[SITES] Не удалось применить: %v", err)
	}
}

func (m *SiteManager) applyLocked() error {
	top := m.detector.Topology()
	restart, err := ApplySites(m.detector.Runtime(), SiteApply{
		OutboundsPath: m.outboundsPath,
		Selector:      top.Selector(),
		PoolBalancer:  top.BalancerTag,
		APIAddr:       m.apiAddr,
	}, m.store.List())
	if err != nil {
		return err
	}

	m.detector.InvalidateTopology()
	if restart {
		m.Logf("[SITES] Маршрутизация изменилась — перезапускаю ядро")
		m.Restart()
	}
	return nil
}

// Run rechecks a site whenever the pool gained a node its results do not cover, and on a timer.
func (m *SiteManager) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if m.Unavailable() != "" {
			continue
		}

		layout, err := ReadPoolLayout(m.outboundsPath, m.detector.Topology().Selector())
		if err != nil {
			continue
		}

		for _, site := range m.store.List() {
			if m.Checking(site.ID) || time.Since(m.attempted[site.ID]) < siteRetryAfter {
				continue
			}
			if stale(site, layout) {
				m.attempted[site.ID] = time.Now()
				m.setChecking(site.ID, true)
				m.check(site.ID)
			}
		}
	}
}

func stale(site Site, layout PoolLayout) bool {
	if time.Since(site.CheckedAt) > siteRecheckEvery {
		return true
	}
	checked := site.CheckedNodes()
	for _, ep := range layout {
		if !checked[ep.Key()] {
			return true
		}
	}
	return false
}

type SiteView struct {
	ID        int            `json:"id"`
	Domains   []string       `json:"domains"`
	CheckURL  string         `json:"check_url"`
	CheckedAt *time.Time     `json:"checked_at,omitempty"`
	Checking  bool           `json:"checking"`
	Passing   int            `json:"passing"`
	Nodes     []SiteNodeView `json:"nodes"`
}

type SiteNodeView struct {
	Tag      string `json:"tag"`
	ServerID int    `json:"server_id"`
	Name     string `json:"name"`
	Country  string `json:"country,omitempty"`
	State    string `json:"state"` // ok | fail | unknown
	Status   int    `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
	Ms       int    `json:"ms,omitempty"`
}

// Views joins stored results with the pool as it is now; nodes added since the last check show as unknown.
func (m *SiteManager) Views() []SiteView {
	var nodes []PoolNode
	if top := m.detector.Topology(); top.Mode == TopologyPool {
		nodes, _ = PoolNodes(m.outboundsPath, top.Selector(), m.subscription.GetServers())
	}

	sites := m.store.List()
	views := make([]SiteView, 0, len(sites))
	for _, site := range sites {
		byNode := make(map[string]SiteResult, len(site.Results))
		for _, r := range site.Results {
			byNode[r.Node] = r
		}

		view := SiteView{
			ID:       site.ID,
			Domains:  site.Domains,
			CheckURL: site.CheckURL,
			Checking: m.Checking(site.ID),
			Nodes:    []SiteNodeView{},
		}
		if !site.CheckedAt.IsZero() {
			at := site.CheckedAt
			view.CheckedAt = &at
		}

		for _, node := range nodes {
			nv := SiteNodeView{Tag: node.Tag, ServerID: -1, Name: node.Address, State: "unknown"}
			if s := node.Server; s != nil {
				nv.ServerID, nv.Name, nv.Country = s.ID, s.Name, ServerCountry(*s)
			}
			if r, ok := byNode[endpoint{Address: node.Address, Port: node.Port, UUID: node.UUID}.Key()]; ok {
				nv.State, nv.Status, nv.Error, nv.Ms = "fail", r.Status, r.Error, r.Ms
				if r.OK {
					nv.State = "ok"
					view.Passing++
				}
			}
			view.Nodes = append(view.Nodes, nv)
		}

		sort.SliceStable(view.Nodes, func(i, j int) bool {
			return stateRank(view.Nodes[i].State) < stateRank(view.Nodes[j].State)
		})
		views = append(views, view)
	}

	return views
}

func stateRank(state string) int {
	switch state {
	case "ok":
		return 0
	case "unknown":
		return 1
	}
	return 2
}

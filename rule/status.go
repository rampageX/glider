package rule

import "time"

type ForwarderStatus struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
	URL string `json:"url"`
	Priority uint32 `json:"priority"`
	Failures uint32 `json:"failures"`
	LatencyMS int64 `json:"latency_ms"`
	Enabled bool `json:"enabled"`
	Current bool `json:"current"`
	Status string `json:"status"`
}

type GroupStatus struct {
	Name string `json:"name"`
	Strategy string `json:"strategy"`
	Priority uint32 `json:"priority"`
	EnabledCount int `json:"enabled_count"`
	TotalCount int `json:"total_count"`
	Status string `json:"status"`
	Current string `json:"current"`
	CurrentMS int64 `json:"current_latency_ms"`
	UpdatedAt time.Time `json:"updated_at"`
	Forwarders []ForwarderStatus `json:"forwarders"`
}

func (p *FwdrGroup) Snapshot() GroupStatus {
	p.mu.RLock(); defer p.mu.RUnlock()
	current := p.CurrentForwarder()
	currentName := ""; currentMS := int64(0)
	if current != nil { currentName = current.Name(); currentMS = time.Duration(current.Latency()).Milliseconds() }
	forwarders := make([]ForwarderStatus, 0, len(p.fwdrs)); enabledCount := 0
	for _, fwdr := range p.fwdrs {
		enabled := fwdr.Enabled(); status := "FAILED"
		if enabled { status = "SUCCESS"; enabledCount++ }
		forwarders = append(forwarders, ForwarderStatus{Name: fwdr.Name(), Addr: fwdr.Addr(), URL: fwdr.URL(), Priority: fwdr.Priority(), Failures: fwdr.Failures(), LatencyMS: time.Duration(fwdr.Latency()).Milliseconds(), Enabled: enabled, Current: current == fwdr, Status: status})
	}
	status := "FAILED"; if enabledCount > 0 { status = "SUCCESS" }
	return GroupStatus{Name: p.name, Strategy: p.Strategy(), Priority: p.Priority(), EnabledCount: enabledCount, TotalCount: len(p.fwdrs), Status: status, Current: currentName, CurrentMS: currentMS, UpdatedAt: time.Now(), Forwarders: forwarders}
}

package stats

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	AvailabilityBucketMinutes            = 15
	DefaultAvailabilityRetentionDays     = 30
	availabilityRetentionEnv             = "GLIDER_LHA_HISTORY_RETENTION_DAYS"
)

// AvailabilitySample stores the result of one LHA forwarder health check.
// The append-only JSONL file keeps history without adding a runtime dependency.
type AvailabilitySample struct {
	At        time.Time `json:"at"`
	Group     string    `json:"group"`
	Node      string    `json:"node"`
	Addr      string    `json:"addr"`
	Priority  uint32    `json:"priority"`
	Success   bool      `json:"success"`
	LatencyMS float64   `json:"latency_ms,omitempty"`
}

type AvailabilitySlot struct {
	Start        string  `json:"start"`
	End          string  `json:"end"`
	Checks       int     `json:"checks"`
	Successes    int     `json:"successes"`
	Failures     int     `json:"failures"`
	Availability float64 `json:"availability"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	JitterMS     float64 `json:"jitter_ms"`
}

type AvailabilityNodeDay struct {
	Name         string             `json:"name"`
	Addr         string             `json:"addr"`
	Priority     uint32             `json:"priority"`
	Checks       int                `json:"checks"`
	Successes    int                `json:"successes"`
	Failures     int                `json:"failures"`
	Availability float64            `json:"availability"`
	AvgLatencyMS float64            `json:"avg_latency_ms"`
	BestSlot     *AvailabilitySlot  `json:"best_slot,omitempty"`
	WorstSlot    *AvailabilitySlot  `json:"worst_slot,omitempty"`
	Slots        []AvailabilitySlot `json:"slots"`
}

type AvailabilityGroupDay struct {
	Name         string               `json:"name"`
	Checks       int                  `json:"checks"`
	Available    int                  `json:"available_windows"`
	Availability float64              `json:"availability"`
	Nodes        []AvailabilityNodeDay `json:"nodes"`
}

type AvailabilityDay struct {
	Date          string                 `json:"date"`
	Timezone      string                 `json:"timezone"`
	BucketMinutes int                    `json:"bucket_minutes"`
	Groups        []AvailabilityGroupDay `json:"groups"`
}

type AvailabilityStore struct {
	mu            sync.RWMutex
	dir           string
	fileDay       string
	file          *os.File
	retentionDays int
	lastPruneDay  string
}

func OpenAvailabilityStore(path string) (*AvailabilityStore, error) {
	return openAvailabilityStore(path, 0)
}

func openAvailabilityStore(path string, retentionDays int) (*AvailabilityStore, error) {
	if retentionDays < 0 {
		return nil, errors.New("availability history retention days cannot be negative")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("availability history path is empty")
	}
	dir, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	store := &AvailabilityStore{dir: dir, retentionDays: retentionDays}
	if retentionDays > 0 {
		now := time.Now()
		if err := pruneAvailabilityHistory(dir, retentionDays, now); err != nil {
			log.Printf("[lha-history] could not remove expired history files: %v", err)
		}
		store.lastPruneDay = now.In(time.Local).Format("2006-01-02")
	}
	return store, nil
}

func (s *AvailabilityStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

func (s *AvailabilityStore) Record(sample AvailabilitySample) error {
	if s == nil {
		return errors.New("availability history store is not initialized")
	}
	if sample.Group == "" || sample.Node == "" {
		return errors.New("availability sample requires group and node")
	}
	if sample.At.IsZero() {
		sample.At = time.Now()
	}
	dateKey := sample.At.In(time.Local).Format("2006-01-02")
	sample.At = sample.At.UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil || s.fileDay != dateKey {
		if s.file != nil {
			if err := s.file.Close(); err != nil {
				return err
			}
			s.file = nil
		}
		if s.retentionDays > 0 && s.lastPruneDay != dateKey {
			if err := pruneAvailabilityHistory(s.dir, s.retentionDays, sample.At.In(time.Local)); err != nil {
				log.Printf("[lha-history] could not remove expired history files: %v", err)
			}
			s.lastPruneDay = dateKey
		}
		filePath := filepath.Join(s.dir, dateKey+".jsonl")
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			s.file = nil
			s.fileDay = ""
			return err
		}
		s.file, s.fileDay = f, dateKey
	}
	encoded, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *AvailabilityStore) Day(day time.Time) AvailabilityDay {
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := day.AddDate(0, 0, 1)
	result := AvailabilityDay{
		Date: day.Format("2006-01-02"), Timezone: day.Location().String(),
		BucketMinutes: AvailabilityBucketMinutes, Groups: make([]AvailabilityGroupDay, 0),
	}
	if s == nil {
		return result
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	filePath := filepath.Join(s.dir, day.Format("2006-01-02")+".jsonl")
	f, err := os.Open(filePath)
	if err != nil {
		return result
	}
	defer f.Close()
	groupSamples := make(map[string][]AvailabilitySample)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		var sample AvailabilitySample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			continue
		}
		local := sample.At.In(day.Location())
		if sample.Group != "" && sample.Node != "" && !sample.At.IsZero() && !local.Before(day) && local.Before(dayEnd) {
			groupSamples[sample.Group] = append(groupSamples[sample.Group], sample)
		}
	}

	groupNames := make([]string, 0, len(groupSamples))
	for name := range groupSamples {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)
	for _, groupName := range groupNames {
		group := buildGroupDay(groupName, groupSamples[groupName], day)
		result.Groups = append(result.Groups, group)
	}
	return result
}

func buildGroupDay(name string, samples []AvailabilitySample, day time.Time) AvailabilityGroupDay {
	nodes := make(map[string][]AvailabilitySample)
	groupWindows := make(map[int]bool)
	for _, sample := range samples {
		key := sample.Node + "\x00" + sample.Addr
		nodes[key] = append(nodes[key], sample)
		index := slotIndex(sample.At.In(day.Location()), day)
		if sample.Success {
			groupWindows[index] = true
		} else if _, exists := groupWindows[index]; !exists {
			groupWindows[index] = false
		}
	}

	nodeKeys := make([]string, 0, len(nodes))
	for key := range nodes {
		nodeKeys = append(nodeKeys, key)
	}
	sort.Strings(nodeKeys)
	group := AvailabilityGroupDay{Name: name, Nodes: make([]AvailabilityNodeDay, 0, len(nodeKeys))}
	for _, key := range nodeKeys {
		group.Nodes = append(group.Nodes, buildNodeDay(nodes[key], day))
	}
	for _, available := range groupWindows {
		group.Checks++
		if available {
			group.Available++
		}
	}
	group.Availability = percentage(group.Available, group.Checks)
	return group
}

func buildNodeDay(samples []AvailabilitySample, day time.Time) AvailabilityNodeDay {
	node := AvailabilityNodeDay{Slots: make([]AvailabilitySlot, 0)}
	if len(samples) == 0 {
		return node
	}
	node.Name = samples[0].Node
	node.Addr = samples[0].Addr
	node.Priority = samples[0].Priority
	start := day
	dayEnd := day.AddDate(0, 0, 1)
	count := int(math.Ceil(dayEnd.Sub(start).Minutes() / AvailabilityBucketMinutes))
	if count < 1 {
		count = 96
	}
	buckets := make([][]AvailabilitySample, count)
	for _, sample := range samples {
		node.Checks++
		if sample.Success {
			node.Successes++
		} else {
			node.Failures++
		}
		if sample.Success && sample.LatencyMS > 0 {
			node.AvgLatencyMS += sample.LatencyMS
		}
		idx := slotIndex(sample.At.In(day.Location()), day)
		if idx >= 0 && idx < len(buckets) {
			buckets[idx] = append(buckets[idx], sample)
		}
	}
	node.Availability = percentage(node.Successes, node.Checks)
	latencyCount := 0
	for _, sample := range samples {
		if sample.Success && sample.LatencyMS > 0 {
			latencyCount++
		}
	}
	if latencyCount > 0 {
		node.AvgLatencyMS /= float64(latencyCount)
	}

	for i, bucket := range buckets {
		startAt := start.Add(time.Duration(i*AvailabilityBucketMinutes) * time.Minute)
		endAt := startAt.Add(time.Duration(AvailabilityBucketMinutes) * time.Minute)
		if endAt.After(dayEnd) {
			endAt = dayEnd
		}
		slot := AvailabilitySlot{Start: startAt.Format("15:04"), End: endAt.Format("15:04")}
		latencies := make([]float64, 0, len(bucket))
		for _, sample := range bucket {
			slot.Checks++
			if sample.Success {
				slot.Successes++
				if sample.LatencyMS > 0 {
					latencies = append(latencies, sample.LatencyMS)
				}
			} else {
				slot.Failures++
			}
		}
		if slot.Checks > 0 {
			slot.Availability = percentage(slot.Successes, slot.Checks)
			if len(latencies) > 0 {
				for _, latency := range latencies {
					slot.AvgLatencyMS += latency
				}
				slot.AvgLatencyMS /= float64(len(latencies))
				var variance float64
				for _, latency := range latencies {
					delta := latency - slot.AvgLatencyMS
					variance += delta * delta
				}
				slot.JitterMS = math.Sqrt(variance / float64(len(latencies)))
			}
		}
		node.Slots = append(node.Slots, slot)
	}
	observed := make([]AvailabilitySlot, 0)
	for _, slot := range node.Slots {
		if slot.Checks > 0 {
			observed = append(observed, slot)
		}
	}
	if len(observed) > 0 {
		best := append([]AvailabilitySlot(nil), observed...)
		sort.Slice(best, func(i, j int) bool {
			if best[i].Availability != best[j].Availability {
				return best[i].Availability > best[j].Availability
			}
			if best[i].JitterMS != best[j].JitterMS {
				return best[i].JitterMS < best[j].JitterMS
			}
			if best[i].AvgLatencyMS != best[j].AvgLatencyMS {
				return best[i].AvgLatencyMS < best[j].AvgLatencyMS
			}
			return best[i].Start < best[j].Start
		})
		worst := append([]AvailabilitySlot(nil), observed...)
		sort.Slice(worst, func(i, j int) bool {
			if worst[i].Availability != worst[j].Availability {
				return worst[i].Availability < worst[j].Availability
			}
			if worst[i].JitterMS != worst[j].JitterMS {
				return worst[i].JitterMS > worst[j].JitterMS
			}
			if worst[i].AvgLatencyMS != worst[j].AvgLatencyMS {
				return worst[i].AvgLatencyMS > worst[j].AvgLatencyMS
			}
			return worst[i].Start < worst[j].Start
		})
		bestSlot, worstSlot := best[0], worst[0]
		node.BestSlot, node.WorstSlot = &bestSlot, &worstSlot
	}
	return node
}

func slotIndex(at, day time.Time) int {
	minutes := at.Sub(day).Minutes()
	if minutes < 0 {
		return -1
	}
	return int(minutes / AvailabilityBucketMinutes)
}

func percentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round(float64(numerator)*10000/float64(denominator)) / 100
}

// pruneAvailabilityHistory keeps today and the previous retentionDays-1 local dates.
// It only removes regular files with exact YYYY-MM-DD.jsonl names.
func pruneAvailabilityHistory(dir string, retentionDays int, now time.Time) error {
	if retentionDays <= 0 {
		return nil
	}
	today := now.In(time.Local)
	cutoff := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -(retentionDays - 1))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".jsonl" {
			continue
		}
		dateText := strings.TrimSuffix(name, ".jsonl")
		fileDate, err := time.ParseInLocation("2006-01-02", dateText, time.Local)
		if err != nil || !fileDate.Before(cutoff) {
			continue
		}
		filePath := filepath.Join(dir, name)
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return nil
}

var defaultAvailabilityStore struct {
	sync.RWMutex
	store *AvailabilityStore
}

func availabilityHistoryDirectory(defaultDirectory, instanceID string) string {
	name := "glider-lha-history"
	if strings.TrimSpace(instanceID) != "" {
		name += "-" + instanceID
	}
	return filepath.Join(defaultDirectory, name)
}

// InitAvailabilityStore loads persisted LHA history. The path can be overridden
// with GLIDER_LHA_HISTORY_PATH; otherwise each instance gets a stable, separate directory.
func InitAvailabilityStore(defaultDirectory, instanceID string) error {
	path := strings.TrimSpace(os.Getenv("GLIDER_LHA_HISTORY_PATH"))
	if path == "" {
		if strings.TrimSpace(defaultDirectory) == "" {
			defaultDirectory = "."
		}
		path = availabilityHistoryDirectory(defaultDirectory, instanceID)
	}
	retentionDays := DefaultAvailabilityRetentionDays
	if raw := strings.TrimSpace(os.Getenv(availabilityRetentionEnv)); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 0 {
			log.Printf("[lha-history] invalid %s value %q; using the default of %d days", availabilityRetentionEnv, raw, DefaultAvailabilityRetentionDays)
		} else {
			retentionDays = days
		}
	}
	store, err := openAvailabilityStore(path, retentionDays)
	if err != nil {
		return fmt.Errorf("open LHA history: %w", err)
	}
	defaultAvailabilityStore.Lock()
	old := defaultAvailabilityStore.store
	defaultAvailabilityStore.store = store
	defaultAvailabilityStore.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func RecordLHAAvailability(sample AvailabilitySample) error {
	defaultAvailabilityStore.RLock()
	store := defaultAvailabilityStore.store
	defaultAvailabilityStore.RUnlock()
	if store == nil {
		// The proxy keeps running when the history directory is unavailable.
		// InitAvailabilityStore reports the startup error once.
		return nil
	}
	return store.Record(sample)
}

func AvailabilityForDay(day time.Time) AvailabilityDay {
	defaultAvailabilityStore.RLock()
	store := defaultAvailabilityStore.store
	defaultAvailabilityStore.RUnlock()
	return store.Day(day)
}

func CloseAvailabilityStore() error {
	defaultAvailabilityStore.Lock()
	store := defaultAvailabilityStore.store
	defaultAvailabilityStore.store = nil
	defaultAvailabilityStore.Unlock()
	return store.Close()
}

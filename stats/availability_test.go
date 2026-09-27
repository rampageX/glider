package stats

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAvailabilityStorePersistsAndBuildsDailyTimeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lha-history")
	store, err := OpenAvailabilityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	samples := []AvailabilitySample{
		{At: day.Add(2 * time.Minute), Group: "main", Node: "node-a", Addr: "a:443", Priority: 10, Success: true, LatencyMS: 50},
		{At: day.Add(10 * time.Minute), Group: "main", Node: "node-a", Addr: "a:443", Priority: 10, Success: false},
		{At: day.Add(5 * time.Minute), Group: "main", Node: "node-b", Addr: "b:443", Priority: 10, Success: true, LatencyMS: 25},
		{At: day.Add(61 * time.Minute), Group: "main", Node: "node-a", Addr: "a:443", Priority: 10, Success: true, LatencyMS: 30},
		{At: day.Add(65 * time.Minute), Group: "main", Node: "node-a", Addr: "a:443", Priority: 10, Success: true, LatencyMS: 40},
	}
	for _, sample := range samples {
		if err := store.Record(sample); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenAvailabilityStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result := store.Day(day)
	if result.Date != "2026-09-26" || len(result.Groups) != 1 {
		t.Fatalf("unexpected day summary: %+v", result)
	}
	group := result.Groups[0]
	if group.Checks != 2 || group.Available != 2 || group.Availability != 100 {
		t.Fatalf("group should be available in both observed windows: %+v", group)
	}
	if len(group.Nodes) != 2 {
		t.Fatalf("expected two nodes, got %d", len(group.Nodes))
	}
	var nodeA AvailabilityNodeDay
	for _, node := range group.Nodes {
		if node.Name == "node-a" {
			nodeA = node
		}
	}
	if nodeA.Checks != 4 || nodeA.Successes != 3 || nodeA.Availability != 75 {
		t.Fatalf("unexpected node availability: %+v", nodeA)
	}
	if len(nodeA.Slots) != 96 || nodeA.Slots[0].Checks != 2 || nodeA.Slots[4].Checks != 2 {
		t.Fatalf("unexpected 15-minute buckets: first=%+v fifth=%+v", nodeA.Slots[0], nodeA.Slots[4])
	}
	if nodeA.BestSlot == nil || nodeA.WorstSlot == nil || nodeA.BestSlot.Availability != 100 || nodeA.WorstSlot.Availability != 50 {
		t.Fatalf("unexpected stable/unstable windows: best=%+v worst=%+v", nodeA.BestSlot, nodeA.WorstSlot)
	}
}

func TestAvailabilityForDayExcludesOtherDates(t *testing.T) {
	store, err := OpenAvailabilityStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	day := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	if err := store.Record(AvailabilitySample{At: day.Add(-time.Second), Group: "main", Node: "node", Addr: "n:443", Success: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(AvailabilitySample{At: day.Add(time.Second), Group: "main", Node: "node", Addr: "n:443", Success: true}); err != nil {
		t.Fatal(err)
	}
	result := store.Day(day)
	if len(result.Groups) != 1 || result.Groups[0].Nodes[0].Checks != 1 {
		t.Fatalf("expected only the requested date's sample: %+v", result)
	}
}

func TestAvailabilityHistoryDirectorySeparatesInstances(t *testing.T) {
	root := t.TempDir()
	first := availabilityHistoryDirectory(root, "a1b2")
	second := availabilityHistoryDirectory(root, "c3d4")
	if first == second || filepath.Base(first) != "glider-lha-history-a1b2" || filepath.Base(second) != "glider-lha-history-c3d4" {
		t.Fatalf("instance history directories are not isolated: %q, %q", first, second)
	}
	if got := availabilityHistoryDirectory(root, ""); filepath.Base(got) != "glider-lha-history" {
		t.Fatalf("unexpected fallback directory: %q", got)
	}
}

func TestPruneAvailabilityHistoryKeepsConfiguredLocalDates(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	for _, name := range []string{"2026-09-24.jsonl", "2026-09-25.jsonl", "2026-09-26.jsonl", "2026-09-27.jsonl", "notes.jsonl", "2026-09-01.jsonl.bak"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "2026-09-01.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := pruneAvailabilityHistory(dir, 3, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2026-09-24.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("expired date file %q was not removed: %v", name, err)
		}
	}
	for _, name := range []string{"2026-09-25.jsonl", "2026-09-26.jsonl", "2026-09-27.jsonl", "notes.jsonl", "2026-09-01.jsonl.bak", "2026-09-01.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("file outside retention cleanup scope %q is missing: %v", name, err)
		}
	}
}

func TestPruneAvailabilityHistoryZeroDisablesCleanup(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "2020-01-01.jsonl")
	if err := os.WriteFile(oldFile, []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pruneAvailabilityHistory(dir, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("zero retention setting should disable cleanup: %v", err)
	}
}

func TestInitAvailabilityStoreAppliesRetentionSetting(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "2000-01-01.jsonl")
	if err := os.WriteFile(oldFile, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLIDER_LHA_HISTORY_PATH", dir)
	t.Setenv("GLIDER_LHA_HISTORY_RETENTION_DAYS", "2")
	if err := InitAvailabilityStore("", "test"); err != nil {
		t.Fatal(err)
	}
	defer CloseAvailabilityStore()
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("startup should remove files outside the configured retention: %v", err)
	}
}

func TestAvailabilityStorePrunesAtDayRollover(t *testing.T) {
	dir := t.TempDir()
	store, err := openAvailabilityStore(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	oldFile := filepath.Join(dir, "2000-01-01.jsonl")
	if err := os.WriteFile(oldFile, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().In(time.Local).AddDate(0, 0, 1)
	if err := store.Record(AvailabilitySample{At: future, Group: "main", Node: "node", Success: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("day rollover should remove expired history: %v", err)
	}
}

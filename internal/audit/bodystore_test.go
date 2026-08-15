package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
)

// timeNowMinusHours returns a time offset into the past (eviction-ordering
// fixture).
func timeNowMinusHours(h int) time.Time { return time.Now().Add(-time.Duration(h) * time.Hour) }

// bodySecret is a synthetic secret-carrier value pinned by the redaction test.
const bodySecret = "sk-live-secret-0123456789abcdef"

// secretBody builds a representative request body carrying a secret.
func secretBody() []byte {
	return []byte(`{"model":"glm-5.2","api_key":"` + bodySecret + `","system":[{"type":"text","text":"sys"}]}`)
}

// TestBodyStore_PutRetrieveRedacted (09-05 T1 Test 1): Put returns the sha256
// hex ref; the file exists at <dir>/<ref[:2]>/<ref>.json holding the REDACTED
// body (field name preserved, secret value gone).
func TestBodyStore_PutRetrieveRedacted(t *testing.T) {
	t.Parallel()

	s := audit.NewBodyStore(t.TempDir(), 0)

	ref, err := s.Put(secretBody())
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if len(ref) != 64 { // sha256 hex
		t.Fatalf("ref = %q; want 64-char sha256 hex", ref)
	}

	path := filepath.Join(s.Dir(), ref[:2], ref+".json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("stored file missing: %v", err)
	}

	if strings.Contains(string(raw), bodySecret) {
		t.Error("stored body leaks the secret value (redact-before-store failed)")
	}

	if !strings.Contains(string(raw), "api_key") {
		t.Error("field name api_key not preserved (D-03 structural shape)")
	}
}

// TestBodyStore_Dedup (Test 2): same body → same ref, one file.
func TestBodyStore_Dedup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := audit.NewBodyStore(dir, 0)

	r1, err := s.Put(secretBody())
	if err != nil {
		t.Fatalf("Put 1: %v", err)
	}

	r2, err := s.Put(secretBody())
	if err != nil {
		t.Fatalf("Put 2: %v", err)
	}

	if r1 != r2 {
		t.Fatalf("refs differ: %q vs %q (content-addressed dedup broken)", r1, r2)
	}

	count := 0

	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
			count++
		}

		return nil
	})

	if count != 1 {
		t.Fatalf("stored files = %d; want exactly 1", count)
	}
}

// TestBodyStore_CapEvictionOldest (Test 3): an 8 KiB cap fed three ~4 KiB
// bodies evicts the OLDEST (mtime via os.Chtimes); newest retrievable, evicted
// returns not-found.
func TestBodyStore_CapEvictionOldest(t *testing.T) {
	t.Parallel()

	s := audit.NewBodyStore(t.TempDir(), 8*1024)

	mkB := func(fill string) []byte {
		b, _ := json.Marshal(map[string]any{"model": "m", "fill": strings.Repeat(fill, 4096)})

		return b
	}

	refA, err := s.Put(mkB("a"))
	if err != nil {
		t.Fatalf("Put A: %v", err)
	}

	refB, err := s.Put(mkB("b"))
	if err != nil {
		t.Fatalf("Put B: %v", err)
	}

	// Age A beyond B (mtime-based eviction).
	old := timeNowMinusHours(2)

	_ = os.Chtimes(filepath.Join(s.Dir(), refA[:2], refA+".json"), old, old)

	refC, err := s.Put(mkB("c"))
	if err != nil {
		t.Fatalf("Put C: %v", err)
	}

	if _, err := s.Get(refC); err != nil {
		t.Errorf("newest not retrievable after eviction: %v", err)
	}

	if _, err := s.Get(refB); err != nil {
		t.Errorf("B should survive (A is older): %v", err)
	}

	if _, err := s.Get(refA); err == nil {
		t.Error("oldest (A) was not evicted under the cap")
	}
}

// TestBodyStore_UnwritableNeverFatal (Test 4): Put over an unwritable dir
// returns the error (caller logs); no panic.
func TestBodyStore_UnwritableNeverFatal(t *testing.T) {
	t.Parallel()

	s := audit.NewBodyStore(filepath.Join(t.TempDir(), "no", "such", "dir"), 0)

	_, err := s.Put(secretBody())
	if err == nil {
		t.Fatal("Put over an unwritable dir must return an error (loud, never silent)")
	}
}

// TestBodyStore_ConcurrentPuts (Test 5): parallel Puts stay consistent under
// -race (internal mutex; per-key single writer).
func TestBodyStore_ConcurrentPuts(t *testing.T) {
	t.Parallel()

	s := audit.NewBodyStore(t.TempDir(), 0)

	var wg sync.WaitGroup

	refs := make(chan string, 32)

	for i := range 32 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			body, _ := json.Marshal(map[string]any{"model": "m", "i": i})
			ref, err := s.Put(body)
			if err != nil {
				t.Errorf("Put %d: %v", i, err)

				return
			}

			refs <- ref
		}(i)
	}

	wg.Wait()
	close(refs)

	seen := map[string]bool{}

	for ref := range refs {
		if seen[ref] {
			t.Error("duplicate ref for distinct bodies")
		}

		seen[ref] = true

		if _, err := s.Get(ref); err != nil {
			t.Errorf("Get %s after concurrent Puts: %v", ref, err)
		}
	}
}

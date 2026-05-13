package logbook

import (
	"sync"
	"testing"
)

func TestLockDocEvictsEntryWhenLastHolderReleases(t *testing.T) {
	s := &IngestionService{docLocks: make(map[string]*docLockEntry)}

	release := s.lockDoc("doc-1")
	if got := len(s.docLocks); got != 1 {
		t.Fatalf("after acquire, want len(docLocks)=1, got %d", got)
	}
	release()
	if got := len(s.docLocks); got != 0 {
		t.Fatalf("after release, want len(docLocks)=0, got %d", got)
	}
}

func TestLockDocManyDistinctDocsDoNotAccumulate(t *testing.T) {
	s := &IngestionService{docLocks: make(map[string]*docLockEntry)}

	for i := 0; i < 1000; i++ {
		release := s.lockDoc("doc")
		release()
	}
	if got := len(s.docLocks); got != 0 {
		t.Fatalf("after 1000 sequential acquire/release pairs, want len(docLocks)=0, got %d", got)
	}

	for i := 0; i < 1000; i++ {
		docID := "doc-" + string(rune('A'+(i%26))) + "-" + itoa(i)
		release := s.lockDoc(docID)
		release()
	}
	if got := len(s.docLocks); got != 0 {
		t.Fatalf("after 1000 distinct docID acquire/release pairs, want len(docLocks)=0, got %d", got)
	}
}

func TestLockDocSerializesConcurrentHoldersAndCleansUp(t *testing.T) {
	s := &IngestionService{docLocks: make(map[string]*docLockEntry)}

	const concurrency = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	var inCrit int
	var maxInCrit int

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := s.lockDoc("hot-doc")
			defer release()
			mu.Lock()
			inCrit++
			if inCrit > maxInCrit {
				maxInCrit = inCrit
			}
			mu.Unlock()
			mu.Lock()
			inCrit--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if maxInCrit != 1 {
		t.Fatalf("lock did not serialize: maxInCrit=%d, want 1", maxInCrit)
	}
	if got := len(s.docLocks); got != 0 {
		t.Fatalf("after concurrent acquire/release, want len(docLocks)=0, got %d", got)
	}
}

// itoa avoids importing strconv just for a test helper.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

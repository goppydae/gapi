package clock

import (
	"sync"
	"testing"
	"time"
)

// #11: MockClock must be safe for concurrent use. Run with -race.
func TestMockClock_ConcurrentAccess(t *testing.T) {
	mc := &MockClock{CurrentTime: time.Unix(0, 0)}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			mc.Advance(time.Second)
		}()
		go func() {
			defer wg.Done()
			_ = mc.Now()
			_ = mc.Since(time.Unix(0, 0))
			_ = mc.After(time.Millisecond)
		}()
	}
	wg.Wait()

	if got := mc.Now().Unix(); got != 50 {
		t.Fatalf("expected 50s advanced, got %ds", got)
	}
}

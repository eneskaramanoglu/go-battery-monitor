package drivers

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/eneskaramanoglu/go-battery-monitor/internal/device"
)

// lastKnown remembers each device's last fresh reading, so a sleeping device can keep
// showing it (dimmed) for a while instead of disappearing.
type lastKnown struct {
	mu sync.Mutex
	m  map[string]lastEntry
}

type lastEntry struct {
	s  device.Status
	at time.Time
}

// staleAfter: a reading older than this is dropped rather than shown as "asleep".
const staleAfter = 6 * time.Hour

func (l *lastKnown) set(key string, s device.Status) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]lastEntry{}
	}
	l.m[key] = lastEntry{s, time.Now()}
}

func (l *lastKnown) get(key string) (device.Status, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.m[key]
	if !ok || time.Since(e.at) > staleAfter {
		return device.Status{}, false
	}
	return e.s, true
}

var compxLast lastKnown

func hex4(v uint16) string { return fmt.Sprintf("%04X", v) }

// shortID turns a device path into a short stable id, so two identical devices get
// different keys.
func shortID(path string) string {
	h := fnv.New32a()
	h.Write([]byte(path))
	return fmt.Sprintf("%08x", h.Sum32())
}

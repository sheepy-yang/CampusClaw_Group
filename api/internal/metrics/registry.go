package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type aggregate struct {
	Count      uint64
	Failures   uint64
	DurationMS uint64
}

var registry = struct {
	sync.RWMutex
	HTTP       map[string]aggregate
	Skills     map[string]aggregate
	QueueDepth int
}{
	HTTP:   map[string]aggregate{},
	Skills: map[string]aggregate{},
}

func ObserveHTTP(method, route string, status int, duration time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	key := strings.ToUpper(method) + "|" + route + "|" + fmt.Sprint(status)
	registry.Lock()
	value := registry.HTTP[key]
	value.Count++
	if status >= 500 {
		value.Failures++
	}
	value.DurationMS += uint64(max(duration.Milliseconds(), 0))
	registry.HTTP[key] = value
	registry.Unlock()
}

func ObserveSkill(skill, result string, duration time.Duration) {
	if skill == "" {
		return
	}
	key := skill + "|" + normalizeResult(result)
	registry.Lock()
	value := registry.Skills[key]
	value.Count++
	if result == "failed" || result == "rejected" || result == "denied" {
		value.Failures++
	}
	value.DurationMS += uint64(max(duration.Milliseconds(), 0))
	registry.Skills[key] = value
	registry.Unlock()
}

func SetQueueDepth(depth int) {
	if depth < 0 {
		depth = 0
	}
	registry.Lock()
	registry.QueueDepth = depth
	registry.Unlock()
}

func WritePrometheus(w io.Writer) {
	registry.RLock()
	defer registry.RUnlock()
	_, _ = io.WriteString(w, "# HELP campusclaw_http_requests_total HTTP requests by bounded route and status.\n")
	_, _ = io.WriteString(w, "# TYPE campusclaw_http_requests_total counter\n")
	httpKeys := sortedKeys(registry.HTTP)
	for _, key := range httpKeys {
		parts := strings.SplitN(key, "|", 3)
		value := registry.HTTP[key]
		_, _ = fmt.Fprintf(w, "campusclaw_http_requests_total{method=%q,route=%q,status=%q} %d\n", parts[0], parts[1], parts[2], value.Count)
		_, _ = fmt.Fprintf(w, "campusclaw_http_request_duration_ms_sum{method=%q,route=%q,status=%q} %d\n", parts[0], parts[1], parts[2], value.DurationMS)
	}
	_, _ = io.WriteString(w, "# HELP campusclaw_skill_invocations_total Skill invocations by skill and bounded result.\n")
	_, _ = io.WriteString(w, "# TYPE campusclaw_skill_invocations_total counter\n")
	skillKeys := sortedKeys(registry.Skills)
	for _, key := range skillKeys {
		parts := strings.SplitN(key, "|", 2)
		value := registry.Skills[key]
		_, _ = fmt.Fprintf(w, "campusclaw_skill_invocations_total{skill=%q,result=%q} %d\n", parts[0], parts[1], value.Count)
		_, _ = fmt.Fprintf(w, "campusclaw_skill_duration_ms_sum{skill=%q,result=%q} %d\n", parts[0], parts[1], value.DurationMS)
	}
	_, _ = fmt.Fprintf(w, "# HELP campusclaw_queue_depth Current in-process queue depth.\n# TYPE campusclaw_queue_depth gauge\ncampusclaw_queue_depth %d\n", registry.QueueDepth)
}

func normalizeResult(result string) string {
	switch result {
	case "succeeded", "accepted", "failed", "rejected", "denied":
		return result
	default:
		return "other"
	}
}

func sortedKeys(values map[string]aggregate) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

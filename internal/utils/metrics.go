package utils

import (
	"sync"
	"time"
)

// Tracks performance metrics across the system
type MetricsCollector struct {
	mu           sync.RWMutex
	requestCount uint64
	errorCount   uint64

	// Maps operation name to list of latencies in nanoseconds
	operationTimes map[string][]int64

	systemStartTime time.Time
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		operationTimes:  make(map[string][]int64),
		systemStartTime: time.Now(),
	}
}

func (mc *MetricsCollector) IncrementRequests() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.requestCount++
}

func (mc *MetricsCollector) IncrementErrors() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.errorCount++
}

func (mc *MetricsCollector) AddOperationLatency(operationName string, duration time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if _, exists := mc.operationTimes[operationName]; !exists {
		mc.operationTimes[operationName] = make([]int64, 0)
	}
	mc.operationTimes[operationName] = append(
		mc.operationTimes[operationName],
		duration.Nanoseconds(),
	)
}

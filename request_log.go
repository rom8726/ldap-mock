package main

import (
	"sync"
	"time"
)

const (
	DefaultRequestLogCapacity = 1000
)

type LDAPRequestLog struct {
	Timestamp   time.Time       `json:"timestamp"`
	RequestID   string          `json:"request_id"`
	Type        string          `json:"type"`
	BaseDN      string          `json:"base_dn"`
	Scope       string          `json:"scope"`
	Filter      string          `json:"filter"`
	Attributes  []string        `json:"attributes,omitempty"`
	MatchedRule *MatchedRuleLog `json:"matched_rule,omitempty"`
	Response    LDAPResponseLog `json:"response"`
}

type MatchedRuleLog struct {
	RuleID   string `json:"id"`
	RuleName string `json:"name"`
}

type LDAPResponseLog struct {
	ReturnedDNs []string `json:"returned_dns"`
	Count       int      `json:"count"`
}

type RequestLogger interface {
	Log(req LDAPRequestLog)
	List() []LDAPRequestLog
	Clear()
}

type InMemoryRequestLogger struct {
	mu       sync.Mutex
	buffer   []LDAPRequestLog
	head     int
	count    int
	capacity int
}

func NewInMemoryRequestLogger(capacity int) *InMemoryRequestLogger {
	if capacity <= 0 {
		capacity = DefaultRequestLogCapacity
	}

	return &InMemoryRequestLogger{
		buffer:   make([]LDAPRequestLog, capacity),
		capacity: capacity,
	}
}

func (l *InMemoryRequestLogger) Log(req LDAPRequestLog) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.capacity == 0 {
		return
	}

	entry := cloneRequestLog(req)

	if l.count < l.capacity {
		idx := (l.head + l.count) % l.capacity
		l.buffer[idx] = entry
		l.count++
		return
	}

	l.buffer[l.head] = entry
	l.head = (l.head + 1) % l.capacity
}

func (l *InMemoryRequestLogger) List() []LDAPRequestLog {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.count == 0 {
		return nil
	}

	result := make([]LDAPRequestLog, 0, l.count)

	for i := 0; i < l.count; i++ {
		idx := (l.head + l.count - 1 - i + l.capacity) % l.capacity
		result = append(result, cloneRequestLog(l.buffer[idx]))
	}

	return result
}

func (l *InMemoryRequestLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.head = 0
	l.count = 0
}

func cloneRequestLog(src LDAPRequestLog) LDAPRequestLog {
	dst := src

	if src.Attributes != nil {
		dst.Attributes = append([]string(nil), src.Attributes...)
	}

	if src.Response.ReturnedDNs != nil {
		dst.Response.ReturnedDNs = append([]string(nil), src.Response.ReturnedDNs...)
	}

	return dst
}

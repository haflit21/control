// Package ha is the Home Assistant side: entity states kept in memory,
// fed either by the WebSocket client (client.go) or by the demo generator
// (internal/demo). Everything that comes from HA is untrusted data: it is
// stored, normalised by internal/build, and escaped by the interface.
package ha

import (
	"context"
	"io"
	"sort"
	"sync"
	"time"
)

// Entity is one HA state object, as received.
type Entity struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
	LastUpdated time.Time      `json:"last_updated"`
}

// Forecast is one weather forecast list (hourly or daily) for an entity.
type Forecast struct {
	Entity string
	Kind   string // hourly | daily
	Items  []map[string]any
	At     time.Time
}

// Source is what the server needs from HA, real or simulated.
type Source interface {
	// Snapshot returns a copy of every known entity, by id.
	Snapshot() map[string]Entity
	// Forecasts returns the cached forecasts (kind → list) for a weather entity.
	Forecasts(entity string) map[string][]map[string]any
	// Subscribe returns a channel that receives a tick after every batch of
	// changes; the receiver then reads Snapshot(). Cancel with the func.
	Subscribe() (<-chan struct{}, func())
	// Connected says whether HA is reachable right now (always true in demo).
	Connected() bool
	// CallService runs a HA service (domain.service) with data.
	CallService(ctx context.Context, domain, service string, data map[string]any) error
	// CameraSnapshot returns one JPEG for a camera entity.
	CameraSnapshot(ctx context.Context, entity string) (body []byte, contentType string, err error)
	// CameraStream copies the MJPEG stream to w until ctx ends.
	CameraStream(ctx context.Context, entity string, w io.Writer, setHeader func(contentType string)) error
}

// Store is the shared in-memory state used by both sources.
type Store struct {
	mu        sync.RWMutex
	entities  map[string]Entity
	forecasts map[string]map[string][]map[string]any
	connected bool
	subs      map[int]chan struct{}
	nextSub   int
}

func NewStore() *Store {
	return &Store{entities: map[string]Entity{}, forecasts: map[string]map[string][]map[string]any{}, subs: map[int]chan struct{}{}}
}

func (s *Store) Snapshot() map[string]Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Entity, len(s.entities))
	for k, v := range s.entities {
		out[k] = v
	}
	return out
}

func (s *Store) Forecasts(entity string) map[string][]map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string][]map[string]any{}
	for k, v := range s.forecasts[entity] {
		out[k] = v
	}
	return out
}

func (s *Store) Connected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// Set replaces entities (nil-safe) and wakes subscribers.
func (s *Store) Set(entities ...Entity) {
	s.mu.Lock()
	for _, e := range entities {
		if e.EntityID != "" {
			s.entities[e.EntityID] = e
		}
	}
	s.mu.Unlock()
	s.notify()
}

// Replace drops every entity and installs the given set (initial get_states).
func (s *Store) Replace(entities []Entity) {
	s.mu.Lock()
	s.entities = make(map[string]Entity, len(entities))
	for _, e := range entities {
		s.entities[e.EntityID] = e
	}
	s.mu.Unlock()
	s.notify()
}

func (s *Store) SetForecast(entity, kind string, items []map[string]any) {
	s.mu.Lock()
	if s.forecasts[entity] == nil {
		s.forecasts[entity] = map[string][]map[string]any{}
	}
	s.forecasts[entity][kind] = items
	s.mu.Unlock()
	s.notify()
}

func (s *Store) SetConnected(v bool) {
	s.mu.Lock()
	changed := s.connected != v
	s.connected = v
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

func (s *Store) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

func (s *Store) notify() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default: // a tick is already pending
		}
	}
}

// IDs returns the known entity ids, sorted (tests, diagnostics).
func (s *Store) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.entities))
	for k := range s.entities {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

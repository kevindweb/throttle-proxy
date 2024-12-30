// Package util holds custom structs and functions to handle common operations
package util

import "sync"

func MapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// SyncMap is a typed sync.Map implementation
type SyncMap[K comparable, V comparable] struct {
	mu    sync.RWMutex
	items map[K]V
}

// NewSyncMap creates a new typed concurrent map
func NewSyncMap[K comparable, V comparable]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		items: map[K]V{},
	}
}

// Store sets the value for a key
func (m *SyncMap[K, V]) Store(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
}

// Range calls f sequentially for each key and value in the map.
// If f returns false, range stops the iteration.
func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.items {
		if !f(k, v) {
			break
		}
	}
}

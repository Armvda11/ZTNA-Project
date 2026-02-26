// Package decisioncache implémente un cache local de décisions d'autorisation.
//
// Il est prévu pour réduire la charge CP et amortir de courtes indisponibilités,
// sans changer le comportement actuel tant qu'il n'est pas explicitement utilisé.
package decisioncache

import (
	"sync"
	"time"
)

// DecisionEntry représente une décision sérialisable dans le cache.
type DecisionEntry struct {
	Decision      string
	Reason        string
	PolicyVersion int64
}

type item struct {
	entry   DecisionEntry
	expires time.Time
}

// Cache stocke des décisions à durée de vie courte.
type Cache struct {
	mu         sync.RWMutex
	items      map[string]item
	maxEntries int
}

// New crée un cache borné.
func New(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	return &Cache{items: make(map[string]item), maxEntries: maxEntries}
}

// Get lit une entrée non expirée.
func (c *Cache) Get(key string, now time.Time) (DecisionEntry, bool) {
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return DecisionEntry{}, false
	}
	if now.After(it.expires) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return DecisionEntry{}, false
	}
	return it.entry, true
}

// Put écrit une entrée avec TTL.
func (c *Cache) Put(key string, entry DecisionEntry, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.maxEntries {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
	c.items[key] = item{entry: entry, expires: time.Now().Add(ttl)}
}

// Clear vide le cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]item, len(c.items))
	c.mu.Unlock()
}

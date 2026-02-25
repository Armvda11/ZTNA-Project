package decisioncache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"ztna-gateway/internal/pep"
)

type entry struct {
	decision pep.AuthorizeResponse
	expires  time.Time
}

// Cache stores short-lived PEP decisions to reduce CP load and latency.
type Cache struct {
	mu             sync.RWMutex
	items          map[string]entry
	maxEntries     int
	lastPolicyVers int64
}

func New(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	return &Cache{
		items:      make(map[string]entry),
		maxEntries: maxEntries,
	}
}

func Key(subject pep.SubjectDTO, action, resourceType, resourceMatch string) string {
	return fmt.Sprintf(
		"sub=%s|usr=%s|groups=%s|action=%s|rtype=%s|rmatch=%s",
		subject.Sub,
		subject.Username,
		strings.Join(subject.Groups, ","),
		strings.ToLower(action),
		strings.ToLower(resourceType),
		strings.ToLower(resourceMatch),
	)
}

func (c *Cache) Get(key string, now time.Time) (pep.AuthorizeResponse, bool) {
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return pep.AuthorizeResponse{}, false
	}
	if now.After(it.expires) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return pep.AuthorizeResponse{}, false
	}
	return it.decision, true
}

func (c *Cache) Put(key string, decision pep.AuthorizeResponse, ttl time.Duration) {
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
	c.items[key] = entry{
		decision: decision,
		expires:  time.Now().Add(ttl),
	}
}

// InvalidateOnPolicyChange drops the cache when policy version changes.
func (c *Cache) InvalidateOnPolicyChange(policyVersion int64) {
	if policyVersion <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastPolicyVers == 0 {
		c.lastPolicyVers = policyVersion
		return
	}
	if c.lastPolicyVers != policyVersion {
		c.items = make(map[string]entry, len(c.items))
		c.lastPolicyVers = policyVersion
	}
}

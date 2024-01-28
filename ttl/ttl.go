package ttl

import (
	"container/list"
	"sync"
	"time"
)

type Item struct {
	Key   string
	Value any

	ExpiresAt    time.Time
	ExpireBucket uint8
}

type LRU struct {
	cap   int
	queue *list.List
	items map[string]*list.Element

	mu   sync.Mutex
	ttl  time.Duration
	done chan struct{}

	buckets           []bucket
	nextCleanupBucket uint8
}

type bucket struct {
	entries     map[string]*list.Element
	newestEntry time.Time
}

const numBuckets = 100

func NewLRU(cap int, ttl time.Duration) *LRU {
	if cap < 0 {
		cap = 0
	}

	if ttl < 0 {
		ttl = 0
	}

	res := &LRU{
		cap:   cap,
		items: make(map[string]*list.Element),
		queue: list.New(),

		ttl:  ttl,
		done: make(chan struct{}),
	}

	res.buckets = make([]bucket, numBuckets)
	for i := 0; i < numBuckets; i++ {
		res.buckets[i] = bucket{entries: make(map[string]*list.Element)}
	}

	if res.ttl != 0 {
		go func(done <-chan struct{}) {
			ticker := time.NewTicker(res.ttl / numBuckets)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					res.deleteExpired()
				}
			}
		}(res.done)
	}

	return res
}

func (c *LRU) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.items {
		delete(c.items, k)
	}
	for _, b := range c.buckets {
		for _, ent := range b.entries {
			delete(b.entries, ent.Value.(*Item).Key)
		}
	}
	c.queue = list.New()
}

func (c *LRU) Add(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()

	if ent, ok := c.items[key]; ok {
		c.queue.MoveToFront(ent)
		c.removeFromBucket(ent)
		ent.Value.(*Item).Value = value
		ent.Value.(*Item).ExpiresAt = now.Add(c.ttl)
		c.addToBucket(ent)
		return
	}

	if len(c.items) == c.cap {
		c.removeOldest()
	}

	ent := &Item{
		Key:       key,
		Value:     value,
		ExpiresAt: now.Add(c.ttl),
	}
	element := c.queue.PushFront(ent)
	c.items[key] = element
	c.addToBucket(element)
}

func (c *LRU) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, ok := c.items[key]
	if ok {
		if time.Now().After(ent.Value.(*Item).ExpiresAt) {
			return nil, false
		}
		c.queue.MoveToFront(ent)
		return ent.Value.(*Item).Value, true
	}
	return nil, false
}

func (c *LRU) Remove(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
		return true
	}
	return false
}

func (c *LRU) RemoveOldest() (string, any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ent := c.queue.Back(); ent != nil {
		c.removeElement(ent)
		return ent.Value.(*Item).Key, ent.Value.(*Item).Value, true
	}
	return "", nil, false
}

func (c *LRU) removeOldest() {
	if ent := c.queue.Back(); ent != nil {
		c.removeElement(ent)
	}
}

func (c *LRU) removeElement(e *list.Element) {
	c.queue.Remove(e)
	delete(c.items, e.Value.(*Item).Key)
	c.removeFromBucket(e)
}

func (c *LRU) deleteExpired() {
	c.mu.Lock()
	bucketIdx := c.nextCleanupBucket
	timeToExpire := time.Until(c.buckets[bucketIdx].newestEntry)
	if timeToExpire > 0 {
		c.mu.Unlock()
		time.Sleep(timeToExpire)
		c.mu.Lock()
	}
	for _, ent := range c.buckets[bucketIdx].entries {
		c.removeElement(ent)
	}
	c.nextCleanupBucket = (c.nextCleanupBucket + 1) % numBuckets
	c.mu.Unlock()
}

func (c *LRU) addToBucket(e *list.Element) {
	bucketId := (numBuckets + c.nextCleanupBucket - 1) % numBuckets
	e.Value.(*Item).ExpireBucket = bucketId
	c.buckets[bucketId].entries[e.Value.(*Item).Key] = e
	if c.buckets[bucketId].newestEntry.Before(e.Value.(*Item).ExpiresAt) {
		c.buckets[bucketId].newestEntry = e.Value.(*Item).ExpiresAt
	}
}

func (c *LRU) removeFromBucket(e *list.Element) {
	delete(c.buckets[e.Value.(*Item).ExpireBucket].entries, e.Value.(*Item).Key)
}

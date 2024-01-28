package simple

import "container/list"

type Item struct {
	Key   string
	Value interface{}
}

type LRU struct {
	cap   int
	items map[string]*list.Element
	queue *list.List
}

func NewLru(cap int) *LRU {
	return &LRU{
		cap:   cap,
		items: make(map[string]*list.Element),
		queue: list.New(),
	}
}

func (c *LRU) Set(key string, value interface{}) {
	if element, exist := c.items[key]; exist {
		c.queue.MoveToFront(element)
		element.Value.(*Item).Value = value
		return
	}

	if c.queue.Len() == c.cap {
		c.purge()
	}

	item := &Item{
		Key:   key,
		Value: value,
	}

	element := c.queue.PushFront(item)
	c.items[item.Key] = element

	return
}

func (c *LRU) purge() {
	if element := c.queue.Back(); element != nil {
		item := c.queue.Remove(element).(*Item)
		delete(c.items, item.Key)
	}
}

func (c *LRU) Get(key string) interface{} {
	element, exist := c.items[key]
	if !exist {
		return nil
	}
	c.queue.MoveToFront(element)
	return element.Value.(*Item).Value
}

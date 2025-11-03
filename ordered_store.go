package smartqueue

import (
	"container/heap"
	"container/list"
	"sync"
	"sync/atomic"
)

type orderedStore struct {
	mu             sync.Mutex
	entryMap       map[int64]*entry
	capacity       int64
	size           atomic.Int64
	order          *list.List
	expiryListHeap expiryList
}

func newOrderedStore(cap int64) *orderedStore {
	os := &orderedStore{
		entryMap:       make(map[int64]*entry),
		order:          list.New(),
		expiryListHeap: expiryList{},
		capacity:       cap,
	}

	heap.Init(&os.expiryListHeap)
	return os
}

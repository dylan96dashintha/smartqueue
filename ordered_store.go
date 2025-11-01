package queue

import (
	"container/list"
	"sync/atomic"
)

type orderedStore struct {
	entryMap map[int64]*entry
	capacity int64
	size     atomic.Int64
	order    *list.List
}

func newOrderedStore(cap int64) *orderedStore {
	os := &orderedStore{
		entryMap: make(map[int64]*entry),
		order:    list.New(),
		capacity: cap,
	}
	return os
}

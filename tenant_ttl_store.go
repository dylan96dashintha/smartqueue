package queue

import (
	"container/heap"
	"sync"
	"time"
)

type tenantTTLStore struct {
	mu                 sync.Mutex
	tenantOrderedStore map[string]*orderedStore
	expiryListHeap     expiryList
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	capacity           int64
}

func NewTenantStore(capacity int64) SmartQueue {
	t := &tenantTTLStore{
		tenantOrderedStore: make(map[string]*orderedStore),
		expiryListHeap:     expiryList{},
		stopCh:             make(chan struct{}),
		capacity:           capacity,
	}
	heap.Init(&t.expiryListHeap)
	t.wg.Add(1)
	go t.cleanupLoop()
	return t
}

func (t *tenantTTLStore) GetTenantOrderedMap(tenantId string) (*orderedStore, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	td, ok := t.tenantOrderedStore[tenantId]
	if !ok {
		return nil, false
	}
	return td, true
}

// Insert or update key for a tenant with per-entry TTL
func (t *tenantTTLStore) Enqueue(tenantId string, key int64, value any,
	callback func(tenantId string, key int64), ttl time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	tenantSpecificOrderedStore := t.ensureTenant(tenantId)
	if tenantSpecificOrderedStore.size.Load() >= tenantSpecificOrderedStore.capacity {
		// dequeue the item and then enqueue
		t.removeOldestForCapacity(tenantId)
	} else {
		tenantSpecificOrderedStore.size.Add(1)
	}
	exp := time.Now().Add(ttl)

	if e, ok := tenantSpecificOrderedStore.entryMap[key]; ok {
		e.value = value
		e.expiryTime = exp
	} else {
		elem := tenantSpecificOrderedStore.order.PushBack(key)
		tenantSpecificOrderedStore.entryMap[key] = &entry{
			id:         key,
			value:      value,
			expiryTime: exp,
			element:    elem,
			expiryFunc: callback,
		}
	}

	heap.Push(&t.expiryListHeap, expiry{
		tenantId:   tenantId,
		key:        key,
		expiration: exp,
	})
}

func (t *tenantTTLStore) Pop(tenantID string, key int64) (any, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantID]
	if !ok {
		return nil, false
	}

	e, ok := tenantSpecificOrderedStore.entryMap[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(e.expiryTime) {
		t.removeInternal(tenantID, key, true)
		return nil, false
	}

	return e.value, true
}

func (t *tenantTTLStore) Dequeue(tenantId string) (int64, any, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantId]
	if !ok {
		return 0, nil, false
	}

	front := tenantSpecificOrderedStore.order.Front()
	if front == nil {
		return 0, nil, false
	}

	key := front.Value.(int64)
	e := tenantSpecificOrderedStore.entryMap[key]

	if time.Now().After(e.expiryTime) {
		t.removeInternal(tenantId, key, true)
		return 0, nil, false
	}

	t.removeInternal(tenantId, key)
	return key, e.value, true
}

func (t *tenantTTLStore) Remove(tenantID string, key int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.removeInternal(tenantID, key)
}

func (t *tenantTTLStore) ensureTenant(tenantID string) *orderedStore {
	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantID]
	if !ok {
		tenantSpecificOrderedStore = newOrderedStore(t.capacity)
		t.tenantOrderedStore[tenantID] = tenantSpecificOrderedStore
	}
	return tenantSpecificOrderedStore
}

func (t *tenantTTLStore) removeInternal(tenantID string, key int64, limitReached ...bool) {
	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantID]
	if !ok {
		return
	}
	e, ok := tenantSpecificOrderedStore.entryMap[key]
	if ok {
		if limitReached != nil && limitReached[0] {
			e.expiryFunc(tenantID, key)
		}
		tenantSpecificOrderedStore.order.Remove(e.element)
		delete(tenantSpecificOrderedStore.entryMap, key)
	} else {
		//fmt.Println("already delete: ", key)

	}

}

func (t *tenantTTLStore) cleanupLoop() {
	defer t.wg.Done()

	for {
		t.mu.Lock()

		if len(t.expiryListHeap) == 0 {
			t.mu.Unlock()
			select {
			case <-t.stopCh:
				return
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}

		next := t.expiryListHeap[0]
		now := time.Now()
		delay := next.expiration.Sub(now)

		if delay > 0 {
			t.mu.Unlock()
			select {
			case <-t.stopCh:
				return
			case <-time.After(delay):
				continue
			}
		}

		heap.Pop(&t.expiryListHeap)
		t.removeInternal(next.tenantId, next.key, true)
		t.mu.Unlock()
	}
}

func (t *tenantTTLStore) removeOldestForCapacity(tenantId string) {

	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantId]
	if !ok {
		return
	}

	front := tenantSpecificOrderedStore.order.Front()
	if front == nil {
		return
	}

	key := front.Value.(int64)
	//e := tenantSpecificOrderedStore.entryMap[key]

	t.removeInternal(tenantId, key, true)
	return

}

func (t *tenantTTLStore) Close() {
	close(t.stopCh)
	t.wg.Wait()
}

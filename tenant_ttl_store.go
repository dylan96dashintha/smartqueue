package smartqueue

import (
	"container/heap"
	"sync"
	"time"
)

type tenantTTLStore struct {
	//mu                 sync.Mutex
	tenantsMu          sync.RWMutex
	tenantOrderedStore map[string]*orderedStore
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	capacity           int64
}

func NewTenantStore(capacity int64) SmartQueue {
	t := &tenantTTLStore{
		tenantOrderedStore: make(map[string]*orderedStore),
		//expiryListHeap:     expiryList{},
		stopCh:   make(chan struct{}),
		capacity: capacity,
	}
	//t.wg.Add(1)
	//go t.cleanupLoop()
	return t
}

func (t *tenantTTLStore) GetTenantOrderedMap(tenantId string) (*orderedStore, bool) {
	t.tenantsMu.RLock()
	defer t.tenantsMu.RUnlock()

	td, ok := t.tenantOrderedStore[tenantId]
	if !ok {
		return nil, false
	}
	return td, true
}

// Insert or update key for a tenant with per-entry TTL
func (t *tenantTTLStore) Enqueue(tenantId string, key int64, value any,
	callback func(tenantId string, key int64), ttl time.Duration) {
	//t.mu.Lock()
	//defer t.mu.Unlock()

	tenantSpecificOrderedStore := t.tenantStore(tenantId)

	tenantSpecificOrderedStore.mu.Lock()
	defer tenantSpecificOrderedStore.mu.Unlock()
	if tenantSpecificOrderedStore.size.Load() >= tenantSpecificOrderedStore.capacity {
		// dequeue the item and then enqueue
		t.removeOldestForCapacity(tenantId, tenantSpecificOrderedStore)
	} else {
		tenantSpecificOrderedStore.size.Add(1)
	}
	exp := time.Now().Add(ttl)

	e, ok := tenantSpecificOrderedStore.entryMap[key]
	if ok {
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

	heap.Push(&tenantSpecificOrderedStore.expiryListHeap, expiry{
		tenantId:   tenantId,
		key:        key,
		expiration: exp,
	})
}

func (t *tenantTTLStore) Pop(tenantID string, key int64) (any, bool) {
	//t.mu.Lock()
	//defer t.mu.Unlock()

	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantID]
	if !ok {
		return nil, false
	}

	tenantSpecificOrderedStore.mu.Lock()
	defer tenantSpecificOrderedStore.mu.Unlock()

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
	//t.mu.Lock()
	//defer t.mu.Unlock()

	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantId]
	if !ok {
		return 0, nil, false
	}

	tenantSpecificOrderedStore.mu.Lock()
	defer tenantSpecificOrderedStore.mu.Unlock()

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
	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantID]
	if !ok {
		return
	}

	tenantSpecificOrderedStore.mu.Lock()
	defer tenantSpecificOrderedStore.mu.Unlock()
	t.removeInternal(tenantID, key)
}

func (t *tenantTTLStore) tenantStore(tenantId string) *orderedStore {
	t.tenantsMu.RLock()
	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantId]
	t.tenantsMu.RUnlock()
	if !ok {
		// If not exists, lock for writing
		t.tenantsMu.Lock()
		// double-check in case another goroutine created it
		tenantSpecificOrderedStore, ok = t.tenantOrderedStore[tenantId]
		if !ok {
			tenantSpecificOrderedStore = newOrderedStore(t.capacity)
			t.tenantOrderedStore[tenantId] = tenantSpecificOrderedStore

			t.wg.Add(1)
			go t.cleanupTenantLoop(tenantId, tenantSpecificOrderedStore)
		}
		t.tenantsMu.Unlock()
	}
	return tenantSpecificOrderedStore
}

func (t *tenantTTLStore) removeInternal(tenantID string, key int64, limitReached ...bool) {
	// Caller must hold tenantSpecificOrderedStore.mu
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

func (t *tenantTTLStore) cleanupTenantLoop(tenantID string, tenantStore *orderedStore) {
	defer t.wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			now := time.Now()

			tenantStore.mu.Lock()
			for tenantStore.expiryListHeap.Len() > 0 {
				next := tenantStore.expiryListHeap[0]
				if next.expiration.After(now) {
					break
				}
				heap.Pop(&tenantStore.expiryListHeap)

				if e, ok := tenantStore.entryMap[next.key]; ok {
					tenantStore.order.Remove(e.element)
					delete(tenantStore.entryMap, next.key)
					tenantStore.size.Add(-1)

					tenantStore.mu.Unlock()
					e.expiryFunc(tenantID, next.key)

					tenantStore.mu.Lock()

				}
			}
			tenantStore.mu.Unlock()
		}
	}
}

func (t *tenantTTLStore) removeOldestForCapacity(tenantId string,
	tenantSpecificOrderedStore *orderedStore) {

	front := tenantSpecificOrderedStore.order.Front()
	if front == nil {
		return
	}

	key := front.Value.(int64)
	//e := tenantSpecificOrderedStore.entryMap[key]

	t.removeInternal(tenantId, key, true)
	return

}

func (t *tenantTTLStore) Stop() {
	close(t.stopCh)
	t.wg.Wait()
}

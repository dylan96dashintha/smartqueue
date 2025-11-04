package smartqueue

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort       = 8098
	tenantSpecificUrl = `/smartqueue/tenant/`
	entryParam        = `entry`
)

type tenantView struct {
	Key        int64         `json:"key"`
	Value      any           `json:"value"`
	ExpiryTime int64         `json:"expiry_time"`
	TTL        time.Duration `json:"ttl_remaining"`
}

type tenantTTLStore struct {
	tenantsMu          sync.RWMutex
	tenantOrderedStore map[string]*orderedStore
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	capacity           int64
}

func NewTenantStore(capacity int64) SmartQueue {
	t := &tenantTTLStore{
		tenantOrderedStore: make(map[string]*orderedStore),
		stopCh:             make(chan struct{}),
		capacity:           capacity,
	}

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

	for {
		tenantStore.mu.Lock()

		if tenantStore.expiryListHeap.Len() == 0 {
			tenantStore.mu.Unlock()
			select {
			case <-t.stopCh:
				return
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}

		next := tenantStore.expiryListHeap[0]
		now := time.Now()
		delay := next.expiration.Sub(now)

		if delay > 0 {
			tenantStore.mu.Unlock()
			select {
			case <-t.stopCh:
				return
			case <-time.After(delay):
				continue
			}
		}

		// Expired now, pop and handle
		heap.Pop(&tenantStore.expiryListHeap)
		if e, ok := tenantStore.entryMap[next.key]; ok {
			e.expiryFunc(tenantID, next.key)
			tenantStore.order.Remove(e.element)
			delete(tenantStore.entryMap, next.key)
			tenantStore.size.Add(-1)
		}

		tenantStore.mu.Unlock()
	}
}

func (t *tenantTTLStore) removeOldestForCapacity(tenantId string,
	tenantSpecificOrderedStore *orderedStore) {

	front := tenantSpecificOrderedStore.order.Front()
	if front == nil {
		return
	}

	key := front.Value.(int64)

	t.removeInternal(tenantId, key, true)
	return

}

func (t *tenantTTLStore) Stop() {
	close(t.stopCh)
	t.wg.Wait()
}

func (t *tenantTTLStore) RegisterHTTPHandlers(port ...int64) (err error) {

	mux := http.NewServeMux()
	httpPort := int64(defaultPort)
	if len(port) != 0 {
		httpPort = port[0]
	}
	err = http.ListenAndServe(fmt.Sprintf(":%d", httpPort), mux)
	if err != nil {
		return err
	}
	// Tenant or Entry details
	mux.HandleFunc(tenantSpecificUrl, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, tenantSpecificUrl)
		parts := strings.Split(path, "/")

		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "tenant ID required", http.StatusBadRequest)
			return
		}

		tenantID := parts[0]

		if len(parts) == 1 {
			t.handleTenantView(w, tenantID)
			return
		}

		if len(parts) == 3 && parts[1] == entryParam {
			entryIDStr := parts[2]
			entryID, err := strconv.ParseInt(entryIDStr, 10, 64)
			if err != nil {
				http.Error(w, "invalid entry ID", http.StatusBadRequest)
				return
			}

			t.handleTenantEntryView(w, tenantID, entryID)
			return
		}

		http.NotFound(w, r)
	})

	return nil
}

func (t *tenantTTLStore) handleTenantView(w http.ResponseWriter, tenantID string) {
	t.tenantsMu.RLock()
	tenantStore, ok := t.tenantOrderedStore[tenantID]
	t.tenantsMu.RUnlock()

	if !ok {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	tenantStore.mu.RLock()
	defer tenantStore.mu.RUnlock()

	//now := time.Now()
	var items []tenantView
	for k, e := range tenantStore.entryMap {
		items = append(items, tenantView{
			Key:        k,
			Value:      e.value,
			ExpiryTime: e.expiryTime.Unix(),
			TTL:        time.Until(e.expiryTime),
		})
	}

	writeJSON(w, items)
}

func (t *tenantTTLStore) handleTenantEntryView(w http.ResponseWriter, tenantId string, entryID int64) {

	t.tenantsMu.RLock()
	tenantSpecificOrderedStore, ok := t.tenantOrderedStore[tenantId]
	t.tenantsMu.RUnlock()

	if !ok {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	tenantSpecificOrderedStore.mu.RLock()
	defer tenantSpecificOrderedStore.mu.RUnlock()

	e, ok := tenantSpecificOrderedStore.entryMap[entryID]
	if !ok {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}

	writeJSON(w, tenantView{
		Key:        e.id,
		Value:      e.value,
		ExpiryTime: e.expiryTime.Unix(),
		TTL:        time.Until(e.expiryTime),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

package queue

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

func BenchmarkTenantTTLStoreEnqueue(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Close()

	tenantID := "t0001"
	callback := func(tenantId string, key int64) {
		fmt.Printf("key: %d, tenantId: %v , fire the init_cancel event", key, tenantId)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := int64(i)
		store.Enqueue(tenantID, key, "value"+strconv.Itoa(i), callback, 5*time.Second)
	}
}

func BenchmarkTenantTTLStorePop(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Close()
	callback := func(tenantId string, key int64) {
		fmt.Printf("key: %d, tenantId: %v , fire the init_cancel event", key, tenantId)
	}
	tenantID := "t0001"
	// pre-fill with items
	for i := 0; i < 10000; i++ {
		store.Enqueue(tenantID, int64(i), "value"+strconv.Itoa(i), callback, 5*time.Second)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Pop(tenantID, int64(i%1000)) // loop over pre-filled keys
	}
}

func BenchmarkTenantTTLStoreEnqueueDequeue(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Close()
	callback := func(tenantId string, key int64) {
		fmt.Printf("key: %d, tenantId: %v , fire the init_cancel event", key, tenantId)
	}
	tenantID := "t0001"
	// pre-fill with items
	for i := 0; i < 10000; i++ {
		store.Enqueue(tenantID, int64(i), "value"+strconv.Itoa(i), callback, 5*time.Second)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Dequeue(tenantID)
	}
}

func BenchmarkTenantTTLStoreRemove(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Close()
	callback := func(tenantId string, key int64) {
		fmt.Printf("key: %d, tenantId: %v , fire the init_cancel event", key, tenantId)
	}
	tenantID := "t0001"
	// pre-fill with items
	for i := 0; i < 10000; i++ {
		store.Enqueue(tenantID, int64(i), "value"+strconv.Itoa(i), callback, 5*time.Second)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Remove(tenantID, int64(i%1000))
	}
}

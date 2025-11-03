package smartqueue

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"
)

func init() {
	runtime.GOMAXPROCS(1) // simulate 0.5 CPU in local machine
}

func BenchmarkTenantTTLStoreEnqueue(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Stop()

	f, err := os.Create("cpu.prof")
	if err != nil {
		b.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
	tenantID := "t0001"
	callback := func(tenantId string, key int64) {
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := int64(i)
		store.Enqueue(tenantID, key, "value"+strconv.Itoa(i), callback, 5*time.Second)
	}
}

func BenchmarkTenantTTLStoreConcurrent(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Stop()

	tenantID := "t0001"
	callback := func(tenantId string, key int64) {}

	//maxKeys := 100

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := int64(i)
			store.Enqueue(
				tenantID,
				key,
				"value"+strconv.Itoa(i),
				callback,
				5*time.Second,
			)

			// Dequeue or Pop occasionally
			if i%5 == 0 {
				store.Dequeue(tenantID)
			}

			i++
		}
	})
}

func BenchmarkTenantTTLStorePop(b *testing.B) {
	store := NewTenantStore(10000000)
	defer store.Stop()
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
	defer store.Stop()
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
	defer store.Stop()
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

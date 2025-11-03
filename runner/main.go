package main

import (
	"fmt"
	"github.com/smartqueue"
	"time"
)

func main() {
	store := smartqueue.NewTenantStore(1000)
	defer store.Stop()

	callback := func(tenantId string, key int64) {
		fmt.Printf("key: %d, tenantId: %v , fire the init_cancel event", key, tenantId)
	}
	//go func() {
	store.Enqueue("t0001", 121, "apple", callback, 6*time.Second)
	//}()

	go func() {
		store.Enqueue("t0002", 124, "banana", callback, 15*time.Second)
	}()
	go func() {
		store.Enqueue("t0001", 125, "avocado", callback, 15*time.Second)

	}()

	fmt.Println(store.Pop("t0001", 121)) // apple
	store.Remove("t0001", 121)

	time.Sleep(6 * time.Second)
	fmt.Println(store.Pop("t0001", 121)) // expired -> nil, false
	fmt.Println(store.Pop("t0002", 124)) // still valid
	//fmt.Println(store.PopFirst("t0001")) // removes oldest valid item (a2)
	time.Sleep(20 * time.Second)
	//fmt.Println(store.GetTenantOrderedMap("t0001"))
}

package smartqueue

import (
	"time"
)

type SmartQueue interface {
	Enqueue(tenantId string, key int64, value any,
		callback func(tenantId string, key int64), ttl time.Duration) (capacityReached bool)
	Pop(tenantID string, key int64) (any, bool)
	Dequeue(tenantID string) (int64, any, bool)
	Remove(tenantID string, key int64)
	GetTenantOrderedMap(tenantId string) (*orderedStore, bool)
	Stop()
	RegisterHTTPHandlers(port ...int64) (err error)
}

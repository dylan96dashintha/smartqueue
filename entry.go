package queue

import (
	"container/list"
	"time"
)

// Item element that will insert to the queue
type entry struct {
	id         int64
	value      interface{}
	expiryTime time.Time
	element    *list.Element
	expiryFunc func(tenantId string, key int64)
}

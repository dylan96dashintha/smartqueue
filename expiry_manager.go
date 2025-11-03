package smartqueue

import "time"

type expiry struct {
	tenantId   string
	key        int64
	expiration time.Time
}

type expiryList []expiry

func (e expiryList) Len() int           { return len(e) }
func (e expiryList) Less(i, j int) bool { return e[i].expiration.Before(e[j].expiration) }
func (e expiryList) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }

func (e *expiryList) Push(x any) { *e = append(*e, x.(expiry)) }

func (e *expiryList) Pop() any {
	old := *e
	n := len(old)
	x := old[n-1]
	*e = old[:n-1]
	return x
}

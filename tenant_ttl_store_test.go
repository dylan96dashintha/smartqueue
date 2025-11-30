package smartqueue

import (
	"sync/atomic"
	"testing"
	"time"
)

type mockEntry struct {
	Id   int64
	Name string
}

func TestTenantTTLStoresEnqueue(t *testing.T) {
	var callbackTriggered int32

	mockCallback := func(tenantId string, key int64) {
		atomic.AddInt32(&callbackTriggered, 1)
	}

	type fields struct {
		capacity int64
	}
	type args struct {
		tenantId string
		key      int64
		value    interface{}
		callback func(tenantId string, key int64)
		ttl      time.Duration
	}

	tests := []struct {
		name            string
		fields          fields
		args            args
		updateExisting  bool
		wantExist       bool
		expectCallback  bool
		capacityExceeds bool
	}{
		{
			name: "Valid Entry enqueue",
			fields: fields{
				capacity: 1000,
			},
			args: args{
				tenantId: "t0001",
				key:      121,
				value: mockEntry{
					Id:   121,
					Name: "test121",
				},
				callback: mockCallback,
				ttl:      500 * time.Millisecond,
			},
			wantExist:      true,
			updateExisting: false,
			expectCallback: true,
		},
		{
			name: "Duplicate key update",
			fields: fields{
				capacity: 1000,
			},
			args: args{
				tenantId: "t0001",
				key:      121,
				value: mockEntry{
					Id:   121,
					Name: "updatedValue",
				},
				callback: mockCallback,
				ttl:      500 * time.Millisecond,
			},
			wantExist:      true,
			updateExisting: true,
			expectCallback: true,
		},
		{
			name: "Multiple tenants isolation",
			fields: fields{
				capacity: 1000,
			},
			args: args{
				tenantId: "t0002",
				key:      222,
				value: mockEntry{
					Id:   222,
					Name: "tenantB",
				},
				callback: mockCallback,
				ttl:      500 * time.Millisecond,
			},
			wantExist:      true,
			updateExisting: false,
			expectCallback: true,
		},
		{
			name: "Capacity overflow eviction",
			fields: fields{
				capacity: 1,
			},
			args: args{
				tenantId: "t0003",
				key:      333,
				value: mockEntry{
					Id:   333,
					Name: "capacity_test",
				},
				callback: mockCallback,
				ttl:      500 * time.Millisecond,
			},
			wantExist:       true,
			updateExisting:  false,
			expectCallback:  true,
			capacityExceeds: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewTenantStore(tt.fields.capacity).(*tenantTTLStore)
			defer store.Stop()

			// enqueue first
			capacity := store.Enqueue(tt.args.tenantId, tt.args.key, tt.args.value, tt.args.callback, tt.args.ttl)

			if tt.capacityExceeds {
				capacity = store.Enqueue(tt.args.tenantId, tt.args.key+1, tt.args.value, tt.args.callback, tt.args.ttl)
			}
			// optional update same key
			if tt.updateExisting {
				updatedValue := mockEntry{
					Id:   tt.args.key,
					Name: "updated_again",
				}
				store.Enqueue(tt.args.tenantId, tt.args.key, updatedValue, tt.args.callback, tt.args.ttl)
			}

			var (
				val     any
				isExist bool
			)
			if !tt.capacityExceeds {
				val, isExist = store.Pop(tt.args.tenantId, tt.args.key)
				if isExist != tt.wantExist {
					t.Errorf("%s: expected existence %v, got %v", tt.name, tt.wantExist, isExist)
				}
			} else {
				val, isExist = store.Pop(tt.args.tenantId, tt.args.key+1)
				if isExist != tt.wantExist {
					t.Errorf("%s: expected existence %v, got %v", tt.name, tt.wantExist, isExist)
				}
				if capacity != tt.capacityExceeds {
					t.Errorf("%s: expected existence %v, got %v", tt.name, tt.capacityExceeds, capacity)
				}
			}

			if isExist {
				mockValue, ok := val.(mockEntry)
				if !ok {
					t.Errorf("%s: expected mockEntry type", tt.name)
				}
				if mockValue.Id != tt.args.value.(mockEntry).Id {
					t.Errorf("%s: expected Id %v, got %v", tt.name, tt.args.value.(mockEntry).Id, mockValue.Id)
				}
			}

			// Wait for possible expiry
			time.Sleep(tt.args.ttl + 200*time.Millisecond)

			if tt.expectCallback && atomic.LoadInt32(&callbackTriggered) == 0 {
				t.Errorf("%s: expected callback to trigger on expiry", tt.name)
			}
		})
	}
}

func TestTenantTTLStorePop(t *testing.T) {
	mockCallback := func(tenantId string, key int64) {}

	type fields struct {
		capacity int64
	}
	type args struct {
		tenantID string
		key      int64
	}

	tests := []struct {
		name        string
		fields      fields
		args        args
		setup       func(store *tenantTTLStore)
		wantExist   bool
		wantNil     bool
		description string
	}{
		{
			name: "Tenant not found",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantID: "unknownTenant",
				key:      1,
			},
			setup:       func(store *tenantTTLStore) {},
			wantExist:   false,
			wantNil:     true,
			description: "Should return false when tenant not found",
		},
		{
			name: "Key not found in tenant",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantID: "t0001",
				key:      999,
			},
			setup: func(store *tenantTTLStore) {
				store.Enqueue("t0001", 1, mockEntry{Id: 1, Name: "A"}, mockCallback, 1*time.Second)
			},
			wantExist:   false,
			wantNil:     true,
			description: "Should return false when key not found",
		},
		{
			name: "Expired entry should be removed and return false",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantID: "t0002",
				key:      2,
			},
			setup: func(store *tenantTTLStore) {
				store.Enqueue("t0002", 2, mockEntry{Id: 2, Name: "Expired"}, mockCallback, 100*time.Millisecond)
				time.Sleep(150 * time.Millisecond) // let it expire
			},
			wantExist:   false,
			wantNil:     true,
			description: "Should return false and remove expired entry",
		},
		{
			name: "Valid entry should return value and true",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantID: "t0003",
				key:      3,
			},
			setup: func(store *tenantTTLStore) {
				store.Enqueue("t0003", 3, mockEntry{Id: 3, Name: "Valid"}, mockCallback, 1*time.Second)
			},
			wantExist:   true,
			wantNil:     false,
			description: "Should return valid value and true when not expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewTenantStore(tt.fields.capacity).(*tenantTTLStore)
			defer store.Stop()

			// setup before test
			tt.setup(store)

			val, exist := store.Pop(tt.args.tenantID, tt.args.key)

			if exist != tt.wantExist {
				t.Errorf("%s: expected exist=%v, got %v", tt.name, tt.wantExist, exist)
			}

			if tt.wantNil && val != nil {
				t.Errorf("%s: expected nil value, got %+v", tt.name, val)
			}

			if !tt.wantNil && val == nil {
				t.Errorf("%s: expected non-nil value, got nil", tt.name)
			}

			if !tt.wantNil {
				mockVal, ok := val.(mockEntry)
				if !ok {
					t.Errorf("%s: expected mockEntry type", tt.name)
				} else if mockVal.Id != tt.args.key {
					t.Errorf("%s: expected Id=%d, got %d", tt.name, tt.args.key, mockVal.Id)
				}
			}
		})
	}
}

func TestTenantTTLStoreDequeue(t *testing.T) {
	mockCallback := func(tenantId string, key int64) {}

	type fields struct {
		capacity int64
	}
	type args struct {
		tenantId string
	}

	tests := []struct {
		name        string
		fields      fields
		args        args
		setup       func(store *tenantTTLStore)
		wantExist   bool
		wantNil     bool
		wantKey     int64
		description string
	}{
		{
			name: "Tenant not found",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantId: "unknownTenant",
			},
			setup:       func(store *tenantTTLStore) {},
			wantExist:   false,
			wantNil:     true,
			wantKey:     0,
			description: "Should return false when tenant not found",
		},
		{
			name: "Empty queue for tenant",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantId: "t0001",
			},
			setup: func(store *tenantTTLStore) {
				store.tenantStore("t0001") // initialize empty tenant
			},
			wantExist:   false,
			wantNil:     true,
			wantKey:     0,
			description: "Should return false when tenant queue is empty",
		},
		{
			name: "Expired entry should return false and be removed",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantId: "t0002",
			},
			setup: func(store *tenantTTLStore) {
				store.Enqueue("t0002", 10, mockEntry{Id: 10, Name: "Expired"}, mockCallback, 50*time.Millisecond)
				time.Sleep(100 * time.Millisecond) // let TTL expire
			},
			wantExist:   false,
			wantNil:     true,
			wantKey:     0,
			description: "Should return false and remove expired item",
		},
		{
			name: "Valid item should dequeue successfully",
			fields: fields{
				capacity: 100,
			},
			args: args{
				tenantId: "t0003",
			},
			setup: func(store *tenantTTLStore) {
				store.Enqueue("t0003", 101, mockEntry{Id: 101, Name: "Item101"}, mockCallback, 1*time.Second)
				store.Enqueue("t0003", 102, mockEntry{Id: 102, Name: "Item102"}, mockCallback, 1*time.Second)
			},
			wantExist:   true,
			wantNil:     false,
			wantKey:     101, // FIFO
			description: "Should dequeue first item in queue (FIFO)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewTenantStore(tt.fields.capacity).(*tenantTTLStore)
			defer store.Stop()

			tt.setup(store)

			key, val, exist := store.Dequeue(tt.args.tenantId)

			if exist != tt.wantExist {
				t.Errorf("%s: expected exist=%v, got %v", tt.name, tt.wantExist, exist)
			}

			if key != tt.wantKey {
				t.Errorf("%s: expected key=%v, got %v", tt.name, tt.wantKey, key)
			}

			if tt.wantNil && val != nil {
				t.Errorf("%s: expected nil value, got %+v", tt.name, val)
			}

			if !tt.wantNil && val == nil {
				t.Errorf("%s: expected non-nil value, got nil", tt.name)
			}

			if !tt.wantNil {
				mockVal, ok := val.(mockEntry)
				if !ok {
					t.Errorf("%s: expected mockEntry type", tt.name)
				} else if mockVal.Id != tt.wantKey {
					t.Errorf("%s: expected Id=%d, got %d", tt.name, tt.wantKey, mockVal.Id)
				}
			}
		})
	}
}

func TestTenantTTLStoreRemove(t *testing.T) {
	mockCallback := func(tenantId string, key int64) {}

	type fields struct {
		capacity int64
	}
	type args struct {
		tenantId string
		key      int64
	}

	tests := []struct {
		name        string
		fields      fields
		args        args
		setup       func(store *tenantTTLStore)
		expectExist bool
		description string
	}{
		{
			name: "Tenant not found - should not panic",
			fields: fields{
				capacity: 10,
			},
			args: args{
				tenantId: "unknownTenant",
				key:      1,
			},
			setup:       func(store *tenantTTLStore) {},
			expectExist: false,
			description: "Remove on unknown tenant should do nothing",
		},
		{
			name: "Key not found - no effect",
			fields: fields{
				capacity: 10,
			},
			args: args{
				tenantId: "t0001",
				key:      999,
			},
			setup: func(store *tenantTTLStore) {
				store.tenantStore("t0001") // initialize tenant store without inserting key
			},
			expectExist: false,
			description: "Remove on missing key should do nothing",
		},
		{
			name: "Existing key - should remove successfully",
			fields: fields{
				capacity: 10,
			},
			args: args{
				tenantId: "t0002",
				key:      123,
			},
			setup: func(store *tenantTTLStore) {
				store.Enqueue("t0002", 123, mockEntry{Id: 123, Name: "to_remove"}, mockCallback, 5*time.Second)
			},
			expectExist: false,
			description: "Existing entry should be removed from tenant store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewTenantStore(tt.fields.capacity).(*tenantTTLStore)
			defer store.Stop()

			tt.setup(store)

			// Act
			store.Remove(tt.args.tenantId, tt.args.key)

			// Verify removal
			val, exist := store.Pop(tt.args.tenantId, tt.args.key)
			if exist != tt.expectExist {
				t.Errorf("%s: expected exist=%v, got %v", tt.name, tt.expectExist, exist)
			}
			if exist && val == nil {
				t.Errorf("%s: expected non-nil value for existing key", tt.name)
			}
		})
	}
}

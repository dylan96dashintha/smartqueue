package smartqueue

import (
	"testing"
	"time"
)

type mockEntry struct {
	Id   int64
	Name string
}

func TestTenantTTLStoreEnqueue(t *testing.T) {

	mockCallback := func(tenantId string, key int64) {
		//fmt.Printf("key: %d, tenantId: %v , fire the init_cancel event", key, tenantId)
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
		name    string
		fields  fields
		args    args
		wantErr bool
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
				ttl:      5 * time.Second,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewTenantStore(tt.fields.capacity)
			a.Enqueue(tt.args.tenantId, tt.args.key, tt.args.value, tt.args.callback, tt.args.ttl)
			val, isExist := a.Pop(tt.args.tenantId, tt.args.key)
			if !isExist != tt.wantErr {
				t.Errorf(tt.name, "error = %v, wantErr %v", isExist, tt.wantErr)
			}
			mockValue, ok := val.(mockEntry)
			if !ok {
				t.Errorf(tt.name, "value not mockEntry")
			}
			if mockValue != tt.args.value {
				t.Errorf(tt.name, "value not mockEntry")
			}
		})
	}
}

package gateway

import (
	"context"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdClient interface defines the etcd operations we need
type EtcdClient interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
	Close() error
}

// Ensure clientv3.Client implements EtcdClient interface
var _ EtcdClient = (*clientv3.Client)(nil)
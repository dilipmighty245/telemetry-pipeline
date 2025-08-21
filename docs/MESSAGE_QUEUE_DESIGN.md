# Custom Message Queue Design

## Table of Contents
1. [Design Overview](#design-overview)
2. [Architecture Components](#architecture-components)
3. [Message Format](#message-format)
4. [Partitioning Strategy](#partitioning-strategy)
5. [Producer Interface](#producer-interface)
6. [Consumer Interface](#consumer-interface)
7. [Persistence and Durability](#persistence-and-durability)
8. [Scalability and Performance](#scalability-and-performance)
9. [Fault Tolerance](#fault-tolerance)
10. [Configuration](#configuration)
11. [Monitoring and Observability](#monitoring-and-observability)
12. [Implementation Details](#implementation-details)

## Design Overview

The custom message queue is designed as a high-performance, distributed pub-sub system optimized for GPU telemetry data streaming. It follows a topic-based partitioned architecture with strong consistency guarantees and horizontal scalability.

### Key Design Principles:
- **High Throughput**: Handle 10,000+ messages/second per broker
- **Low Latency**: Sub-millisecond message delivery
- **Fault Tolerance**: No message loss with configurable durability
- **Horizontal Scalability**: Scale brokers and partitions independently
- **Operational Simplicity**: Easy deployment and management in Kubernetes

### Architecture Pattern:
**Distributed Topic-Partition Model** with Leader-Follower replication

## Architecture Components

### 1. Message Broker Cluster

#### Broker Node
Each broker node is a stateful service responsible for:
- Managing assigned topic partitions
- Handling producer connections and message ingestion
- Serving consumer requests and message delivery
- Participating in cluster coordination and leader election
- Maintaining local message storage and replication

#### Cluster Coordinator
- **Leader Election**: Uses Raft consensus for broker leader election
- **Metadata Management**: Maintains cluster topology and partition assignments
- **Health Monitoring**: Tracks broker health and triggers rebalancing
- **Client Routing**: Provides partition-to-broker mapping for clients

### 2. Topic and Partition Management

#### Topic Structure
```go
type Topic struct {
    Name        string
    Partitions  []Partition
    Config      TopicConfig
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type TopicConfig struct {
    PartitionCount     int
    ReplicationFactor  int
    RetentionTime      time.Duration
    RetentionSize      int64
    CompressionType    string
}
```

#### Partition Structure
```go
type Partition struct {
    ID          int
    TopicName   string
    Leader      string // Broker ID
    Replicas    []string // Broker IDs
    ISR         []string // In-Sync Replicas
    LogSegments []LogSegment
    HighWatermark int64
    LastOffset    int64
}
```

### 3. Storage Engine

#### Log Segment Structure
- **Segment Files**: Immutable log files containing messages
- **Index Files**: Offset-to-position mapping for fast lookups
- **Time Index**: Timestamp-to-offset mapping for time-based queries
- **Write-Ahead Log**: Ensures durability before acknowledgment

#### Storage Layout
```
/data/topics/
├── gpu-telemetry/
│   ├── partition-0/
│   │   ├── 00000000000000000000.log
│   │   ├── 00000000000000000000.index
│   │   ├── 00000000000000000000.timeindex
│   │   └── leader-epoch-checkpoint
│   ├── partition-1/
│   └── partition-N/
└── __cluster_metadata/
    ├── brokers.json
    └── topics.json
```

## Message Format

### Message Structure
```go
type Message struct {
    Headers    map[string]string
    Key        []byte
    Value      []byte
    Timestamp  int64
    Offset     int64
    Partition  int32
    Checksum   uint32
}
```

### Wire Protocol
```
Message Wire Format (Binary):
[4 bytes: Message Length]
[4 bytes: CRC32 Checksum]
[1 byte: Magic Byte/Version]
[1 byte: Attributes/Flags]
[8 bytes: Timestamp]
[4 bytes: Key Length]
[N bytes: Key]
[4 bytes: Value Length]
[N bytes: Value]
[4 bytes: Headers Count]
[Headers: Key-Value Pairs]
```

### Telemetry Message Example
```json
{
  "headers": {
    "source": "streamer-1",
    "version": "1.0",
    "content-type": "application/json"
  },
  "key": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
  "value": {
    "timestamp": "2025-07-18T20:42:34Z",
    "metric_name": "DCGM_FI_DEV_GPU_UTIL",
    "gpu_id": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
    "device": "nvidia0",
    "hostname": "mtv5-dgx1-hgpu-031",
    "value": 85.0,
    "labels": {
      "driver_version": "535.129.03",
      "model_name": "NVIDIA H100 80GB HBM3"
    }
  }
}
```

## Partitioning Strategy

### Partition Assignment
1. **Hash-based Partitioning**: `partition = hash(message.key) % partition_count`
2. **Round-robin**: For messages without keys
3. **Custom Partitioner**: Pluggable partitioning logic

### Partition Distribution
- **Even Distribution**: Partitions evenly distributed across brokers
- **Rack Awareness**: Replicas placed across different availability zones
- **Load Balancing**: Monitor partition load and rebalance if needed

### Partition Scaling
- **Dynamic Partition Addition**: Add partitions without downtime
- **Rebalancing**: Automatic partition reassignment
- **Data Migration**: Background data movement for rebalancing

## Producer Interface

### Producer API
```go
type Producer interface {
    Send(ctx context.Context, message *Message) (*SendResult, error)
    SendAsync(ctx context.Context, message *Message, callback func(*SendResult, error))
    SendBatch(ctx context.Context, messages []*Message) ([]*SendResult, error)
    Flush(ctx context.Context) error
    Close() error
}

type ProducerConfig struct {
    BootstrapServers []string
    ClientID         string
    Acks             int // 0, 1, or -1 (all)
    RetryCount       int
    RetryDelay       time.Duration
    BatchSize        int
    LingerTime       time.Duration
    CompressionType  string
    MaxMessageSize   int
}
```

### Producer Implementation Features

#### Batching
- **Automatic Batching**: Group messages for efficient network utilization
- **Size-based Batching**: Send when batch reaches size limit
- **Time-based Batching**: Send after linger time expires
- **Compression**: Optional batch compression (gzip, snappy, lz4)

#### Reliability
- **Acknowledgment Modes**:
  - `acks=0`: Fire-and-forget (fastest, least reliable)
  - `acks=1`: Leader acknowledgment (balanced)
  - `acks=-1`: All replicas acknowledgment (slowest, most reliable)
- **Retry Logic**: Exponential backoff with jitter
- **Idempotent Sends**: Prevent duplicate messages

#### Performance Optimizations
- **Connection Pooling**: Reuse TCP connections
- **Pipelining**: Send multiple requests without waiting for responses
- **Memory Management**: Efficient buffer management and recycling

## Consumer Interface

### Consumer API
```go
type Consumer interface {
    Subscribe(topics []string) error
    Poll(ctx context.Context, timeout time.Duration) ([]*Message, error)
    Commit(ctx context.Context, offsets map[string]map[int32]int64) error
    Close() error
}

type ConsumerConfig struct {
    BootstrapServers   []string
    GroupID           string
    ClientID          string
    AutoOffsetReset   string // earliest, latest
    EnableAutoCommit  bool
    AutoCommitInterval time.Duration
    SessionTimeout    time.Duration
    HeartbeatInterval time.Duration
    MaxPollRecords    int
    FetchMinBytes     int
    FetchMaxWait      time.Duration
}
```

### Consumer Group Management

#### Group Coordination
- **Group Membership**: Dynamic consumer registration/deregistration
- **Partition Assignment**: Automatic partition-to-consumer assignment
- **Rebalancing**: Reassign partitions when group membership changes
- **Offset Management**: Track and commit consumer progress

#### Assignment Strategies
1. **Range Assignment**: Assign consecutive partitions to consumers
2. **Round-Robin Assignment**: Distribute partitions evenly
3. **Sticky Assignment**: Minimize partition movement during rebalancing

#### Offset Management
```go
type OffsetManager interface {
    CommitOffset(topic string, partition int32, offset int64) error
    FetchOffset(topic string, partition int32) (int64, error)
    ResetOffset(topic string, partition int32, position string) error
}
```

## Persistence and Durability

### Write-Ahead Log (WAL)
- **Sequential Writes**: Append-only log for optimal disk performance
- **Fsync Policy**: Configurable sync frequency for durability vs. performance
- **Log Rotation**: Automatic log segment rotation based on size/time
- **Compaction**: Background log compaction for space efficiency

### Replication
- **Synchronous Replication**: Messages replicated to all ISR before acknowledgment
- **Leader-Follower Model**: One leader per partition, multiple followers
- **Replica Lag Monitoring**: Track follower lag and ISR membership
- **Automatic Failover**: Promote follower to leader on failure

### Data Retention
- **Time-based Retention**: Delete messages older than retention period
- **Size-based Retention**: Delete oldest messages when size limit exceeded
- **Compaction**: Keep only latest value for each key (log compaction)

## Scalability and Performance

### Horizontal Scaling

#### Broker Scaling
- **Dynamic Broker Addition**: Add brokers without service interruption
- **Partition Reassignment**: Redistribute partitions across brokers
- **Load Monitoring**: Track broker CPU, memory, disk, and network usage
- **Auto-scaling Integration**: Kubernetes HPA integration

#### Performance Targets
- **Throughput**: 100,000+ messages/second per broker
- **Latency**: P99 < 10ms for end-to-end delivery
- **Concurrent Connections**: 10,000+ producer/consumer connections
- **Message Size**: Support up to 1MB messages

### Performance Optimizations

#### Network Layer
- **Zero-Copy Transfers**: Use sendfile() for efficient data transfer
- **Connection Multiplexing**: Multiple requests per connection
- **Compression**: Reduce network bandwidth usage
- **Keep-Alive**: Persistent connections for reduced overhead

#### Storage Layer
- **Page Cache Utilization**: Leverage OS page cache for reads
- **Sequential I/O**: Optimize for sequential disk access patterns
- **Memory Mapping**: Use mmap for efficient file access
- **Batch Writes**: Group multiple writes for better throughput

#### Memory Management
- **Object Pooling**: Reuse objects to reduce GC pressure
- **Off-heap Storage**: Store messages outside JVM heap (if applicable)
- **Buffer Management**: Efficient buffer allocation and recycling

## Fault Tolerance

### Failure Scenarios and Handling

#### Broker Failures
- **Leader Election**: Automatic leader promotion from ISR
- **Partition Reassignment**: Redistribute partitions from failed broker
- **Client Failover**: Clients automatically discover new leaders
- **Data Recovery**: Recover data from replicas and WAL

#### Network Partitions
- **Split-brain Prevention**: Require majority quorum for operations
- **Graceful Degradation**: Continue serving available partitions
- **Automatic Recovery**: Rejoin cluster when partition heals
- **Consistency Guarantees**: Maintain data consistency during partitions

#### Data Corruption
- **Checksum Validation**: Detect corrupted messages
- **Replica Comparison**: Cross-check data across replicas
- **Automatic Repair**: Replace corrupted data from healthy replicas
- **Alert Generation**: Notify operators of corruption events

### High Availability Features
- **Multi-AZ Deployment**: Deploy across multiple availability zones
- **Backup and Restore**: Regular data backups to external storage
- **Disaster Recovery**: Cross-region replication for DR scenarios
- **Health Checks**: Comprehensive health monitoring and alerting

## Configuration

### Broker Configuration
```yaml
# Broker Settings
broker:
  id: 1
  host: "0.0.0.0"
  port: 9092
  data_dir: "/data/message-queue"
  
# Cluster Settings  
cluster:
  bootstrap_servers: 
    - "broker-1:9092"
    - "broker-2:9092"
    - "broker-3:9092"
  
# Storage Settings
storage:
  segment_size: "1GB"
  segment_time: "24h"
  retention_time: "7d"
  retention_size: "100GB"
  fsync_interval: "1s"
  
# Performance Settings
performance:
  max_connections: 10000
  socket_send_buffer: "100KB"
  socket_receive_buffer: "100KB"
  max_message_size: "1MB"
  batch_size: "16KB"
  linger_time: "5ms"
  
# Replication Settings
replication:
  default_replication_factor: 3
  min_insync_replicas: 2
  replica_lag_time_max: "10s"
  replica_lag_records_max: 1000
```

### Topic Configuration
```yaml
topics:
  gpu-telemetry:
    partitions: 12
    replication_factor: 3
    retention_time: "30d"
    retention_size: "500GB"
    compression_type: "snappy"
    cleanup_policy: "delete"
    segment_size: "1GB"
    
  system-metrics:
    partitions: 6
    replication_factor: 2
    retention_time: "7d"
    compression_type: "gzip"
```

## Monitoring and Observability

### Metrics Collection
```go
type BrokerMetrics struct {
    // Throughput Metrics
    MessagesInPerSec    float64
    MessagesOutPerSec   float64
    BytesInPerSec       float64
    BytesOutPerSec      float64
    
    // Latency Metrics
    ProduceLatencyP99   time.Duration
    FetchLatencyP99     time.Duration
    
    // Resource Metrics
    CPUUsage           float64
    MemoryUsage        int64
    DiskUsage          int64
    NetworkIO          int64
    
    // Partition Metrics
    PartitionCount     int
    LeaderCount        int
    ReplicaCount       int
    UnderReplicatedPartitions int
    
    // Error Metrics
    ProduceErrors      int64
    FetchErrors        int64
    ReplicationErrors  int64
}
```

### Health Checks
- **Broker Health**: CPU, memory, disk space, network connectivity
- **Cluster Health**: Leader distribution, replica synchronization
- **Topic Health**: Partition availability, replication status
- **Client Health**: Producer/consumer connection status

### Alerting Rules
- **High Latency**: P99 latency > 100ms
- **Low Throughput**: Messages/sec below threshold
- **Replica Lag**: Follower lag > 10 seconds
- **Disk Space**: Available space < 20%
- **Connection Errors**: Error rate > 1%

### Dashboards
- **Cluster Overview**: Broker status, topic summary, throughput
- **Broker Details**: Resource usage, partition assignments, performance
- **Topic Analytics**: Message rates, size distribution, consumer lag
- **Client Monitoring**: Producer/consumer metrics, error rates

## Implementation Details

### Technology Stack
- **Language**: Go 1.21+
- **Networking**: TCP with custom binary protocol
- **Storage**: Local filesystem with WAL
- **Serialization**: Protocol Buffers or MessagePack
- **Consensus**: Raft algorithm for leader election
- **Monitoring**: Prometheus metrics, structured logging

### Package Structure
```
pkg/messagequeue/
├── broker/          # Broker implementation
├── client/          # Producer/Consumer clients
├── protocol/        # Wire protocol definitions
├── storage/         # Storage engine
├── cluster/         # Cluster coordination
├── partition/       # Partition management
├── replication/     # Replication logic
├── metrics/         # Monitoring and metrics
└── config/          # Configuration management
```

### Key Interfaces
```go
// Core broker interface
type Broker interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    CreateTopic(topic string, config TopicConfig) error
    DeleteTopic(topic string) error
    GetMetadata() *ClusterMetadata
}

// Storage interface
type Storage interface {
    Append(partition string, messages []*Message) error
    Read(partition string, offset int64, maxBytes int) ([]*Message, error)
    GetHighWatermark(partition string) int64
    CreatePartition(partition string) error
    DeletePartition(partition string) error
}

// Replication interface
type Replicator interface {
    StartReplication(partition string, leader string) error
    StopReplication(partition string) error
    SyncReplica(partition string, offset int64) error
    GetReplicationLag(partition string) int64
}
```

This custom message queue design provides a robust, scalable, and performant foundation for the GPU telemetry pipeline, with careful consideration of reliability, observability, and operational requirements.

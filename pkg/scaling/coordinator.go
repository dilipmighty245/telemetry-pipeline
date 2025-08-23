// Package scaling provides distributed auto-scaling functionality for microservices.
// It includes metrics collection, scaling decision algorithms, and coordination
// mechanisms to automatically scale services based on load and performance metrics.
package scaling

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dilipmighty245/telemetry-pipeline/pkg/logging"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// InstanceMetrics represents performance and health metrics for a service instance.
// These metrics are used by the scaling coordinator to make scaling decisions.
type InstanceMetrics struct {
	InstanceID     string    `json:"instance_id"`     // Unique identifier for the service instance
	ServiceType    string    `json:"service_type"`    // Type of service (e.g., "collector", "streamer")
	CPUUsage       float64   `json:"cpu_usage"`       // CPU utilization as a percentage (0.0-1.0)
	MemoryUsage    float64   `json:"memory_usage"`    // Memory utilization as a percentage (0.0-1.0)
	QueueDepth     float64   `json:"queue_depth"`     // Number of items in processing queue
	ProcessingRate float64   `json:"processing_rate"` // Items processed per second
	ErrorRate      float64   `json:"error_rate"`      // Error rate as a percentage (0.0-1.0)
	Timestamp      time.Time `json:"timestamp"`       // When the metrics were collected
	Health         string    `json:"health"`          // Health status ("healthy", "unhealthy", "degraded")
}

// ScalingDecision represents a scaling recommendation made by the coordinator.
// It contains the decision details, rationale, and confidence level.
type ScalingDecision struct {
	ServiceType      string    `json:"service_type"`      // Type of service this decision applies to
	Action           string    `json:"action"`            // Scaling action: "scale_up", "scale_down", "no_action"
	CurrentCount     int       `json:"current_count"`     // Current number of service instances
	RecommendedCount int       `json:"recommended_count"` // Recommended number of instances
	Reason           string    `json:"reason"`            // Human-readable reason for the decision
	Confidence       float64   `json:"confidence"`        // Confidence level in the decision (0.0-1.0)
	Timestamp        time.Time `json:"timestamp"`         // When the decision was made
}

// ScalingRules defines the scaling behavior and thresholds for a service type.
// These rules control when and how scaling decisions are made.
type ScalingRules struct {
	MinInstances       int                `json:"min_instances"`        // Minimum number of instances to maintain
	MaxInstances       int                `json:"max_instances"`        // Maximum number of instances allowed
	ScaleUpThreshold   float64            `json:"scale_up_threshold"`   // Load threshold to trigger scale-up (0.0-1.0)
	ScaleDownThreshold float64            `json:"scale_down_threshold"` // Load threshold to trigger scale-down (0.0-1.0)
	CooldownPeriod     time.Duration      `json:"cooldown_period"`      // Minimum time between scaling actions
	MetricWeights      map[string]float64 `json:"metric_weights"`       // Weights for different metrics in load calculation
}

// ScalingCoordinator manages distributed scaling decisions for a service type.
// It collects metrics from service instances, analyzes load patterns, and makes
// scaling recommendations while coordinating with other coordinators to avoid conflicts.
type ScalingCoordinator struct {
	client      *clientv3.Client   // etcd client for coordination and data storage
	serviceType string             // Type of service this coordinator manages
	instanceID  string             // Unique ID of this coordinator instance
	rules       *ScalingRules      // Scaling rules and thresholds
	leaseID     clientv3.LeaseID   // etcd lease ID for this coordinator
	mu          sync.RWMutex       // Mutex for thread-safe access
	ctx         context.Context    // Context for cancellation
	cancel      context.CancelFunc // Cancel function for graceful shutdown
}

// NewScalingCoordinator creates a new scaling coordinator for the specified service type.
// If rules is nil, default scaling rules will be applied.
//
// Parameters:
//   - client: etcd client for coordination and storage
//   - serviceType: Type of service to manage (e.g., "collector", "streamer")
//   - instanceID: Unique identifier for this coordinator instance
//   - rules: Scaling rules, or nil to use defaults
//
// Returns:
//   - *ScalingCoordinator: A new scaling coordinator instance
func NewScalingCoordinator(client *clientv3.Client, serviceType, instanceID string, rules *ScalingRules) *ScalingCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	if rules == nil {
		rules = &ScalingRules{
			MinInstances:       1,
			MaxInstances:       10,
			ScaleUpThreshold:   0.8,
			ScaleDownThreshold: 0.3,
			CooldownPeriod:     5 * time.Minute,
			MetricWeights: map[string]float64{
				"cpu":    0.3,
				"memory": 0.3,
				"queue":  0.4,
			},
		}
	}

	return &ScalingCoordinator{
		client:      client,
		serviceType: serviceType,
		instanceID:  instanceID,
		rules:       rules,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the scaling coordinator and begins metrics reporting and decision making.
// It creates an etcd lease, starts background goroutines for metrics reporting and
// scaling decisions, and begins the keep-alive process for the lease.
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (sc *ScalingCoordinator) Start() error {
	// Create lease for metrics reporting
	lease, err := sc.client.Grant(sc.ctx, 60) // 60 second TTL
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}
	sc.leaseID = lease.ID

	// Keep lease alive
	ch, err := sc.client.KeepAlive(sc.ctx, sc.leaseID)
	if err != nil {
		return fmt.Errorf("failed to start keep alive: %w", err)
	}

	go func() {
		for ka := range ch {
			if ka == nil {
				return
			}
		}
	}()

	// Start metrics reporting
	go sc.startMetricsReporting()

	// Start scaling decision making (only one coordinator should do this)
	go sc.startScalingDecisionMaking()

	logging.Infof("Scaling coordinator started for %s/%s", sc.serviceType, sc.instanceID)
	return nil
}

// ReportMetrics reports instance metrics to etcd for use in scaling decisions.
// The metrics are stored with a lease to ensure they expire if the instance becomes unavailable.
//
// Parameters:
//   - metrics: The instance metrics to report
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (sc *ScalingCoordinator) ReportMetrics(metrics InstanceMetrics) error {
	metrics.InstanceID = sc.instanceID
	metrics.ServiceType = sc.serviceType
	metrics.Timestamp = time.Now()

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	metricsKey := fmt.Sprintf("/metrics/%s/%s", sc.serviceType, sc.instanceID)

	ctx, cancel := context.WithTimeout(sc.ctx, 5*time.Second)
	defer cancel()

	_, err = sc.client.Put(ctx, metricsKey, string(data), clientv3.WithLease(sc.leaseID))
	if err != nil {
		return fmt.Errorf("failed to report metrics: %w", err)
	}

	logging.Debugf("Reported metrics for %s: CPU=%.2f%%, Memory=%.2f%%, Queue=%.0f",
		sc.instanceID, metrics.CPUUsage*100, metrics.MemoryUsage*100, metrics.QueueDepth)
	return nil
}

// GetScalingDecision analyzes current metrics and returns a scaling recommendation.
// It considers all active instances, calculates aggregate load, checks cooldown periods,
// and applies scaling rules to determine the appropriate action.
//
// Returns:
//   - *ScalingDecision: The scaling decision with action, counts, and rationale
//   - error: nil on success, error describing the failure otherwise
func (sc *ScalingCoordinator) GetScalingDecision() (*ScalingDecision, error) {
	ctx, cancel := context.WithTimeout(sc.ctx, 10*time.Second)
	defer cancel()

	// Get all metrics for this service type
	resp, err := sc.client.Get(ctx, fmt.Sprintf("/metrics/%s/", sc.serviceType),
		clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return &ScalingDecision{
			ServiceType:      sc.serviceType,
			Action:           "no_action",
			CurrentCount:     0,
			RecommendedCount: sc.rules.MinInstances,
			Reason:           "no metrics available",
			Confidence:       0.5,
			Timestamp:        time.Now(),
		}, nil
	}

	// Parse all metrics
	var allMetrics []InstanceMetrics
	healthyInstances := 0

	for _, kv := range resp.Kvs {
		var metrics InstanceMetrics
		if err := json.Unmarshal(kv.Value, &metrics); err != nil {
			logging.Errorf("Failed to unmarshal metrics from %s: %v", kv.Key, err)
			continue
		}

		// Skip old metrics (older than 2 minutes)
		if time.Since(metrics.Timestamp) > 2*time.Minute {
			continue
		}

		allMetrics = append(allMetrics, metrics)
		if metrics.Health == "healthy" {
			healthyInstances++
		}
	}

	if len(allMetrics) == 0 {
		return &ScalingDecision{
			ServiceType:      sc.serviceType,
			Action:           "no_action",
			CurrentCount:     0,
			RecommendedCount: sc.rules.MinInstances,
			Reason:           "no recent metrics available",
			Confidence:       0.5,
			Timestamp:        time.Now(),
		}, nil
	}

	// Calculate aggregate metrics
	avgLoad := sc.calculateAggregateLoad(allMetrics)
	currentCount := len(allMetrics)

	// Check cooldown period
	if sc.isInCooldownPeriod() {
		return &ScalingDecision{
			ServiceType:      sc.serviceType,
			Action:           "no_action",
			CurrentCount:     currentCount,
			RecommendedCount: currentCount,
			Reason:           "in cooldown period",
			Confidence:       0.8,
			Timestamp:        time.Now(),
		}, nil
	}

	// Make scaling decision
	decision := &ScalingDecision{
		ServiceType:  sc.serviceType,
		CurrentCount: currentCount,
		Timestamp:    time.Now(),
	}

	if avgLoad > sc.rules.ScaleUpThreshold && currentCount < sc.rules.MaxInstances {
		decision.Action = "scale_up"
		decision.RecommendedCount = minInt(currentCount+1, sc.rules.MaxInstances)
		decision.Reason = fmt.Sprintf("high load detected: %.2f%% > %.2f%%",
			avgLoad*100, sc.rules.ScaleUpThreshold*100)
		decision.Confidence = minFloat(avgLoad/sc.rules.ScaleUpThreshold, 1.0)
	} else if avgLoad < sc.rules.ScaleDownThreshold && currentCount > sc.rules.MinInstances {
		decision.Action = "scale_down"
		decision.RecommendedCount = maxInt(currentCount-1, sc.rules.MinInstances)
		decision.Reason = fmt.Sprintf("low load detected: %.2f%% < %.2f%%",
			avgLoad*100, sc.rules.ScaleDownThreshold*100)
		decision.Confidence = minFloat(sc.rules.ScaleDownThreshold/avgLoad, 1.0)
	} else {
		decision.Action = "no_action"
		decision.RecommendedCount = currentCount
		decision.Reason = fmt.Sprintf("load within acceptable range: %.2f%%", avgLoad*100)
		decision.Confidence = 0.9
	}

	return decision, nil
}

// calculateAggregateLoad calculates the weighted average load across all instances.
// It applies the configured metric weights to CPU, memory, and queue depth metrics
// to produce a single load value between 0.0 and 1.0.
//
// Parameters:
//   - metrics: Slice of instance metrics to analyze
//
// Returns:
//   - float64: Aggregate load value (0.0-1.0)
func (sc *ScalingCoordinator) calculateAggregateLoad(metrics []InstanceMetrics) float64 {
	if len(metrics) == 0 {
		return 0
	}

	var totalLoad float64
	for _, m := range metrics {
		instanceLoad := (m.CPUUsage * sc.rules.MetricWeights["cpu"]) +
			(m.MemoryUsage * sc.rules.MetricWeights["memory"]) +
			(minFloat(m.QueueDepth/100.0, 1.0) * sc.rules.MetricWeights["queue"])
		totalLoad += instanceLoad
	}

	return totalLoad / float64(len(metrics))
}

// isInCooldownPeriod checks if the service is currently in a cooldown period.
// During cooldown, no scaling actions are taken to prevent oscillation.
//
// Returns:
//   - bool: true if in cooldown period, false otherwise
func (sc *ScalingCoordinator) isInCooldownPeriod() bool {
	ctx, cancel := context.WithTimeout(sc.ctx, 5*time.Second)
	defer cancel()

	// Check for recent scaling decisions
	resp, err := sc.client.Get(ctx, fmt.Sprintf("/scaling/decisions/%s", sc.serviceType))
	if err != nil {
		return false // No previous decision, not in cooldown
	}

	if len(resp.Kvs) == 0 {
		return false
	}

	var lastDecision ScalingDecision
	if err := json.Unmarshal(resp.Kvs[0].Value, &lastDecision); err != nil {
		return false
	}

	// Check if the last scaling action was recent
	if lastDecision.Action != "no_action" &&
		time.Since(lastDecision.Timestamp) < sc.rules.CooldownPeriod {
		return true
	}

	return false
}

// PublishScalingDecision publishes a scaling decision to etcd for other components to act upon.
// The decision is stored in a well-known location where scaling executors can find it.
//
// Parameters:
//   - decision: The scaling decision to publish
//
// Returns:
//   - error: nil on success, error describing the failure otherwise
func (sc *ScalingCoordinator) PublishScalingDecision(decision *ScalingDecision) error {
	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("failed to marshal scaling decision: %w", err)
	}

	decisionKey := fmt.Sprintf("/scaling/decisions/%s", sc.serviceType)

	ctx, cancel := context.WithTimeout(sc.ctx, 5*time.Second)
	defer cancel()

	_, err = sc.client.Put(ctx, decisionKey, string(data))
	if err != nil {
		return fmt.Errorf("failed to publish scaling decision: %w", err)
	}

	logging.Infof("Published scaling decision for %s: %s (%d -> %d)",
		sc.serviceType, decision.Action, decision.CurrentCount, decision.RecommendedCount)
	return nil
}

// startMetricsReporting starts periodic metrics reporting
func (sc *ScalingCoordinator) startMetricsReporting() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// This would typically collect real metrics from the system
			// For now, we'll use placeholder values
			metrics := InstanceMetrics{
				CPUUsage:       0.5,   // 50%
				MemoryUsage:    0.6,   // 60%
				QueueDepth:     10.0,  // 10 messages
				ProcessingRate: 100.0, // 100 messages/sec
				ErrorRate:      0.01,  // 1%
				Health:         "healthy",
			}

			if err := sc.ReportMetrics(metrics); err != nil {
				logging.Errorf("Failed to report metrics: %v", err)
			}
		case <-sc.ctx.Done():
			return
		}
	}
}

// startScalingDecisionMaking starts the scaling decision making process
func (sc *ScalingCoordinator) startScalingDecisionMaking() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Only the leader should make scaling decisions
			if sc.tryAcquireLeadership() {
				decision, err := sc.GetScalingDecision()
				if err != nil {
					logging.Errorf("Failed to get scaling decision: %v", err)
					continue
				}

				if err := sc.PublishScalingDecision(decision); err != nil {
					logging.Errorf("Failed to publish scaling decision: %v", err)
				}
			}
		case <-sc.ctx.Done():
			return
		}
	}
}

// tryAcquireLeadership tries to acquire leadership for scaling decisions
func (sc *ScalingCoordinator) tryAcquireLeadership() bool {
	leaderKey := fmt.Sprintf("/scaling/leader/%s", sc.serviceType)

	ctx, cancel := context.WithTimeout(sc.ctx, 5*time.Second)
	defer cancel()

	// Try to acquire leadership with a lease
	lease, err := sc.client.Grant(ctx, 30) // 30 second leadership
	if err != nil {
		return false
	}

	// Try to put the key only if it doesn't exist
	txnResp, err := sc.client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(leaderKey), "=", 0)).
		Then(clientv3.OpPut(leaderKey, sc.instanceID, clientv3.WithLease(lease.ID))).
		Commit()

	if err != nil || !txnResp.Succeeded {
		sc.client.Revoke(ctx, lease.ID)
		return false
	}

	return true
}

// Stop gracefully stops the scaling coordinator and cleans up resources.
// It cancels the context, revokes the etcd lease, and stops all background operations.
//
// Returns:
//   - error: Always returns nil (errors are logged but not returned)
func (sc *ScalingCoordinator) Stop() error {
	sc.cancel()

	if sc.leaseID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sc.client.Revoke(ctx, sc.leaseID)
	}

	logging.Infof("Scaling coordinator stopped for %s/%s", sc.serviceType, sc.instanceID)
	return nil
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt returns the larger of two integers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// minFloat returns the smaller of two float64 values.
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

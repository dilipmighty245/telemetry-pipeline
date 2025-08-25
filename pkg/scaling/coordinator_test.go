package scaling

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v3client"
)

// setupEtcdTestServer creates an embedded etcd server for testing
func setupEtcdTestServer(t *testing.T) (*embed.Etcd, *clientv3.Client, func()) {
	cfg := embed.NewConfig()
	cfg.Name = "test-etcd"
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error"

	// Use available ports
	clientURL, _ := url.Parse("http://127.0.0.1:0")
	peerURL, _ := url.Parse("http://127.0.0.1:0")

	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = cfg.ListenPeerUrls
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew

	e, err := embed.StartEtcd(cfg)
	require.NoError(t, err)

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Close()
		t.Fatalf("etcd server failed to start within 10 seconds")
	}

	client := v3client.New(e.Server)

	cleanup := func() {
		if client != nil {
			client.Close()
		}
		if e != nil {
			e.Close()
			select {
			case <-e.Server.StopNotify():
			case <-time.After(5 * time.Second):
			}
		}
	}

	return e, client, cleanup
}

func TestNewScalingCoordinator(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	tests := []struct {
		name        string
		serviceType string
		instanceID  string
		rules       *ScalingRules
		expectNil   bool
	}{
		{
			name:        "with default rules",
			serviceType: "collector",
			instanceID:  "test-instance",
			rules:       nil,
			expectNil:   false,
		},
		{
			name:        "with custom rules",
			serviceType: "streamer",
			instanceID:  "test-instance-2",
			rules: &ScalingRules{
				MinInstances:       2,
				MaxInstances:       20,
				ScaleUpThreshold:   0.7,
				ScaleDownThreshold: 0.2,
				CooldownPeriod:     10 * time.Minute,
				MetricWeights: map[string]float64{
					"cpu":    0.4,
					"memory": 0.3,
					"queue":  0.3,
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewScalingCoordinator(client, tt.serviceType, tt.instanceID, tt.rules)

			if tt.expectNil {
				assert.Nil(t, sc)
			} else {
				assert.NotNil(t, sc)
				assert.Equal(t, tt.serviceType, sc.serviceType)
				assert.Equal(t, tt.instanceID, sc.instanceID)
				assert.Equal(t, client, sc.client)
				assert.NotNil(t, sc.rules)

				if tt.rules == nil {
					// Check default rules
					assert.Equal(t, 1, sc.rules.MinInstances)
					assert.Equal(t, 10, sc.rules.MaxInstances)
					assert.Equal(t, 0.8, sc.rules.ScaleUpThreshold)
					assert.Equal(t, 0.3, sc.rules.ScaleDownThreshold)
					assert.Equal(t, 5*time.Minute, sc.rules.CooldownPeriod)
					assert.Contains(t, sc.rules.MetricWeights, "cpu")
					assert.Contains(t, sc.rules.MetricWeights, "memory")
					assert.Contains(t, sc.rules.MetricWeights, "queue")
				} else {
					assert.Equal(t, tt.rules, sc.rules)
				}
			}
		})
	}
}

func TestScalingCoordinator_Start(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx := context.Background()
	sc := NewScalingCoordinator(client, "test-service", "test-instance", nil)
	defer func() { _ = sc.Stop(ctx) }()

	err := sc.Start(ctx)
	assert.NoError(t, err)
	assert.NotEqual(t, clientv3.LeaseID(0), sc.leaseID)

	// Test starting again (should not error)
	err = sc.Start(ctx)
	assert.NoError(t, err)
}

func TestScalingCoordinator_ReportMetrics(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx := context.Background()

	sc := NewScalingCoordinator(client, "test-service", "test-instance", nil)
	defer func() { _ = sc.Stop(ctx) }()

	err := sc.Start(ctx)
	require.NoError(t, err)

	metrics := InstanceMetrics{
		CPUUsage:       0.75,
		MemoryUsage:    0.60,
		QueueDepth:     25.0,
		ProcessingRate: 150.0,
		ErrorRate:      0.02,
		Health:         "healthy",
	}

	err = sc.ReportMetrics(ctx, metrics)
	assert.NoError(t, err)

	// Verify metrics were stored
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	metricsKey := "/metrics/test-service/test-instance"
	resp, err := client.Get(ctx, metricsKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	var storedMetrics InstanceMetrics
	err = json.Unmarshal(resp.Kvs[0].Value, &storedMetrics)
	require.NoError(t, err)

	assert.Equal(t, "test-instance", storedMetrics.InstanceID)
	assert.Equal(t, "test-service", storedMetrics.ServiceType)
	assert.Equal(t, 0.75, storedMetrics.CPUUsage)
	assert.Equal(t, 0.60, storedMetrics.MemoryUsage)
	assert.Equal(t, 25.0, storedMetrics.QueueDepth)
	assert.Equal(t, "healthy", storedMetrics.Health)
	assert.WithinDuration(t, time.Now(), storedMetrics.Timestamp, 5*time.Second)
}

func TestScalingCoordinator_GetScalingDecision(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rules := &ScalingRules{
		MinInstances:       1,
		MaxInstances:       5,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.3,
		CooldownPeriod:     1 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.3,
			"memory": 0.3,
			"queue":  0.4,
		},
	}

	sc := NewScalingCoordinator(client, "test-service", "test-instance", rules)
	defer func() { _ = sc.Stop(ctx) }()

	err := sc.Start(ctx)
	require.NoError(t, err)

	t.Run("no metrics available", func(t *testing.T) {
		decision, err := sc.GetScalingDecision(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "no_action", decision.Action)
		assert.Equal(t, 0, decision.CurrentCount)
		assert.Equal(t, 1, decision.RecommendedCount)
		assert.Contains(t, decision.Reason, "no metrics available")
	})

	t.Run("scale up decision", func(t *testing.T) {
		// Report high load metrics
		highLoadMetrics := InstanceMetrics{
			CPUUsage:    0.9,
			MemoryUsage: 0.85,
			QueueDepth:  100.0,
			Health:      "healthy",
		}
		err := sc.ReportMetrics(ctx, highLoadMetrics)
		require.NoError(t, err)

		decision, err := sc.GetScalingDecision(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "scale_up", decision.Action)
		assert.Equal(t, 1, decision.CurrentCount)
		assert.Equal(t, 2, decision.RecommendedCount)
		assert.Contains(t, decision.Reason, "high load detected")
		assert.Greater(t, decision.Confidence, 0.0)
	})

	t.Run("scale down decision", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// First, add multiple instances with low load
		for i := 0; i < 3; i++ {
			instanceSC := NewScalingCoordinator(client, "test-service", fmt.Sprintf("instance-%d", i), rules)
			err := instanceSC.Start(ctx)
			require.NoError(t, err)
			defer func() { _ = instanceSC.Stop(ctx) }()

			lowLoadMetrics := InstanceMetrics{
				CPUUsage:    0.1,
				MemoryUsage: 0.15,
				QueueDepth:  2.0,
				Health:      "healthy",
			}
			err = instanceSC.ReportMetrics(ctx, lowLoadMetrics)
			require.NoError(t, err)
		}

		// Wait a bit for metrics to be stored
		time.Sleep(100 * time.Millisecond)

		decision, err := sc.GetScalingDecision(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "scale_down", decision.Action)
		assert.Greater(t, decision.CurrentCount, 1)
		assert.Equal(t, decision.CurrentCount-1, decision.RecommendedCount)
		assert.Contains(t, decision.Reason, "low load detected")
	})
}

func TestScalingCoordinator_CalculateAggregateLoad(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	rules := &ScalingRules{
		MetricWeights: map[string]float64{
			"cpu":    0.3,
			"memory": 0.3,
			"queue":  0.4,
		},
	}

	sc := NewScalingCoordinator(client, "test-service", "test-instance", rules)

	t.Run("empty metrics", func(t *testing.T) {
		load := sc.calculateAggregateLoad([]InstanceMetrics{})
		assert.Equal(t, 0.0, load)
	})

	t.Run("single instance", func(t *testing.T) {
		metrics := []InstanceMetrics{
			{
				CPUUsage:    0.5,
				MemoryUsage: 0.6,
				QueueDepth:  50.0, // This will be normalized to 0.5 (50/100)
			},
		}
		load := sc.calculateAggregateLoad(metrics)
		expected := (0.5 * 0.3) + (0.6 * 0.3) + (0.5 * 0.4)
		assert.InDelta(t, expected, load, 0.01)
	})

	t.Run("multiple instances", func(t *testing.T) {
		metrics := []InstanceMetrics{
			{CPUUsage: 0.8, MemoryUsage: 0.7, QueueDepth: 80.0},
			{CPUUsage: 0.4, MemoryUsage: 0.5, QueueDepth: 20.0},
		}
		load := sc.calculateAggregateLoad(metrics)

		// Calculate expected load
		load1 := (0.8 * 0.3) + (0.7 * 0.3) + (0.8 * 0.4) // 80/100 = 0.8
		load2 := (0.4 * 0.3) + (0.5 * 0.3) + (0.2 * 0.4) // 20/100 = 0.2
		expected := (load1 + load2) / 2

		assert.InDelta(t, expected, load, 0.01)
	})
}

func TestScalingCoordinator_IsInCooldownPeriod(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rules := &ScalingRules{
		CooldownPeriod: 1 * time.Minute,
	}

	sc := NewScalingCoordinator(client, "test-service", "test-instance", rules)
	defer func() { _ = sc.Stop(ctx) }()

	err := sc.Start(ctx)
	require.NoError(t, err)

	t.Run("no previous decision", func(t *testing.T) {
		inCooldown := sc.isInCooldownPeriod(ctx)
		assert.False(t, inCooldown)
	})

	t.Run("recent scaling decision", func(t *testing.T) {
		// Publish a recent scaling decision
		decision := &ScalingDecision{
			ServiceType:      "test-service",
			Action:           "scale_up",
			CurrentCount:     1,
			RecommendedCount: 2,
			Timestamp:        time.Now(),
		}
		err := sc.PublishScalingDecision(ctx, decision)
		require.NoError(t, err)

		inCooldown := sc.isInCooldownPeriod(ctx)
		assert.True(t, inCooldown)
	})

	t.Run("old scaling decision", func(t *testing.T) {
		// Publish an old scaling decision
		decision := &ScalingDecision{
			ServiceType:      "test-service",
			Action:           "scale_up",
			CurrentCount:     1,
			RecommendedCount: 2,
			Timestamp:        time.Now().Add(-2 * time.Minute), // Older than cooldown
		}
		err := sc.PublishScalingDecision(ctx, decision)
		require.NoError(t, err)

		inCooldown := sc.isInCooldownPeriod(ctx)
		assert.False(t, inCooldown)
	})

	t.Run("no_action decision", func(t *testing.T) {
		// Publish a no_action decision (should not trigger cooldown)
		decision := &ScalingDecision{
			ServiceType:      "test-service",
			Action:           "no_action",
			CurrentCount:     2,
			RecommendedCount: 2,
			Timestamp:        time.Now(),
		}
		err := sc.PublishScalingDecision(ctx, decision)
		require.NoError(t, err)

		inCooldown := sc.isInCooldownPeriod(ctx)
		assert.False(t, inCooldown)
	})
}

func TestScalingCoordinator_PublishScalingDecision(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := NewScalingCoordinator(client, "test-service", "test-instance", nil)
	defer func() { _ = sc.Stop(ctx) }()

	err := sc.Start(ctx)
	require.NoError(t, err)

	decision := &ScalingDecision{
		ServiceType:      "test-service",
		Action:           "scale_up",
		CurrentCount:     1,
		RecommendedCount: 2,
		Reason:           "high load detected",
		Confidence:       0.85,
		Timestamp:        time.Now(),
	}

	err = sc.PublishScalingDecision(ctx, decision)
	assert.NoError(t, err)

	decisionKey := "/scaling/decisions/test-service"
	resp, err := client.Get(ctx, decisionKey)
	require.NoError(t, err)
	require.Len(t, resp.Kvs, 1)

	var storedDecision ScalingDecision
	err = json.Unmarshal(resp.Kvs[0].Value, &storedDecision)
	require.NoError(t, err)

	assert.Equal(t, decision.ServiceType, storedDecision.ServiceType)
	assert.Equal(t, decision.Action, storedDecision.Action)
	assert.Equal(t, decision.CurrentCount, storedDecision.CurrentCount)
	assert.Equal(t, decision.RecommendedCount, storedDecision.RecommendedCount)
	assert.Equal(t, decision.Reason, storedDecision.Reason)
	assert.Equal(t, decision.Confidence, storedDecision.Confidence)
}

func TestScalingCoordinator_Stop(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := NewScalingCoordinator(client, "test-service", "test-instance", nil)

	err := sc.Start(ctx)
	require.NoError(t, err)

	// Stop should not error
	err = sc.Stop(ctx)
	assert.NoError(t, err)

	// Stop again should not error
	err = sc.Stop(ctx)
	assert.NoError(t, err)
}

func TestScalingCoordinator_TryAcquireLeadership(t *testing.T) {
	_, client, cleanup := setupEtcdTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc1 := NewScalingCoordinator(client, "test-service", "instance-1", nil)
	sc2 := NewScalingCoordinator(client, "test-service", "instance-2", nil)
	defer func() { _ = sc1.Stop(ctx) }()
	defer func() { _ = sc2.Stop(ctx) }()

	err := sc1.Start(ctx)
	require.NoError(t, err)
	err = sc2.Start(ctx)
	require.NoError(t, err)

	// First instance should acquire leadership
	acquired1 := sc1.tryAcquireLeadership(ctx)
	assert.True(t, acquired1)

	// Second instance should not acquire leadership
	acquired2 := sc2.tryAcquireLeadership(ctx)
	assert.False(t, acquired2)
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("minInt", func(t *testing.T) {
		assert.Equal(t, 1, minInt(1, 2))
		assert.Equal(t, 1, minInt(2, 1))
		assert.Equal(t, 5, minInt(5, 5))
		assert.Equal(t, -1, minInt(-1, 0))
	})

	t.Run("maxInt", func(t *testing.T) {
		assert.Equal(t, 2, maxInt(1, 2))
		assert.Equal(t, 2, maxInt(2, 1))
		assert.Equal(t, 5, maxInt(5, 5))
		assert.Equal(t, 0, maxInt(-1, 0))
	})

	t.Run("minFloat", func(t *testing.T) {
		assert.Equal(t, 1.5, minFloat(1.5, 2.5))
		assert.Equal(t, 1.5, minFloat(2.5, 1.5))
		assert.Equal(t, 5.0, minFloat(5.0, 5.0))
		assert.Equal(t, -1.5, minFloat(-1.5, 0.0))
	})
}

func TestScalingRulesValidation(t *testing.T) {
	rules := &ScalingRules{
		MinInstances:       2,
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

	assert.Equal(t, 2, rules.MinInstances)
	assert.Equal(t, 10, rules.MaxInstances)
	assert.Equal(t, 0.8, rules.ScaleUpThreshold)
	assert.Equal(t, 0.3, rules.ScaleDownThreshold)
	assert.Equal(t, 5*time.Minute, rules.CooldownPeriod)
	assert.Len(t, rules.MetricWeights, 3)

	// Verify weights sum to 1.0
	total := 0.0
	for _, weight := range rules.MetricWeights {
		total += weight
	}
	assert.InDelta(t, 1.0, total, 0.01)
}

func TestInstanceMetricsValidation(t *testing.T) {
	metrics := InstanceMetrics{
		InstanceID:     "test-instance",
		ServiceType:    "collector",
		CPUUsage:       0.75,
		MemoryUsage:    0.60,
		QueueDepth:     25.0,
		ProcessingRate: 150.0,
		ErrorRate:      0.02,
		Timestamp:      time.Now(),
		Health:         "healthy",
	}

	assert.Equal(t, "test-instance", metrics.InstanceID)
	assert.Equal(t, "collector", metrics.ServiceType)
	assert.True(t, metrics.CPUUsage >= 0.0 && metrics.CPUUsage <= 1.0)
	assert.True(t, metrics.MemoryUsage >= 0.0 && metrics.MemoryUsage <= 1.0)
	assert.True(t, metrics.ErrorRate >= 0.0 && metrics.ErrorRate <= 1.0)
	assert.True(t, metrics.QueueDepth >= 0.0)
	assert.True(t, metrics.ProcessingRate >= 0.0)
	assert.Contains(t, []string{"healthy", "unhealthy", "degraded"}, metrics.Health)
}

func TestScalingDecisionValidation(t *testing.T) {
	decision := ScalingDecision{
		ServiceType:      "collector",
		Action:           "scale_up",
		CurrentCount:     2,
		RecommendedCount: 3,
		Reason:           "high load detected",
		Confidence:       0.85,
		Timestamp:        time.Now(),
	}

	assert.Equal(t, "collector", decision.ServiceType)
	assert.Contains(t, []string{"scale_up", "scale_down", "no_action"}, decision.Action)
	assert.True(t, decision.CurrentCount >= 0)
	assert.True(t, decision.RecommendedCount >= 0)
	assert.NotEmpty(t, decision.Reason)
	assert.True(t, decision.Confidence >= 0.0 && decision.Confidence <= 1.0)
	assert.WithinDuration(t, time.Now(), decision.Timestamp, time.Second)
}

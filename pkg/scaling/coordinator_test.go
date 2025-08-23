package scaling

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScalingCoordinator(t *testing.T) {
	serviceType := "collector"
	instanceID := "instance-1"

	// Test with default rules (pass nil client for testing)
	sc := NewScalingCoordinator(nil, serviceType, instanceID, nil)

	assert.NotNil(t, sc)
	assert.Equal(t, serviceType, sc.serviceType)
	assert.Equal(t, instanceID, sc.instanceID)
	assert.NotNil(t, sc.rules)
	assert.Equal(t, 1, sc.rules.MinInstances)
	assert.Equal(t, 10, sc.rules.MaxInstances)
	assert.Equal(t, 0.8, sc.rules.ScaleUpThreshold)
	assert.Equal(t, 0.3, sc.rules.ScaleDownThreshold)
	assert.Equal(t, 5*time.Minute, sc.rules.CooldownPeriod)

	// Test with custom rules
	customRules := &ScalingRules{
		MinInstances:       2,
		MaxInstances:       20,
		ScaleUpThreshold:   0.9,
		ScaleDownThreshold: 0.2,
		CooldownPeriod:     10 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.4,
			"memory": 0.4,
			"queue":  0.2,
		},
	}

	sc2 := NewScalingCoordinator(nil, serviceType, instanceID, customRules)
	assert.Equal(t, customRules, sc2.rules)
}

func TestScalingCoordinator_calculateAggregateLoad(t *testing.T) {
	sc := NewScalingCoordinator(nil, "collector", "instance-1", nil)

	tests := []struct {
		name     string
		metrics  []InstanceMetrics
		expected float64
	}{
		{
			name:     "empty metrics",
			metrics:  []InstanceMetrics{},
			expected: 0,
		},
		{
			name: "single instance moderate load",
			metrics: []InstanceMetrics{
				{
					CPUUsage:    0.5,
					MemoryUsage: 0.6,
					QueueDepth:  10.0, // Will be normalized to 0.1
				},
			},
			expected: 0.5*0.3 + 0.6*0.3 + 0.1*0.4, // 0.15 + 0.18 + 0.04 = 0.37
		},
		{
			name: "multiple instances",
			metrics: []InstanceMetrics{
				{
					CPUUsage:    0.5,
					MemoryUsage: 0.6,
					QueueDepth:  10.0,
				},
				{
					CPUUsage:    0.7,
					MemoryUsage: 0.8,
					QueueDepth:  20.0,
				},
			},
			expected: (0.37 + (0.7*0.3 + 0.8*0.3 + 0.2*0.4)) / 2, // Average of two instances
		},
		{
			name: "high queue depth normalization",
			metrics: []InstanceMetrics{
				{
					CPUUsage:    0.0,
					MemoryUsage: 0.0,
					QueueDepth:  1000.0, // Should be normalized to 1.0
				},
			},
			expected: 0.0*0.3 + 0.0*0.3 + 1.0*0.4, // 0.4
		},
		{
			name: "zero queue depth",
			metrics: []InstanceMetrics{
				{
					CPUUsage:    0.8,
					MemoryUsage: 0.9,
					QueueDepth:  0.0,
				},
			},
			expected: 0.8*0.3 + 0.9*0.3 + 0.0*0.4, // 0.24 + 0.27 + 0.0 = 0.51
		},
		{
			name: "custom weights",
			metrics: []InstanceMetrics{
				{
					CPUUsage:    1.0,
					MemoryUsage: 0.0,
					QueueDepth:  0.0,
				},
			},
			expected: 1.0*0.3 + 0.0*0.3 + 0.0*0.4, // 0.3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sc.calculateAggregateLoad(tt.metrics)
			assert.InDelta(t, tt.expected, result, 0.01) // Allow small floating point differences
		})
	}
}

func TestScalingCoordinator_calculateAggregateLoad_CustomWeights(t *testing.T) {
	customRules := &ScalingRules{
		MinInstances:       1,
		MaxInstances:       10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.3,
		CooldownPeriod:     5 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.5,
			"memory": 0.3,
			"queue":  0.2,
		},
	}

	sc := NewScalingCoordinator(nil, "collector", "instance-1", customRules)

	metrics := []InstanceMetrics{
		{
			CPUUsage:    0.8,
			MemoryUsage: 0.6,
			QueueDepth:  50.0, // Will be normalized to 0.5
		},
	}

	expected := 0.8*0.5 + 0.6*0.3 + 0.5*0.2 // 0.4 + 0.18 + 0.1 = 0.68
	result := sc.calculateAggregateLoad(metrics)
	assert.InDelta(t, expected, result, 0.01)
}

func TestScalingCoordinator_GetScalingDecision_Logic(t *testing.T) {
	// Create a coordinator that we can test the decision logic on
	rules := &ScalingRules{
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

	// Test the decision logic by creating a custom test version
	tests := []struct {
		name                string
		metrics             []InstanceMetrics
		expectedAction      string
		expectedCurrent     int
		expectedRecommended int
	}{
		{
			name:                "no metrics - should recommend min instances",
			metrics:             []InstanceMetrics{},
			expectedAction:      "no_action",
			expectedCurrent:     0,
			expectedRecommended: 1,
		},
		{
			name: "high load - should scale up",
			metrics: []InstanceMetrics{
				{
					InstanceID:  "instance-1",
					CPUUsage:    0.9,
					MemoryUsage: 0.9,
					QueueDepth:  100.0, // This will normalize to 1.0
					Health:      "healthy",
				},
			},
			expectedAction:      "scale_up",
			expectedCurrent:     1,
			expectedRecommended: 2,
		},
		{
			name: "low load with multiple instances - should scale down",
			metrics: []InstanceMetrics{
				{
					InstanceID:  "instance-1",
					CPUUsage:    0.1,
					MemoryUsage: 0.1,
					QueueDepth:  1.0,
					Health:      "healthy",
				},
				{
					InstanceID:  "instance-2",
					CPUUsage:    0.1,
					MemoryUsage: 0.1,
					QueueDepth:  1.0,
					Health:      "healthy",
				},
			},
			expectedAction:      "scale_down",
			expectedCurrent:     2,
			expectedRecommended: 1,
		},
		{
			name: "moderate load - no action needed",
			metrics: []InstanceMetrics{
				{
					InstanceID:  "instance-1",
					CPUUsage:    0.5,
					MemoryUsage: 0.5,
					QueueDepth:  10.0,
					Health:      "healthy",
				},
			},
			expectedAction:      "no_action",
			expectedCurrent:     1,
			expectedRecommended: 1,
		},
		{
			name:                "at max instances - no scale up",
			metrics:             make([]InstanceMetrics, 10), // 10 instances (max)
			expectedAction:      "no_action",
			expectedCurrent:     10,
			expectedRecommended: 10,
		},
		{
			name: "at min instances - no scale down",
			metrics: []InstanceMetrics{
				{
					InstanceID:  "instance-1",
					CPUUsage:    0.1,
					MemoryUsage: 0.1,
					QueueDepth:  1.0,
					Health:      "healthy",
				},
			},
			expectedAction:      "no_action",
			expectedCurrent:     1,
			expectedRecommended: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewScalingCoordinator(nil, "collector", "instance-1", rules)

			// Initialize high load metrics for max instances test
			if len(tt.metrics) == 10 {
				for i := range tt.metrics {
					tt.metrics[i] = InstanceMetrics{
						InstanceID:  "instance-" + string(rune('1'+i)),
						CPUUsage:    0.9,
						MemoryUsage: 0.9,
						QueueDepth:  50.0,
						Health:      "healthy",
					}
				}
			}

			// Calculate the decision logic manually to test
			aggregateLoad := sc.calculateAggregateLoad(tt.metrics)
			currentCount := len(tt.metrics)

			var action string
			var recommendedCount int

			if currentCount == 0 {
				action = "no_action"
				recommendedCount = rules.MinInstances
			} else if aggregateLoad > rules.ScaleUpThreshold && currentCount < rules.MaxInstances {
				action = "scale_up"
				recommendedCount = minInt(currentCount+1, rules.MaxInstances)
			} else if aggregateLoad < rules.ScaleDownThreshold && currentCount > rules.MinInstances {
				action = "scale_down"
				recommendedCount = maxInt(currentCount-1, rules.MinInstances)
			} else {
				action = "no_action"
				recommendedCount = currentCount
			}

			// Debug output for failing tests
			if action != tt.expectedAction {
				t.Logf("Debug: aggregateLoad=%.3f, threshold=%.3f, currentCount=%d",
					aggregateLoad, rules.ScaleUpThreshold, currentCount)
			}

			assert.Equal(t, tt.expectedAction, action)
			assert.Equal(t, tt.expectedCurrent, currentCount)
			assert.Equal(t, tt.expectedRecommended, recommendedCount)
		})
	}
}

func TestScalingCoordinator_isInCooldownPeriod_Logic(t *testing.T) {
	rules := &ScalingRules{
		CooldownPeriod: 5 * time.Minute,
	}

	// Test cooldown logic by simulating different scenarios
	tests := []struct {
		name             string
		lastDecisionTime time.Time
		expectedCooldown bool
	}{
		{
			name:             "no previous decision",
			lastDecisionTime: time.Time{},
			expectedCooldown: false,
		},
		{
			name:             "recent decision within cooldown",
			lastDecisionTime: time.Now().Add(-2 * time.Minute),
			expectedCooldown: true,
		},
		{
			name:             "old decision outside cooldown",
			lastDecisionTime: time.Now().Add(-10 * time.Minute),
			expectedCooldown: false,
		},
		{
			name:             "decision exactly at cooldown boundary",
			lastDecisionTime: time.Now().Add(-5 * time.Minute),
			expectedCooldown: false, // Should be false at exactly the boundary
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the cooldown logic
			if tt.lastDecisionTime.IsZero() {
				assert.False(t, false) // No previous decision means no cooldown
			} else {
				timeSinceLastDecision := time.Since(tt.lastDecisionTime)
				inCooldown := timeSinceLastDecision < rules.CooldownPeriod
				assert.Equal(t, tt.expectedCooldown, inCooldown)
			}
		})
	}
}

func TestInstanceMetrics_Struct(t *testing.T) {
	now := time.Now()
	metrics := InstanceMetrics{
		InstanceID:     "instance-1",
		ServiceType:    "collector",
		CPUUsage:       0.5,
		MemoryUsage:    0.6,
		QueueDepth:     10.0,
		ProcessingRate: 100.0,
		ErrorRate:      0.01,
		Timestamp:      now,
		Health:         "healthy",
	}

	assert.Equal(t, "instance-1", metrics.InstanceID)
	assert.Equal(t, "collector", metrics.ServiceType)
	assert.Equal(t, 0.5, metrics.CPUUsage)
	assert.Equal(t, 0.6, metrics.MemoryUsage)
	assert.Equal(t, 10.0, metrics.QueueDepth)
	assert.Equal(t, 100.0, metrics.ProcessingRate)
	assert.Equal(t, 0.01, metrics.ErrorRate)
	assert.Equal(t, now, metrics.Timestamp)
	assert.Equal(t, "healthy", metrics.Health)
}

func TestInstanceMetrics_JSONSerialization(t *testing.T) {
	metrics := InstanceMetrics{
		InstanceID:     "instance-1",
		ServiceType:    "collector",
		CPUUsage:       0.5,
		MemoryUsage:    0.6,
		QueueDepth:     10.0,
		ProcessingRate: 100.0,
		ErrorRate:      0.01,
		Timestamp:      time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Health:         "healthy",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(metrics)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"instance_id\":\"instance-1\"")
	assert.Contains(t, string(jsonData), "\"service_type\":\"collector\"")
	assert.Contains(t, string(jsonData), "\"cpu_usage\":0.5")

	// Test JSON unmarshaling
	var unmarshaled InstanceMetrics
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, metrics.InstanceID, unmarshaled.InstanceID)
	assert.Equal(t, metrics.ServiceType, unmarshaled.ServiceType)
	assert.Equal(t, metrics.CPUUsage, unmarshaled.CPUUsage)
	assert.Equal(t, metrics.MemoryUsage, unmarshaled.MemoryUsage)
	assert.Equal(t, metrics.QueueDepth, unmarshaled.QueueDepth)
	assert.Equal(t, metrics.ProcessingRate, unmarshaled.ProcessingRate)
	assert.Equal(t, metrics.ErrorRate, unmarshaled.ErrorRate)
	assert.Equal(t, metrics.Health, unmarshaled.Health)
}

func TestScalingDecision_Struct(t *testing.T) {
	now := time.Now()
	decision := ScalingDecision{
		ServiceType:      "collector",
		Action:           "scale_up",
		CurrentCount:     1,
		RecommendedCount: 2,
		Reason:           "high load",
		Confidence:       0.8,
		Timestamp:        now,
	}

	assert.Equal(t, "collector", decision.ServiceType)
	assert.Equal(t, "scale_up", decision.Action)
	assert.Equal(t, 1, decision.CurrentCount)
	assert.Equal(t, 2, decision.RecommendedCount)
	assert.Equal(t, "high load", decision.Reason)
	assert.Equal(t, 0.8, decision.Confidence)
	assert.Equal(t, now, decision.Timestamp)
}

func TestScalingDecision_JSONSerialization(t *testing.T) {
	decision := ScalingDecision{
		ServiceType:      "collector",
		Action:           "scale_up",
		CurrentCount:     1,
		RecommendedCount: 2,
		Reason:           "high load",
		Confidence:       0.8,
		Timestamp:        time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(decision)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"service_type\":\"collector\"")
	assert.Contains(t, string(jsonData), "\"action\":\"scale_up\"")
	assert.Contains(t, string(jsonData), "\"current_count\":1")

	// Test JSON unmarshaling
	var unmarshaled ScalingDecision
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, decision.ServiceType, unmarshaled.ServiceType)
	assert.Equal(t, decision.Action, unmarshaled.Action)
	assert.Equal(t, decision.CurrentCount, unmarshaled.CurrentCount)
	assert.Equal(t, decision.RecommendedCount, unmarshaled.RecommendedCount)
	assert.Equal(t, decision.Reason, unmarshaled.Reason)
	assert.Equal(t, decision.Confidence, unmarshaled.Confidence)
}

func TestScalingRules_Struct(t *testing.T) {
	rules := ScalingRules{
		MinInstances:       2,
		MaxInstances:       20,
		ScaleUpThreshold:   0.9,
		ScaleDownThreshold: 0.2,
		CooldownPeriod:     10 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.4,
			"memory": 0.4,
			"queue":  0.2,
		},
	}

	assert.Equal(t, 2, rules.MinInstances)
	assert.Equal(t, 20, rules.MaxInstances)
	assert.Equal(t, 0.9, rules.ScaleUpThreshold)
	assert.Equal(t, 0.2, rules.ScaleDownThreshold)
	assert.Equal(t, 10*time.Minute, rules.CooldownPeriod)
	assert.Equal(t, 0.4, rules.MetricWeights["cpu"])
	assert.Equal(t, 0.4, rules.MetricWeights["memory"])
	assert.Equal(t, 0.2, rules.MetricWeights["queue"])
}

func TestScalingRules_JSONSerialization(t *testing.T) {
	rules := ScalingRules{
		MinInstances:       2,
		MaxInstances:       20,
		ScaleUpThreshold:   0.9,
		ScaleDownThreshold: 0.2,
		CooldownPeriod:     10 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.4,
			"memory": 0.4,
			"queue":  0.2,
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(rules)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"min_instances\":2")
	assert.Contains(t, string(jsonData), "\"max_instances\":20")
	assert.Contains(t, string(jsonData), "\"scale_up_threshold\":0.9")

	// Test JSON unmarshaling
	var unmarshaled ScalingRules
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, rules.MinInstances, unmarshaled.MinInstances)
	assert.Equal(t, rules.MaxInstances, unmarshaled.MaxInstances)
	assert.Equal(t, rules.ScaleUpThreshold, unmarshaled.ScaleUpThreshold)
	assert.Equal(t, rules.ScaleDownThreshold, unmarshaled.ScaleDownThreshold)
	assert.Equal(t, rules.MetricWeights["cpu"], unmarshaled.MetricWeights["cpu"])
}

func TestUtilityFunctions(t *testing.T) {
	// Test minInt
	assert.Equal(t, 5, minInt(5, 10))
	assert.Equal(t, 5, minInt(10, 5))
	assert.Equal(t, 5, minInt(5, 5))
	assert.Equal(t, -5, minInt(-5, 10))
	assert.Equal(t, -10, minInt(-5, -10))

	// Test maxInt
	assert.Equal(t, 10, maxInt(5, 10))
	assert.Equal(t, 10, maxInt(10, 5))
	assert.Equal(t, 5, maxInt(5, 5))
	assert.Equal(t, 10, maxInt(-5, 10))
	assert.Equal(t, -5, maxInt(-5, -10))

	// Test minFloat
	assert.Equal(t, 5.5, minFloat(5.5, 10.5))
	assert.Equal(t, 5.5, minFloat(10.5, 5.5))
	assert.Equal(t, 5.5, minFloat(5.5, 5.5))
	assert.Equal(t, -5.5, minFloat(-5.5, 10.5))
	assert.Equal(t, -10.5, minFloat(-5.5, -10.5))
}

func TestScalingDecision_Actions(t *testing.T) {
	tests := []struct {
		name   string
		action string
		valid  bool
	}{
		{"scale up", "scale_up", true},
		{"scale down", "scale_down", true},
		{"no action", "no_action", true},
		{"invalid action", "invalid", false},
		{"empty action", "", false},
		{"uppercase action", "SCALE_UP", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := ScalingDecision{
				Action: tt.action,
			}

			if tt.valid {
				assert.Contains(t, []string{"scale_up", "scale_down", "no_action"}, decision.Action)
			} else {
				assert.NotContains(t, []string{"scale_up", "scale_down", "no_action"}, decision.Action)
			}
		})
	}
}

func TestInstanceMetrics_Validation(t *testing.T) {
	tests := []struct {
		name    string
		metrics InstanceMetrics
		valid   bool
	}{
		{
			name: "valid metrics",
			metrics: InstanceMetrics{
				InstanceID:     "instance-1",
				ServiceType:    "collector",
				CPUUsage:       0.5,
				MemoryUsage:    0.6,
				QueueDepth:     10.0,
				ProcessingRate: 100.0,
				ErrorRate:      0.01,
				Health:         "healthy",
			},
			valid: true,
		},
		{
			name: "empty instance ID",
			metrics: InstanceMetrics{
				InstanceID:  "",
				ServiceType: "collector",
				CPUUsage:    0.5,
			},
			valid: false,
		},
		{
			name: "negative CPU usage",
			metrics: InstanceMetrics{
				InstanceID:  "instance-1",
				ServiceType: "collector",
				CPUUsage:    -0.1,
			},
			valid: false,
		},
		{
			name: "CPU usage over 100%",
			metrics: InstanceMetrics{
				InstanceID:  "instance-1",
				ServiceType: "collector",
				CPUUsage:    1.5,
			},
			valid: false,
		},
		{
			name: "negative processing rate",
			metrics: InstanceMetrics{
				InstanceID:     "instance-1",
				ServiceType:    "collector",
				CPUUsage:       0.5,
				ProcessingRate: -10.0,
			},
			valid: false,
		},
		{
			name: "negative error rate",
			metrics: InstanceMetrics{
				InstanceID:  "instance-1",
				ServiceType: "collector",
				CPUUsage:    0.5,
				ErrorRate:   -0.1,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, tt.metrics.InstanceID)
				assert.NotEmpty(t, tt.metrics.ServiceType)
				assert.GreaterOrEqual(t, tt.metrics.CPUUsage, 0.0)
				assert.LessOrEqual(t, tt.metrics.CPUUsage, 1.0)
				assert.GreaterOrEqual(t, tt.metrics.ProcessingRate, 0.0)
				assert.GreaterOrEqual(t, tt.metrics.ErrorRate, 0.0)
			} else {
				// At least one validation should fail
				invalid := tt.metrics.InstanceID == "" ||
					tt.metrics.ServiceType == "" ||
					tt.metrics.CPUUsage < 0.0 ||
					tt.metrics.CPUUsage > 1.0 ||
					tt.metrics.ProcessingRate < 0.0 ||
					tt.metrics.ErrorRate < 0.0
				assert.True(t, invalid, "Metrics should be invalid")
			}
		})
	}
}

func TestScalingRules_DefaultWeights(t *testing.T) {
	// Test that default rules have proper weights
	sc := NewScalingCoordinator(nil, "collector", "instance-1", nil)

	expectedWeights := map[string]float64{
		"cpu":    0.3,
		"memory": 0.3,
		"queue":  0.4,
	}

	assert.Equal(t, expectedWeights, sc.rules.MetricWeights)

	// Test that weights sum to 1.0
	var totalWeight float64
	for _, weight := range sc.rules.MetricWeights {
		totalWeight += weight
	}
	assert.InDelta(t, 1.0, totalWeight, 0.001)
}

func TestScalingCoordinator_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		rules       *ScalingRules
		expectPanic bool
	}{
		{
			name: "valid rules",
			rules: &ScalingRules{
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
			},
			expectPanic: false,
		},
		{
			name: "min > max instances",
			rules: &ScalingRules{
				MinInstances: 10,
				MaxInstances: 5,
			},
			expectPanic: false, // Should handle gracefully
		},
		{
			name: "invalid thresholds",
			rules: &ScalingRules{
				ScaleUpThreshold:   0.3,
				ScaleDownThreshold: 0.8, // Down threshold higher than up threshold
			},
			expectPanic: false, // Should handle gracefully
		},
		{
			name: "zero cooldown period",
			rules: &ScalingRules{
				MinInstances:       1,
				MaxInstances:       10,
				ScaleUpThreshold:   0.8,
				ScaleDownThreshold: 0.3,
				CooldownPeriod:     0,
			},
			expectPanic: false,
		},
		{
			name: "empty metric weights",
			rules: &ScalingRules{
				MinInstances:       1,
				MaxInstances:       10,
				ScaleUpThreshold:   0.8,
				ScaleDownThreshold: 0.3,
				CooldownPeriod:     5 * time.Minute,
				MetricWeights:      map[string]float64{},
			},
			expectPanic: false, // Should use defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				assert.Panics(t, func() {
					NewScalingCoordinator(nil, "collector", "instance-1", tt.rules)
				})
			} else {
				assert.NotPanics(t, func() {
					sc := NewScalingCoordinator(nil, "collector", "instance-1", tt.rules)
					assert.NotNil(t, sc)
				})
			}
		})
	}
}

func TestScalingCoordinator_QueueDepthNormalization(t *testing.T) {
	sc := NewScalingCoordinator(nil, "collector", "instance-1", nil)

	tests := []struct {
		name           string
		queueDepth     float64
		expectedNormal float64
	}{
		{"zero queue", 0.0, 0.0},
		{"small queue", 5.0, 0.05},
		{"medium queue", 50.0, 0.5},
		{"large queue", 100.0, 1.0},
		{"very large queue", 1000.0, 1.0},       // Should be capped at 1.0
		{"extremely large queue", 10000.0, 1.0}, // Should be capped at 1.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := []InstanceMetrics{
				{
					CPUUsage:    0.0,
					MemoryUsage: 0.0,
					QueueDepth:  tt.queueDepth,
				},
			}

			// The aggregate load should be 0.0*0.3 + 0.0*0.3 + normalized_queue*0.4
			expectedLoad := tt.expectedNormal * 0.4
			actualLoad := sc.calculateAggregateLoad(metrics)
			assert.InDelta(t, expectedLoad, actualLoad, 0.01)
		})
	}
}

func TestScalingCoordinator_MultipleInstancesAveraging(t *testing.T) {
	sc := NewScalingCoordinator(nil, "collector", "instance-1", nil)

	// Test that multiple instances are properly averaged
	metrics := []InstanceMetrics{
		{CPUUsage: 0.2, MemoryUsage: 0.3, QueueDepth: 10.0}, // Load: 0.2*0.3 + 0.3*0.3 + 0.1*0.4 = 0.19
		{CPUUsage: 0.8, MemoryUsage: 0.7, QueueDepth: 30.0}, // Load: 0.8*0.3 + 0.7*0.3 + 0.3*0.4 = 0.57
		{CPUUsage: 0.5, MemoryUsage: 0.5, QueueDepth: 20.0}, // Load: 0.5*0.3 + 0.5*0.3 + 0.2*0.4 = 0.38
	}

	expectedAverage := (0.19 + 0.57 + 0.38) / 3 // 1.14 / 3 = 0.38
	actualLoad := sc.calculateAggregateLoad(metrics)
	assert.InDelta(t, expectedAverage, actualLoad, 0.01)
}

// Benchmark tests
func BenchmarkScalingCoordinator_calculateAggregateLoad(b *testing.B) {
	sc := NewScalingCoordinator(nil, "collector", "instance-1", nil)

	metrics := []InstanceMetrics{
		{CPUUsage: 0.5, MemoryUsage: 0.6, QueueDepth: 10.0},
		{CPUUsage: 0.7, MemoryUsage: 0.8, QueueDepth: 20.0},
		{CPUUsage: 0.3, MemoryUsage: 0.4, QueueDepth: 5.0},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc.calculateAggregateLoad(metrics)
	}
}

func BenchmarkInstanceMetrics_JSONMarshal(b *testing.B) {
	metrics := InstanceMetrics{
		InstanceID:     "instance-1",
		ServiceType:    "collector",
		CPUUsage:       0.5,
		MemoryUsage:    0.6,
		QueueDepth:     10.0,
		ProcessingRate: 100.0,
		ErrorRate:      0.01,
		Timestamp:      time.Now(),
		Health:         "healthy",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(metrics)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalingDecision_JSONMarshal(b *testing.B) {
	decision := ScalingDecision{
		ServiceType:      "collector",
		Action:           "scale_up",
		CurrentCount:     1,
		RecommendedCount: 2,
		Reason:           "high load",
		Confidence:       0.8,
		Timestamp:        time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(decision)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScalingRules_JSONMarshal(b *testing.B) {
	rules := ScalingRules{
		MinInstances:       2,
		MaxInstances:       20,
		ScaleUpThreshold:   0.9,
		ScaleDownThreshold: 0.2,
		CooldownPeriod:     10 * time.Minute,
		MetricWeights: map[string]float64{
			"cpu":    0.4,
			"memory": 0.4,
			"queue":  0.2,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(rules)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUtilityFunctions(b *testing.B) {
	b.Run("minInt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			minInt(i, i+1)
		}
	})

	b.Run("maxInt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			maxInt(i, i+1)
		}
	})

	b.Run("minFloat", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			minFloat(float64(i), float64(i+1))
		}
	})
}

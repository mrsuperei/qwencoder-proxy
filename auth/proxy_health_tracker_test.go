// Package auth provides authentication and token management functionality.
package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/sunbankio/qwencoder-proxy/logging"
)

// TestNewProxyHealthTracker verifies tracker creation with default and custom values.
func TestNewProxyHealthTracker(t *testing.T) {
	logger := logging.NewLogger()

	// Test with default values
	tracker := NewProxyHealthTracker(logger, 0, 0)
	if tracker.failureThreshold != 5 {
		t.Errorf("Expected default failureThreshold to be 5, got %d", tracker.failureThreshold)
	}
	if tracker.healthCheckInterval != 5*time.Minute {
		t.Errorf("Expected default healthCheckInterval to be 5 minutes, got %v", tracker.healthCheckInterval)
	}

	// Test with custom values
	tracker = NewProxyHealthTracker(logger, 10, 10*time.Minute)
	if tracker.failureThreshold != 10 {
		t.Errorf("Expected failureThreshold to be 10, got %d", tracker.failureThreshold)
	}
	if tracker.healthCheckInterval != 10*time.Minute {
		t.Errorf("Expected healthCheckInterval to be 10 minutes, got %v", tracker.healthCheckInterval)
	}
}

// TestUpdateHealthSuccess verifies health score increases on success.
func TestUpdateHealthSuccess(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// First success - should create record and return 1.0
	score := tracker.UpdateHealth(tokenID, true, nil)
	if score != 1.0 {
		t.Errorf("Expected score to be 1.0 on first success, got %f", score)
	}

	// Verify health status
	health := tracker.GetHealthStatus(tokenID)
	if health == nil {
		t.Fatal("Expected health status to not be nil")
	}
	if !health.IsHealthy {
		t.Error("Expected IsHealthy to be true")
	}
	if health.ConsecutiveFailures != 0 {
		t.Errorf("Expected ConsecutiveFailures to be 0, got %d", health.ConsecutiveFailures)
	}
	if health.LastError != "" {
		t.Errorf("Expected LastError to be empty, got %s", health.LastError)
	}
}

// TestUpdateHealthFailure verifies health score decreases on failure.
func TestUpdateHealthFailure(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"
	testError := errors.New("connection failed")

	// First failure
	score := tracker.UpdateHealth(tokenID, false, testError)
	if score != 0.8 {
		t.Errorf("Expected score to be 0.8 on first failure, got %f", score)
	}

	health := tracker.GetHealthStatus(tokenID)
	if health == nil {
		t.Fatal("Expected health status to not be nil")
	}
	if health.IsHealthy {
		t.Error("Expected IsHealthy to be false")
	}
	if health.ConsecutiveFailures != 1 {
		t.Errorf("Expected ConsecutiveFailures to be 1, got %d", health.ConsecutiveFailures)
	}
	if health.LastError != testError.Error() {
		t.Errorf("Expected LastError to be %s, got %s", testError.Error(), health.LastError)
	}

	// Second failure
	score = tracker.UpdateHealth(tokenID, false, testError)
	if score != 0.6 {
		t.Errorf("Expected score to be 0.6 on second failure, got %f", score)
	}

	health = tracker.GetHealthStatus(tokenID)
	if health.ConsecutiveFailures != 2 {
		t.Errorf("Expected ConsecutiveFailures to be 2, got %d", health.ConsecutiveFailures)
	}
}

// TestUpdateHealthRecovery verifies health recovers after failure.
func TestUpdateHealthRecovery(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"
	testError := errors.New("connection failed")

	// Fail twice
	tracker.UpdateHealth(tokenID, false, testError)
	tracker.UpdateHealth(tokenID, false, testError)

	// Verify unhealthy
	health := tracker.GetHealthStatus(tokenID)
	if health.IsHealthy {
		t.Error("Expected IsHealthy to be false after failures")
	}
	if health.ConsecutiveFailures != 2 {
		t.Errorf("Expected ConsecutiveFailures to be 2, got %d", health.ConsecutiveFailures)
	}

	// Recover with success
	score := tracker.UpdateHealth(tokenID, true, nil)
	if score != 1.0 {
		t.Errorf("Expected score to be 1.0 on recovery, got %f", score)
	}

	// Verify recovered
	health = tracker.GetHealthStatus(tokenID)
	if !health.IsHealthy {
		t.Error("Expected IsHealthy to be true after recovery")
	}
	if health.ConsecutiveFailures != 0 {
		t.Errorf("Expected ConsecutiveFailures to be 0 after recovery, got %d", health.ConsecutiveFailures)
	}
	if health.LastError != "" {
		t.Errorf("Expected LastError to be empty after recovery, got %s", health.LastError)
	}
}

// TestUpdateHealthFailureThreshold verifies consecutive failures exceed threshold.
func TestUpdateHealthFailureThreshold(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 3, 5*time.Minute) // Threshold of 3

	tokenID := "test-token-id"
	testError := errors.New("connection failed")

	// Fail 3 times (at threshold)
	for i := 0; i < 3; i++ {
		tracker.UpdateHealth(tokenID, false, testError)
	}

	health := tracker.GetHealthStatus(tokenID)
	if health.ConsecutiveFailures != 3 {
		t.Errorf("Expected ConsecutiveFailures to be 3, got %d", health.ConsecutiveFailures)
	}

	// Fail 4 times (exceeds threshold)
	tracker.UpdateHealth(tokenID, false, testError)

	health = tracker.GetHealthStatus(tokenID)
	if health.ConsecutiveFailures != 4 {
		t.Errorf("Expected ConsecutiveFailures to be 4, got %d", health.ConsecutiveFailures)
	}
	// Should still be unhealthy
	if health.IsHealthy {
		t.Error("Expected IsHealthy to be false when exceeding threshold")
	}
}

// TestGetHealthScoreDefault verifies default score for untracked tokens.
func TestGetHealthScoreDefault(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "untracked-token"

	// Untracked token should return 1.0 (default healthy)
	score := tracker.GetHealthScore(tokenID)
	if score != 1.0 {
		t.Errorf("Expected default score to be 1.0, got %f", score)
	}
}

// TestGetHealthScoreTracked verifies score for tracked tokens.
func TestGetHealthScoreTracked(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// Mark as healthy
	tracker.UpdateHealth(tokenID, true, nil)
	score := tracker.GetHealthScore(tokenID)
	if score != 1.0 {
		t.Errorf("Expected score to be 1.0 for healthy token, got %f", score)
	}

	// Mark as unhealthy
	tracker.UpdateHealth(tokenID, false, errors.New("error"))
	score = tracker.GetHealthScore(tokenID)
	if score != 0.8 {
		t.Errorf("Expected score to be 0.8 for unhealthy token, got %f", score)
	}
}

// TestGetHealthStatusNilForUntracked verifies nil for untracked tokens.
func TestGetHealthStatusNilForUntracked(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "untracked-token"

	health := tracker.GetHealthStatus(tokenID)
	if health != nil {
		t.Error("Expected health status to be nil for untracked token")
	}
}

// TestGetHealthStatusReturnsCopy verifies returned health is a copy.
func TestGetHealthStatusReturnsCopy(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	tracker.UpdateHealth(tokenID, true, nil)
	health := tracker.GetHealthStatus(tokenID)

	if health == nil {
		t.Fatal("Expected health status to not be nil")
	}

	// Modify the returned copy
	health.IsHealthy = false
	health.ConsecutiveFailures = 10

	// Verify original was not modified
	originalHealth := tracker.GetHealthStatus(tokenID)
	if originalHealth.IsHealthy {
		t.Error("Original health should not be affected by copy modification")
	}
	if originalHealth.ConsecutiveFailures == 10 {
		t.Error("Original consecutive failures should not be affected by copy modification")
	}
}

// TestRecordLatency verifies latency recording and averaging.
func TestRecordLatency(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// Record initial latency
	tracker.RecordLatency(tokenID, 100)

	health := tracker.GetHealthStatus(tokenID)
	if health == nil {
		t.Fatal("Expected health status to not be nil")
	}
	if health.AverageLatencyMs != 100 {
		t.Errorf("Expected AverageLatencyMs to be 100, got %d", health.AverageLatencyMs)
	}

	// Record second latency (average should be (100 + 200) / 2 = 150)
	tracker.RecordLatency(tokenID, 200)

	health = tracker.GetHealthStatus(tokenID)
	if health.AverageLatencyMs != 150 {
		t.Errorf("Expected AverageLatencyMs to be 150, got %d", health.AverageLatencyMs)
	}

	// Record third latency (average should be (150 + 300) / 2 = 225)
	tracker.RecordLatency(tokenID, 300)

	health = tracker.GetHealthStatus(tokenID)
	if health.AverageLatencyMs != 225 {
		t.Errorf("Expected AverageLatencyMs to be 225, got %d", health.AverageLatencyMs)
	}
}

// TestIsHealthyDefault verifies default healthy for untracked tokens.
func TestIsHealthyDefault(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "untracked-token"

	// Untracked token should be considered healthy
	if !tracker.IsHealthy(tokenID) {
		t.Error("Expected untracked token to be healthy")
	}
}

// TestIsHealthyTracked verifies healthy status for tracked tokens.
func TestIsHealthyTracked(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// Initially healthy
	if !tracker.IsHealthy(tokenID) {
		t.Error("Expected token to be healthy initially")
	}

	// Mark as unhealthy
	tracker.UpdateHealth(tokenID, false, errors.New("error"))
	if tracker.IsHealthy(tokenID) {
		t.Error("Expected token to be unhealthy after failure")
	}

	// Mark as healthy again
	tracker.UpdateHealth(tokenID, true, nil)
	if !tracker.IsHealthy(tokenID) {
		t.Error("Expected token to be healthy after recovery")
	}
}

// TestClearHealth verifies clearing health for a single token.
func TestClearHealth(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// Record some health data
	tracker.UpdateHealth(tokenID, false, errors.New("error"))
	tracker.RecordLatency(tokenID, 100)

	// Verify data exists
	if tracker.GetHealthStatus(tokenID) == nil {
		t.Error("Expected health status to exist")
	}

	// Clear health
	tracker.ClearHealth(tokenID)

	// Verify data is cleared
	if tracker.GetHealthStatus(tokenID) != nil {
		t.Error("Expected health status to be nil after clear")
	}
}

// TestClearAllHealth verifies clearing all health records.
func TestClearAllHealth(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	// Add multiple tokens
	for i := 0; i < 5; i++ {
		tokenID := "test-token-" + string(rune('0'+i))
		tracker.UpdateHealth(tokenID, true, nil)
	}

	// Verify tokens are tracked
	if tracker.GetTrackedTokenCount() != 5 {
		t.Errorf("Expected 5 tracked tokens, got %d", tracker.GetTrackedTokenCount())
	}

	// Clear all
	tracker.ClearAllHealth()

	// Verify all cleared
	if tracker.GetTrackedTokenCount() != 0 {
		t.Errorf("Expected 0 tracked tokens after clear, got %d", tracker.GetTrackedTokenCount())
	}
}

// TestGetTrackedTokenCount verifies tracking count.
func TestGetTrackedTokenCount(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	// Initially empty
	if tracker.GetTrackedTokenCount() != 0 {
		t.Errorf("Expected 0 tracked tokens initially, got %d", tracker.GetTrackedTokenCount())
	}

	// Add tokens
	for i := 0; i < 3; i++ {
		tokenID := "test-token-" + string(rune('0'+i))
		tracker.UpdateHealth(tokenID, true, nil)
	}

	if tracker.GetTrackedTokenCount() != 3 {
		t.Errorf("Expected 3 tracked tokens, got %d", tracker.GetTrackedTokenCount())
	}
}

// TestGetUnhealthyTokenCount verifies unhealthy token count.
func TestGetUnhealthyTokenCount(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	// Initially 0
	if tracker.GetUnhealthyTokenCount() != 0 {
		t.Errorf("Expected 0 unhealthy tokens initially, got %d", tracker.GetUnhealthyTokenCount())
	}

	// Add healthy tokens
	tracker.UpdateHealth("healthy-1", true, nil)
	tracker.UpdateHealth("healthy-2", true, nil)

	if tracker.GetUnhealthyTokenCount() != 0 {
		t.Errorf("Expected 0 unhealthy tokens, got %d", tracker.GetUnhealthyTokenCount())
	}

	// Add unhealthy tokens
	tracker.UpdateHealth("unhealthy-1", false, errors.New("error"))
	tracker.UpdateHealth("unhealthy-2", false, errors.New("error"))

	if tracker.GetUnhealthyTokenCount() != 2 {
		t.Errorf("Expected 2 unhealthy tokens, got %d", tracker.GetUnhealthyTokenCount())
	}
}

// TestListUnhealthyTokens verifies listing unhealthy tokens.
func TestListUnhealthyTokens(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	// Add tokens
	tracker.UpdateHealth("healthy-1", true, nil)
	tracker.UpdateHealth("unhealthy-1", false, errors.New("error"))
	tracker.UpdateHealth("unhealthy-2", false, errors.New("error"))
	tracker.UpdateHealth("healthy-2", true, nil)

	// List unhealthy tokens
	unhealthy := tracker.ListUnhealthyTokens()

	if len(unhealthy) != 2 {
		t.Errorf("Expected 2 unhealthy tokens, got %d", len(unhealthy))
	}

	// Verify the list contains the expected tokens
	hasUnhealthy1 := false
	hasUnhealthy2 := false
	for _, tokenID := range unhealthy {
		if tokenID == "unhealthy-1" {
			hasUnhealthy1 = true
		}
		if tokenID == "unhealthy-2" {
			hasUnhealthy2 = true
		}
	}

	if !hasUnhealthy1 {
		t.Error("Expected unhealthy-1 in the list")
	}
	if !hasUnhealthy2 {
		t.Error("Expected unhealthy-2 in the list")
	}
}

// TestHealthScoreClamping verifies health score is clamped between 0.0 and 1.0.
func TestHealthScoreClamping(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// Multiple failures should not go below 0.0
	for i := 0; i < 10; i++ {
		tracker.UpdateHealth(tokenID, false, errors.New("error"))
	}

	score := tracker.GetHealthScore(tokenID)
	if score < 0.0 {
		t.Errorf("Expected score to be >= 0.0, got %f", score)
	}

	// Multiple successes should not go above 1.0
	tracker.UpdateHealth(tokenID, true, nil)
	tracker.UpdateHealth(tokenID, true, nil)
	tracker.UpdateHealth(tokenID, true, nil)

	score = tracker.GetHealthScore(tokenID)
	if score > 1.0 {
		t.Errorf("Expected score to be <= 1.0, got %f", score)
	}
}

// TestConcurrentAccess verifies thread safety of health tracker.
func TestConcurrentAccess(t *testing.T) {
	logger := logging.NewLogger()
	tracker := NewProxyHealthTracker(logger, 5, 5*time.Minute)

	tokenID := "test-token-id"

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tracker.UpdateHealth(tokenID, j%2 == 0, nil)
				tracker.RecordLatency(tokenID, j*10)
				tracker.GetHealthScore(tokenID)
				tracker.IsHealthy(tokenID)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify tracker is still functional
	health := tracker.GetHealthStatus(tokenID)
	if health == nil {
		t.Error("Expected health status to not be nil after concurrent access")
	}
}

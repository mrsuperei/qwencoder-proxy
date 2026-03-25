package ratelimit

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cacheMockLogger is a mock implementation of logging.Logger for testing.
type cacheMockLogger struct{}

func (m *cacheMockLogger) InfoLog(msg string, args ...interface{})  {}
func (m *cacheMockLogger) DebugLog(msg string, args ...interface{}) {}
func (m *cacheMockLogger) WarnLog(msg string, args ...interface{})  {}
func (m *cacheMockLogger) ErrorLog(msg string, args ...interface{}) {}

func TestNewCacheInvalidator(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)

	require.NotNil(t, invalidator)
	assert.NotNil(t, invalidator.eventChan)
	assert.NotNil(t, invalidator.subscribers)

	// Cleanup
	invalidator.Close()
}

func TestCacheInvalidator_Publish(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)
	defer invalidator.Close()

	// Subscribe to events
	eventChan, unsubscribe := invalidator.Subscribe()
	defer unsubscribe()

	// Publish an event
	event := InvalidationEvent{
		Type:      InvalidationProvider,
		Target:    "test-provider",
		Timestamp: time.Now(),
		Source:    "test",
	}
	invalidator.Publish(event)

	// Wait for event
	select {
	case received := <-eventChan:
		assert.Equal(t, InvalidationProvider, received.Type)
		assert.Equal(t, "test-provider", received.Target)
		assert.Equal(t, "test", received.Source)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}
}

func TestCacheInvalidator_PublishInvalidation(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)
	defer invalidator.Close()

	// Subscribe to events
	eventChan, unsubscribe := invalidator.Subscribe()
	defer unsubscribe()

	// Publish invalidation using convenience method
	invalidator.PublishInvalidation(InvalidationToken, "test-token", "test")

	// Wait for event
	select {
	case received := <-eventChan:
		assert.Equal(t, InvalidationToken, received.Type)
		assert.Equal(t, "test-token", received.Target)
		assert.Equal(t, "test", received.Source)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}
}

func TestCacheInvalidator_MultipleSubscribers(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)
	defer invalidator.Close()

	// Subscribe multiple times
	eventChan1, unsubscribe1 := invalidator.Subscribe()
	defer unsubscribe1()

	eventChan2, unsubscribe2 := invalidator.Subscribe()
	defer unsubscribe2()

	eventChan3, unsubscribe3 := invalidator.Subscribe()
	defer unsubscribe3()

	// Publish an event
	invalidator.PublishInvalidation(InvalidationProvider, "test", "test")

	// Wait for all subscribers to receive the event
	var wg sync.WaitGroup
	wg.Add(3)

	receivedCount := 0
	mu := sync.Mutex{}

	// Subscriber 1
	go func() {
		defer wg.Done()
		select {
		case <-eventChan1:
			mu.Lock()
			receivedCount++
			mu.Unlock()
		case <-time.After(100 * time.Millisecond):
			t.Error("Subscriber 1 timeout")
		}
	}()

	// Subscriber 2
	go func() {
		defer wg.Done()
		select {
		case <-eventChan2:
			mu.Lock()
			receivedCount++
			mu.Unlock()
		case <-time.After(100 * time.Millisecond):
			t.Error("Subscriber 2 timeout")
		}
	}()

	// Subscriber 3
	go func() {
		defer wg.Done()
		select {
		case <-eventChan3:
			mu.Lock()
			receivedCount++
			mu.Unlock()
		case <-time.After(100 * time.Millisecond):
			t.Error("Subscriber 3 timeout")
		}
	}()

	wg.Wait()

	assert.Equal(t, 3, receivedCount)
}

func TestCacheInvalidator_GetStats(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)
	defer invalidator.Close()

	// Subscribe to ensure stats are accurate
	_, unsubscribe := invalidator.Subscribe()
	defer unsubscribe()

	// Publish some events
	for i := 0; i < 5; i++ {
		invalidator.PublishInvalidation(InvalidationProvider, "test", "test")
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Get stats
	stats := invalidator.GetStats()

	assert.Equal(t, int64(5), stats.EventsProcessed)
	assert.Equal(t, 1, stats.SubscriberCount)
	assert.Equal(t, 1, stats.ActiveSubscribers)
}

func TestCacheInvalidator_Close(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)

	// Subscribe to events
	eventChan, unsubscribe := invalidator.Subscribe()

	// Close invalidator
	invalidator.Close()

	// Verify channel is closed
	select {
	case _, ok := <-eventChan:
		assert.False(t, ok, "Channel should be closed")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Channel should be closed")
	}

	// Unsubscribe should not panic
	unsubscribe()
}

func TestCacheInvalidator_EventTimestamp(t *testing.T) {
	logger := &cacheMockLogger{}
	invalidator := NewCacheInvalidator(logger)
	defer invalidator.Close()

	// Subscribe to events
	eventChan, unsubscribe := invalidator.Subscribe()
	defer unsubscribe()

	// Publish event without timestamp
	invalidator.Publish(InvalidationEvent{
		Type:   InvalidationProvider,
		Target: "test",
		Source: "test",
	})

	// Wait for event
	select {
	case received := <-eventChan:
		// Timestamp should be set
		assert.False(t, received.Timestamp.IsZero())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}
}

func TestInvalidationType_String(t *testing.T) {
	tests := []struct {
		name      string
		invalType InvalidationType
		expected  string
	}{
		{"provider", InvalidationProvider, "provider"},
		{"token", InvalidationToken, "token"},
		{"all", InvalidationAll, "all"},
		{"config", InvalidationConfig, "config"},
		{"unknown", InvalidationType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.invalType.String())
		})
	}
}

package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunbankio/qwencoder-proxy/internal/logging"
)

// InvalidationType represents the type of cache invalidation.
type InvalidationType int

const (
	// InvalidationProvider invalidates cache for a specific provider.
	InvalidationProvider InvalidationType = iota
	// InvalidationToken invalidates cache for a specific token.
	InvalidationToken
	// InvalidationAll invalidates all cache entries.
	InvalidationAll
	// InvalidationConfig invalidates cache when configuration changes.
	InvalidationConfig
)

// String returns the string representation of InvalidationType.
func (it InvalidationType) String() string {
	switch it {
	case InvalidationProvider:
		return "provider"
	case InvalidationToken:
		return "token"
	case InvalidationAll:
		return "all"
	case InvalidationConfig:
		return "config"
	default:
		return "unknown"
	}
}

// InvalidationEvent represents a cache invalidation event.
type InvalidationEvent struct {
	Type      InvalidationType // Type of invalidation
	Target    string           // Target identifier (provider ID, token ID, etc.)
	Timestamp time.Time        // When the event occurred
	Source    string           // Source of the invalidation (e.g., "api", "database", "write")
}

// InvalidationHandler is a function that handles invalidation events.
type InvalidationHandler func(event InvalidationEvent)

// CacheInvalidator manages cache invalidation events and subscriptions.
type CacheInvalidator struct {
	// Event channel for broadcasting invalidation events
	eventChan chan InvalidationEvent

	// Subscriber management
	mu           sync.RWMutex
	subscribers  map[string]chan InvalidationEvent
	subscriberID atomic.Int64

	// Metrics
	eventsProcessed atomic.Int64
	eventsDropped   atomic.Int64

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Logger
	logger logging.Logger
}

// NewCacheInvalidator creates a new cache invalidator.
func NewCacheInvalidator(logger logging.Logger) *CacheInvalidator {
	ctx, cancel := context.WithCancel(context.Background())

	invalidator := &CacheInvalidator{
		eventChan:   make(chan InvalidationEvent, 100),
		subscribers: make(map[string]chan InvalidationEvent),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}

	// Start event broadcaster
	go invalidator.eventBroadcaster()

	logger.InfoLog("[CacheInvalidator] Initialized with event channel buffer: 100")

	return invalidator
}

// Publish publishes an invalidation event to all subscribers.
func (ci *CacheInvalidator) Publish(event InvalidationEvent) {
	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case ci.eventChan <- event:
		ci.logger.DebugLog("[CacheInvalidator] Published event: type=%s, target=%s, source=%s",
			event.Type, event.Target, event.Source)
	default:
		// Channel full - drop event
		ci.eventsDropped.Add(1)
		ci.logger.WarnLog("[CacheInvalidator] Event dropped (channel full): type=%s, target=%s",
			event.Type, event.Target)
	}
}

// PublishInvalidation is a convenience method to publish an invalidation event.
func (ci *CacheInvalidator) PublishInvalidation(invalidationType InvalidationType, target, source string) {
	ci.Publish(InvalidationEvent{
		Type:   invalidationType,
		Target: target,
		Source: source,
	})
}

// Subscribe subscribes to invalidation events.
// Returns a channel that will receive events and a function to unsubscribe.
func (ci *CacheInvalidator) Subscribe() (chan InvalidationEvent, func()) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	// Generate unique subscriber ID
	id := ci.subscriberID.Add(1)
	subID := string(rune('a'+(id%26))) + string(rune('0'+(id%10)))

	// Create buffered channel for subscriber
	eventChan := make(chan InvalidationEvent, 10)

	// Register subscriber
	ci.subscribers[subID] = eventChan

	ci.logger.DebugLog("[CacheInvalidator] New subscriber: id=%s, total=%d",
		subID, len(ci.subscribers))

	// Return unsubscribe function
	unsubscribe := func() {
		ci.Unsubscribe(subID)
	}

	return eventChan, unsubscribe
}

// Unsubscribe removes a subscriber.
func (ci *CacheInvalidator) Unsubscribe(subID string) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	if eventChan, exists := ci.subscribers[subID]; exists {
		close(eventChan)
		delete(ci.subscribers, subID)
		ci.logger.DebugLog("[CacheInvalidator] Unsubscribed: id=%s, remaining=%d",
			subID, len(ci.subscribers))
	}
}

// eventBroadcaster broadcasts events to all subscribers.
func (ci *CacheInvalidator) eventBroadcaster() {
	for {
		select {
		case <-ci.ctx.Done():
			ci.logger.DebugLog("[CacheInvalidator] Event broadcaster stopped")
			return
		case event := <-ci.eventChan:
			ci.eventsProcessed.Add(1)
			ci.broadcastEvent(event)
		}
	}
}

// broadcastEvent sends an event to all subscribers.
func (ci *CacheInvalidator) broadcastEvent(event InvalidationEvent) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	for subID, eventChan := range ci.subscribers {
		select {
		case eventChan <- event:
			// Event sent successfully
		default:
			// Subscriber channel full - skip this subscriber
			ci.logger.DebugLog("[CacheInvalidator] Subscriber channel full, skipping: id=%s", subID)
		}
	}
}

// GetStats returns the current statistics for the cache invalidator.
func (ci *CacheInvalidator) GetStats() CacheInvalidatorStats {
	return CacheInvalidatorStats{
		EventsProcessed: ci.eventsProcessed.Load(),
		EventsDropped:   ci.eventsDropped.Load(),
		SubscriberCount: int(ci.subscriberID.Load()),
		ActiveSubscribers: func() int {
			ci.mu.RLock()
			defer ci.mu.RUnlock()
			return len(ci.subscribers)
		}(),
	}
}

// Close closes the cache invalidator and cleans up resources.
func (ci *CacheInvalidator) Close() error {
	ci.cancel()

	// Close all subscriber channels
	ci.mu.Lock()
	defer ci.mu.Unlock()

	for subID, eventChan := range ci.subscribers {
		close(eventChan)
		delete(ci.subscribers, subID)
		ci.logger.DebugLog("[CacheInvalidator] Closed subscriber: id=%s", subID)
	}

	ci.logger.InfoLog("[CacheInvalidator] Closed")

	return nil
}

// CacheInvalidatorStats represents statistics for the cache invalidator.
type CacheInvalidatorStats struct {
	EventsProcessed   int64 `json:"events_processed"`
	EventsDropped     int64 `json:"events_dropped"`
	SubscriberCount   int   `json:"subscriber_count"`
	ActiveSubscribers int   `json:"active_subscribers"`
}

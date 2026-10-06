package storage

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	RedisChannelEvents        = "wedding:events"
	RedisChannelNotifications = "wedding:notifications"
	RedisKeyStatsCache        = "wedding:cache:stats"
)

// RedisClient wraps Redis functionality with seamless in-memory fallback
type RedisClient struct {
	client    *redis.Client
	ctx       context.Context
	available bool
	mu        sync.RWMutex
}

// NewRedisClient initializes connection to Redis if REDIS_ADDR or REDIS_URL is configured
func NewRedisClient() *RedisClient {
	ctx := context.Background()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = os.Getenv("REDIS_URL")
	}
	if addr == "" {
		// Default to localhost:6379 in container/dev environment if available
		addr = "localhost:6379"
	}

	password := os.Getenv("REDIS_PASSWORD")

	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           0,
		DialTimeout:  1500 * time.Millisecond,
		ReadTimeout:  1000 * time.Millisecond,
		WriteTimeout: 1000 * time.Millisecond,
		PoolSize:     64,
		MinIdleConns: 10,
	})

	// Test connection
	pingCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	rc := &RedisClient{
		client: rdb,
		ctx:    ctx,
	}

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		log.Printf("[Redis] Redis not reachable at %s (%v) — operating in fast in-memory fallback mode", addr, err)
		rc.available = false
	} else {
		log.Printf("[Redis] Connected successfully to Redis at %s (Pub/Sub & Distributed Cache Active)", addr)
		rc.available = true
	}

	return rc
}

// IsAvailable returns whether Redis is connected
func (r *RedisClient) IsAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.available
}

// PublishEvent broadcasts an event across Blue/Green instances
func (r *RedisClient) PublishEvent(event string, payload interface{}) error {
	if !r.IsAvailable() {
		return nil
	}

	data, err := json.Marshal(map[string]interface{}{
		"event":     event,
		"payload":   payload,
		"timestamp": time.Now().Unix(),
	})
	if err != nil {
		return err
	}

	return r.client.Publish(r.ctx, RedisChannelEvents, data).Err()
}

// SubscribeEvents listens for events published across instances
func (r *RedisClient) SubscribeEvents(handler func(event string, payload []byte)) {
	if !r.IsAvailable() {
		return
	}

	pubsub := r.client.Subscribe(r.ctx, RedisChannelEvents)
	ch := pubsub.Channel()

	go func() {
		defer pubsub.Close()
		for msg := range ch {
			var wrapper struct {
				Event   string          `json:"event"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &wrapper); err == nil {
				handler(wrapper.Event, wrapper.Payload)
			}
		}
	}()
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

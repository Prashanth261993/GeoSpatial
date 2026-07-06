// Package bus wraps the Redpanda/Kafka client (franz-go) for producers and
// consumer groups. Positions are keyed by a coarse H3 cell so an area's events
// land on the same partition/consumer (spatial locality), while the indexer
// still indexes at a finer resolution.
package bus

import (
	"context"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	TopicPositions = "positions"
	TopicTrips     = "trips"
	// KeyRes is the H3 resolution used ONLY for partition keying (neighborhood
	// scale). Decoupled from the indexer's finer indexing resolution.
	KeyRes = 7
)

// Brokers parses a comma-separated broker list.
func Brokers(s string) []string { return strings.Split(s, ",") }

// NewProducer returns an idempotent producer (dedupes broker-side retries).
func NewProducer(brokers []string) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		// Idempotent producer is on by default in franz-go (acks=all, no dup on retry).
	)
}

// NewConsumer returns a consumer-group client that reads the given topics.
// Auto-commit is disabled: callers commit offsets manually after processing
// (at-least-once with idempotent handlers).
func NewConsumer(brokers []string, group string, topics ...string) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
	)
}

// Produce sends one record (fire-and-forget with the client's internal batching).
func Produce(ctx context.Context, cl *kgo.Client, topic, key string, val []byte) {
	cl.Produce(ctx, &kgo.Record{Topic: topic, Key: []byte(key), Value: val}, nil)
}

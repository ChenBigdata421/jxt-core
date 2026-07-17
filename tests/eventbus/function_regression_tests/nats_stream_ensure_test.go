package function_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNATSPublish_PerTopicStreamEnsure guards the createdStreams cache fix.
//
// bus.Publish (the non-envelope path, nats.go Publish) must ensure EACH unique
// topic is registered into its JetStream stream via ensureTopicInJetStream →
// addTopicToStream on first publish — not just the first topic.
//
// Pre-fix, Publish's "skip ensure" cache was keyed by STREAM NAME; and
// getStreamNameForTopic returns the SAME configured stream name for every topic.
// So in a shared named stream, only the first topic was ensured; subsequent
// topics skipped ensure and published to subjects the stream didn't cover → no
// JetStream persistence → no ACK.
//
// This test configures a stream whose subject filter does NOT cover the publish
// topics, so each topic MUST be auto-registered into the stream. If the cache
// regresses to stream-name keying, only the first topic appears in the stream's
// subjects and the assertion for the rest fails.
func TestNATSPublish_PerTopicStreamEnsure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping broker-dependent NATS stream-ensure test in short mode")
	}
	clientID := fmt.Sprintf("stream-ensure-%d", time.Now().UnixNano())
	streamName := fmt.Sprintf("TEST_ENSURE_%s", clientID)

	cfg := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		JetStream: eventbus.JetStreamConfig{
			Enabled: true,
			Stream: eventbus.StreamConfig{
				Name: streamName,
				// Narrow filter that does NOT cover the publish topics below — so every
				// topic must be explicitly added to the stream via ensure/addTopicToStream.
				Subjects: []string{clientID + ".exclusive.>"},
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// Publish via the non-envelope path to several topics, NONE of which match the
	// stream's narrow subject filter. Topics are clientID-prefixed so they cannot
	// collide with subjects claimed by leftover streams from prior runs on the
	// shared broker (NATS rejects "subjects overlap with an existing stream").
	// Each first-publish must trigger ensureTopicInJetStream → addTopicToStream.
	topics := []string{
		clientID + ".ensure-alpha",
		clientID + ".ensure-beta",
		clientID + ".ensure-gamma",
		clientID + ".ensure-delta",
	}
	ctx := context.Background()
	for _, topic := range topics {
		require.NoError(t, bus.Publish(ctx, topic, []byte("msg-"+topic)))
	}

	// ensure runs synchronously before PublishAsync; allow a brief settle for the
	// async publish side and the stream UpdateStream to be reflected.
	time.Sleep(500 * time.Millisecond)

	// Inspect the stream: ALL topics must have been added to its subjects list.
	nc, err := nats.Connect("nats://localhost:4223")
	require.NoError(t, err)
	defer nc.Drain()
	js, err := nc.JetStream()
	require.NoError(t, err)
	// Clean up the stream so its subjects don't linger on the shared broker and
	// collide with future runs.
	defer func() { _ = js.DeleteStream(streamName) }()

	info, err := js.StreamInfo(streamName)
	require.NoError(t, err)

	t.Logf("stream %q subjects after publish: %v", streamName, info.Config.Subjects)
	for _, topic := range topics {
		assert.Containsf(t, info.Config.Subjects, topic,
			"topic %q must be ensured into stream %q (added to subjects)", topic, streamName)
	}
}

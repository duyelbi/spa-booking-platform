package realtime

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

const ChannelBookings = "spa:bookings"

// StartRedisSubscriber forwards Redis pub/sub messages to local WebSocket clients (horizontally scalable).
func StartRedisSubscriber(ctx context.Context, rdb *redis.Client, hub *Hub, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	sub := rdb.Subscribe(ctx, ChannelBookings)
	defer func() { _ = sub.Close() }()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				hub.BroadcastJSON(map[string]any{"type": "parse_error", "raw": msg.Payload})
				continue
			}
			hub.BroadcastJSON(payload)
		}
	}
}

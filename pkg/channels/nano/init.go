package nano

import (
	"thor/pkg/bus"
	"thor/pkg/channels"
	"thor/pkg/config"
)

func init() {
	channels.RegisterFactory("nano", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewNanoChannel(cfg.Channels.Nano, b)
	})
}

package telegram

import (
	"thor/pkg/bus"
	"thor/pkg/channels"
	"thor/pkg/config"
)

func init() {
	channels.RegisterFactory("telegram", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewTelegramChannel(cfg, b)
	})
}

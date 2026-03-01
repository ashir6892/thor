// analytics_cron.go — Weekly Tool Analytics Report Sender
// Edison 🤖: Built as part of Brain Loop Cycle 2 (Tool Analytics + Auto-Optimization)
// Sends a weekly analytics report via the message bus every Sunday at 09:00.
package tools

import (
	"context"
	"fmt"
	"time"

	"thor/pkg/bus"
	"thor/pkg/logger"
)

// AnalyticsReporter sends a weekly tool analytics report via the message bus.
type AnalyticsReporter struct {
	bus     *bus.MessageBus
	chatID  string
	channel string
}

// NewAnalyticsReporter creates a new AnalyticsReporter.
func NewAnalyticsReporter(msgBus *bus.MessageBus, channel, chatID string) *AnalyticsReporter {
	return &AnalyticsReporter{
		bus:     msgBus,
		chatID:  chatID,
		channel: channel,
	}
}

// Run generates and sends the weekly analytics report.
// Designed to be called by a scheduled goroutine (every Sunday at 09:00).
func (ar *AnalyticsReporter) Run(ctx context.Context) {
	report, err := GenerateReport(168) // last 7 days
	if err != nil {
		logger.WarnCF("cron", "Analytics report generation failed", map[string]any{
			"error": err.Error(),
		})
		return
	}

	formatted := FormatReport(report)
	msg := fmt.Sprintf("🤖 *Weekly Thor Analytics* — Auto-Report\n\n%s", formatted)

	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := ar.bus.PublishOutbound(pubCtx, bus.OutboundMessage{
		Channel: ar.channel,
		ChatID:  ar.chatID,
		Content: msg,
	}); err != nil {
		logger.WarnCF("cron", "Failed to send weekly analytics report", map[string]any{
			"error":   err.Error(),
			"channel": ar.channel,
			"chat_id": ar.chatID,
		})
		return
	}

	logger.InfoCF("cron", "Weekly analytics report sent", map[string]any{
		"tools_tracked": len(report.Tools),
		"period":        report.Period,
		"channel":       ar.channel,
	})
}

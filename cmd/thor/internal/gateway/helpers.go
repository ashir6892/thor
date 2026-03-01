package gateway

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"thor/cmd/thor/internal"
	"thor/pkg/agent"
	"thor/pkg/bus"
	"thor/pkg/channels"
	_ "thor/pkg/channels/dingtalk"
	_ "thor/pkg/channels/discord"
	_ "thor/pkg/channels/feishu"
	_ "thor/pkg/channels/line"
	_ "thor/pkg/channels/maixcam"
	_ "thor/pkg/channels/onebot"
	_ "thor/pkg/channels/nano"
	_ "thor/pkg/channels/qq"
	_ "thor/pkg/channels/slack"
	_ "thor/pkg/channels/telegram"
	_ "thor/pkg/channels/wecom"
	_ "thor/pkg/channels/whatsapp"
	_ "thor/pkg/channels/whatsapp_native"
	"thor/pkg/config"
	"thor/pkg/cron"
	"thor/pkg/devices"
	"thor/pkg/health"
	"thor/pkg/heartbeat"
	"thor/pkg/logger"
	"thor/pkg/media"
	"thor/pkg/providers"
	"thor/pkg/state"
	"thor/pkg/tools"
)

// tailLines returns the last n lines from a string.
func tailLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// diagnoseUnplannedRestart reads PM2 log files and returns a human-readable
// diagnosis of why Thor restarted unexpectedly.
func diagnoseUnplannedRestart() string {
	const errLogPath = "/data/data/com.termux/files/home/.thor/logs/thor-err.log"
	const outLogPath = "/data/data/com.termux/files/home/.thor/logs/thor-out.log"

	errData, errReadErr := os.ReadFile(errLogPath)
	outData, outReadErr := os.ReadFile(outLogPath)

	// If we can't read either log, fall back gracefully.
	if errReadErr != nil && outReadErr != nil {
		return "Unknown restart reason (logs unreadable)"
	}

	// Work with the last 50 err lines and last 20 out lines.
	errLines := tailLines(string(errData), 50)
	outLines := tailLines(string(outData), 20)

	combined := strings.Join(errLines, "\n") + "\n" + strings.Join(outLines, "\n")
	combinedLower := strings.ToLower(combined)

	// --- Detection rules (most specific first) ---

	// safe-deploy
	if strings.Contains(combinedLower, "safe-deploy") ||
		strings.Contains(combinedLower, "deploying new thor binary") ||
		strings.Contains(combinedLower, "safe deploy") {
		return "Restarted via safe-deploy ✅"
	}

	// panic / runtime error
	if strings.Contains(combinedLower, "panic:") || strings.Contains(combinedLower, "runtime error") {
		// Find the first panic line for context.
		for _, l := range errLines {
			ll := strings.ToLower(l)
			if strings.Contains(ll, "panic:") || strings.Contains(ll, "runtime error") {
				trimmed := strings.TrimSpace(l)
				if len(trimmed) > 120 {
					trimmed = trimmed[:120] + "…"
				}
				return "Crashed: PANIC — " + trimmed
			}
		}
		return "Crashed: PANIC (see logs)"
	}

	// OOM / killed by OS
	if strings.Contains(combinedLower, "signal: killed") ||
		strings.Contains(combinedLower, "out of memory") ||
		strings.Contains(combinedLower, "oom") {
		return "Killed by OS (OOM or signal)"
	}

	// Clean shutdown then restart
	if strings.Contains(combinedLower, "shutting down") {
		return "Clean shutdown then restart"
	}

	// Fatal / exit
	if strings.Contains(combinedLower, "fatal error") || strings.Contains(combinedLower, "exit status") {
		for _, l := range errLines {
			ll := strings.ToLower(l)
			if strings.Contains(ll, "fatal error") || strings.Contains(ll, "exit status") {
				trimmed := strings.TrimSpace(l)
				if len(trimmed) > 120 {
					trimmed = trimmed[:120] + "…"
				}
				return "Fatal error — " + trimmed
			}
		}
	}

	// Unknown — show last 5 err lines as context.
	lastFew := errLines
	if len(lastFew) > 5 {
		lastFew = lastFew[len(lastFew)-5:]
	}
	// Filter out blank lines.
	var nonBlank []string
	for _, l := range lastFew {
		if strings.TrimSpace(l) != "" {
			nonBlank = append(nonBlank, strings.TrimSpace(l))
		}
	}
	if len(nonBlank) == 0 {
		return "Unknown restart reason (no recent error output)"
	}
	return "Unknown reason — last log lines:\n" + strings.Join(nonBlank, "\n")
}

// readRestartContext reads RESTART_CONTEXT.md and builds a startup notification message.
// If an intentional restart context is found, it returns a rich message and clears the file.
// Otherwise it auto-diagnoses the restart cause from PM2 logs.
func readRestartContext(workspacePath string) string {
	contextFile := filepath.Join(workspacePath, "memory", "RESTART_CONTEXT.md")
	data, err := os.ReadFile(contextFile)
	if err != nil {
		// No context file at all — diagnose from logs.
		diagnosis := diagnoseUnplannedRestart()
		now := time.Now().Format("2006-01-02 15:04")
		return fmt.Sprintf("⚡ Thor is back online!\n🔍 Restart reason: %s\n🕐 Time: %s\n\nJust reply to continue! 🦞", diagnosis, now)
	}

	content := string(data)

	// Extract the "## Last Restart" section
	sectionIdx := strings.Index(content, "## Last Restart")
	if sectionIdx == -1 {
		diagnosis := diagnoseUnplannedRestart()
		now := time.Now().Format("2006-01-02 15:04")
		return fmt.Sprintf("⚡ Thor is back online!\n🔍 Restart reason: %s\n🕐 Time: %s\n\nJust reply to continue! 🦞", diagnosis, now)
	}

	section := content[sectionIdx:]

	// Helper to extract a field value
	extractField := func(fieldName string) string {
		marker := "**" + fieldName + "**"
		idx := strings.Index(section, marker)
		if idx == -1 {
			return ""
		}
		line := section[idx+len(marker):]
		// Remove leading ": " or " "
		line = strings.TrimPrefix(line, ":")
		line = strings.TrimSpace(line)
		// Take only first line
		if nl := strings.Index(line, "\n"); nl >= 0 {
			line = line[:nl]
		}
		return strings.TrimSpace(line)
	}

	reason := extractField("Reason:")
	task := extractField("Task:")
	progress := extractField("Progress:")
	expected := extractField("Expected After Restart:")
	restartTime := extractField("Time:")
	status := extractField("Status:")

	// Check if this is a real restart context (not the placeholder)
	if reason == "" || strings.Contains(reason, "none yet") {
		// No intentional context written — diagnose from logs.
		diagnosis := diagnoseUnplannedRestart()
		now := time.Now().Format("2006-01-02 15:04")
		return fmt.Sprintf("⚡ Thor is back online!\n🔍 Restart reason: %s\n🕐 Time: %s\n\nJust reply to continue! 🦞", diagnosis, now)
	}

	// Build rich message for intentional restart
	msg := fmt.Sprintf("⚡ Thor restarted itself!\n\n🔧 Reason: %s\n📌 Task: %s\n✅ Progress: %s\n🎯 Expected: %s\n📊 Status: %s\n🕐 Time: %s\n\nJust reply to continue! 🦞",
		reason, task, progress, expected, status, restartTime)

	// Clear the restart context so next restart shows as unplanned if not written
	clearContent := `# Restart Context

This file is written by Thor BEFORE intentionally restarting itself.
The startup notification reads this file and sends a detailed report to Telegram.

## Last Restart

_(none yet — this file is updated by Thor before each intentional restart)_
`
	_ = os.WriteFile(contextFile, []byte(clearContent), 0644)

	return msg
}

func gatewayCmd(debug bool) error {
	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("🔍 Debug mode enabled")
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	// Use the resolved model ID from provider creation
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Print agent startup info
	fmt.Println("\n📦 Agent Status:")
	startupInfo := agentLoop.GetStartupInfo()
	toolsInfo := startupInfo["tools"].(map[string]any)
	skillsInfo := startupInfo["skills"].(map[string]any)
	fmt.Printf("  • Tools: %d loaded\n", toolsInfo["count"])
	fmt.Printf("  • Skills: %d/%d available\n",
		skillsInfo["available"],
		skillsInfo["total"])

	// Log to file as well
	logger.InfoCF("agent", "Agent initialized",
		map[string]any{
			"tools_count":      toolsInfo["count"],
			"skills_total":     skillsInfo["total"],
			"skills_available": skillsInfo["available"],
		})

	// Setup cron tool and service
	execTimeout := time.Duration(cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	cronService := setupCronTool(
		agentLoop,
		msgBus,
		cfg.WorkspacePath(),
		cfg.Agents.Defaults.RestrictToWorkspace,
		execTimeout,
		cfg,
	)

	heartbeatService := heartbeat.NewHeartbeatService(
		cfg.WorkspacePath(),
		cfg.Heartbeat.Interval,
		cfg.Heartbeat.Enabled,
	)
	heartbeatService.SetBus(msgBus)
	heartbeatService.SetHandler(func(prompt, channel, chatID string) *tools.ToolResult {
		// Use cli:direct as fallback if no valid channel
		if channel == "" || chatID == "" {
			channel, chatID = "cli", "direct"
		}
		// Use ProcessHeartbeat - no session history, each heartbeat is independent
		var response string
		response, err = agentLoop.ProcessHeartbeat(context.Background(), prompt, channel, chatID)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("Heartbeat error: %v", err))
		}
		if response == "HEARTBEAT_OK" {
			return tools.SilentResult("Heartbeat OK")
		}
		// For heartbeat, always return silent - the subagent result will be
		// sent to user via processSystemMessage when the async task completes
		return tools.SilentResult(response)
	})

	// Create media store for file lifecycle management with TTL cleanup
	mediaStore := media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})
	mediaStore.Start()

	channelManager, err := channels.NewManager(cfg, msgBus, mediaStore)
	if err != nil {
		mediaStore.Stop()
		return fmt.Errorf("error creating channel manager: %w", err)
	}

	// Inject channel manager and media store into agent loop
	agentLoop.SetChannelManager(channelManager)
	agentLoop.SetMediaStore(mediaStore)

	enabledChannels := channelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	fmt.Printf("✓ Gateway started on %s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cronService.Start(); err != nil {
		fmt.Printf("Error starting cron service: %v\n", err)
	}
	fmt.Println("✓ Cron service started")

	if err := heartbeatService.Start(); err != nil {
		fmt.Printf("Error starting heartbeat service: %v\n", err)
	}
	fmt.Println("✓ Heartbeat service started")

	stateManager := state.NewManager(cfg.WorkspacePath())
	deviceService := devices.NewService(devices.Config{
		Enabled:    cfg.Devices.Enabled,
		MonitorUSB: cfg.Devices.MonitorUSB,
	}, stateManager)
	deviceService.SetBus(msgBus)
	if err := deviceService.Start(ctx); err != nil {
		fmt.Printf("Error starting device service: %v\n", err)
	} else if cfg.Devices.Enabled {
		fmt.Println("✓ Device event service started")
	}

	// Setup shared HTTP server with health endpoints and webhook handlers
	healthServer := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port)
	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	channelManager.SetupHTTPServer(addr, healthServer)

	if err := channelManager.StartAll(ctx); err != nil {
		fmt.Printf("Error starting channels: %v\n", err)
		return err
	}

	fmt.Printf("✓ Health endpoints available at http://%s:%d/health and /ready\n", cfg.Gateway.Host, cfg.Gateway.Port)

	go agentLoop.Run(ctx)

	// Send startup notification to the owner so they know Thor has come back
	// online (e.g. after a crash or restart).
	go func() {
		// Wait a moment for all channels (Telegram, etc.) to fully initialise
		// their connections before we try to send.
		time.Sleep(3 * time.Second)

		// Always notify the primary owner on Telegram.
		const primaryChannel = "telegram"
		const primaryChatID = "1930168837"

		notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer notifyCancel()

		if err := msgBus.PublishOutbound(notifyCtx, bus.OutboundMessage{
			Channel: primaryChannel,
			ChatID:  primaryChatID,
			Content: readRestartContext(cfg.WorkspacePath()),
		}); err != nil {
			logger.WarnCF("gateway", "Failed to send startup notification", map[string]any{
				"channel": primaryChannel,
				"chat_id": primaryChatID,
				"error":   err.Error(),
			})
		} else {
			logger.InfoCF("gateway", "Startup notification sent", map[string]any{
				"channel": primaryChannel,
				"chat_id": primaryChatID,
			})
		}

		// Also notify the last known channel/user if it differs from the primary.
		startupState := state.NewManager(cfg.WorkspacePath())
		lastChannel := startupState.GetLastChannel()
		if lastChannel == "" {
			return
		}

		// lastChannel is stored as "platform:chatID"
		parts := strings.SplitN(lastChannel, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return
		}
		platform, userID := parts[0], parts[1]

		// Skip if this is the same as the primary notification target.
		if platform == primaryChannel && userID == primaryChatID {
			return
		}

		// Skip internal/system channels — they have no real user on the other end.
		internalChannels := []string{"cli", "system", "subagent", "cron", "direct"}
		for _, ic := range internalChannels {
			if platform == ic {
				return
			}
		}

		lastCtx, lastCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer lastCancel()

		if err := msgBus.PublishOutbound(lastCtx, bus.OutboundMessage{
			Channel: platform,
			ChatID:  userID,
			Content: "⚡ Thor is back online!",
		}); err != nil {
			logger.WarnCF("gateway", "Failed to send startup notification to last channel", map[string]any{
				"channel": platform,
				"chat_id": userID,
				"error":   err.Error(),
			})
			return
		}

		logger.InfoCF("gateway", "Startup notification sent to last channel", map[string]any{
			"channel": platform,
			"chat_id": userID,
		})
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\nShutting down...")
	if cp, ok := provider.(providers.StatefulProvider); ok {
		cp.Close()
	}
	cancel()
	msgBus.Close()

	// Use a fresh context with timeout for graceful shutdown,
	// since the original ctx is already canceled.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	channelManager.StopAll(shutdownCtx)
	deviceService.Stop()
	heartbeatService.Stop()
	cronService.Stop()
	mediaStore.Stop()
	agentLoop.Stop()
	fmt.Println("✓ Gateway stopped")

	return nil
}

func setupCronTool(
	agentLoop *agent.AgentLoop,
	msgBus *bus.MessageBus,
	workspace string,
	restrict bool,
	execTimeout time.Duration,
	cfg *config.Config,
) *cron.CronService {
	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")

	// Create cron service
	cronService := cron.NewCronService(cronStorePath, nil)

	// Create and register CronTool
	cronTool, err := tools.NewCronTool(cronService, agentLoop, msgBus, workspace, restrict, execTimeout, cfg)
	if err != nil {
		log.Fatalf("Critical error during CronTool initialization: %v", err)
	}

	agentLoop.RegisterTool(cronTool)

	// Set the onJob handler
	cronService.SetOnJob(func(job *cron.CronJob) (string, error) {
		result := cronTool.ExecuteJob(context.Background(), job)
		return result, nil
	})

	return cronService
}

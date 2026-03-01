package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegohandler"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"thor/pkg/bus"
	"thor/pkg/channels"
	"thor/pkg/config"
	"thor/pkg/identity"
	"thor/pkg/logger"
	"thor/pkg/media"
	"thor/pkg/utils"
)

var (
	reHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reBlockquote = regexp.MustCompile(`(?m)^>\s*(.*)$`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBoldStar   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldUnder  = regexp.MustCompile(`__(.+?)__`)
	reItalic     = regexp.MustCompile(`\*([^*\n]+)\*`)
	reStrike     = regexp.MustCompile(`~~(.+?)~~`)
	reListItem   = regexp.MustCompile(`(?m)^[-*]\s+`)
	reCodeBlock  = regexp.MustCompile("(?s)```[\\w]*\\n?([\\s\\S]*?)```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
)

type TelegramChannel struct {
	*channels.BaseChannel
	bot      *telego.Bot
	bh       *telegohandler.BotHandler
	commands TelegramCommander
	config   *config.Config
	chatIDs  map[string]int64
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewTelegramChannel(cfg *config.Config, bus *bus.MessageBus) (*TelegramChannel, error) {
	var opts []telego.BotOption
	telegramCfg := cfg.Channels.Telegram

	if telegramCfg.Proxy != "" {
		proxyURL, parseErr := url.Parse(telegramCfg.Proxy)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", telegramCfg.Proxy, parseErr)
		}
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}))
	} else if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		// Use environment proxy if configured
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}))
	}

	bot, err := telego.NewBot(telegramCfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	base := channels.NewBaseChannel(
		"telegram",
		telegramCfg,
		bus,
		telegramCfg.AllowFrom,
		channels.WithMaxMessageLength(4096),
		channels.WithGroupTrigger(telegramCfg.GroupTrigger),
		channels.WithReasoningChannelID(telegramCfg.ReasoningChannelID),
	)

	return &TelegramChannel{
		BaseChannel: base,
		commands:    NewTelegramCommands(bot, cfg),
		bot:         bot,
		config:      cfg,
		chatIDs:     make(map[string]int64),
	}, nil
}

func (c *TelegramChannel) Start(ctx context.Context) error {
	logger.InfoC("telegram", "Starting Telegram bot (polling mode)...")

	c.ctx, c.cancel = context.WithCancel(ctx)

	updates, err := c.bot.UpdatesViaLongPolling(c.ctx, &telego.GetUpdatesParams{
		Timeout: 30,
	})
	if err != nil {
		c.cancel()
		return fmt.Errorf("failed to start long polling: %w", err)
	}

	bh, err := telegohandler.NewBotHandler(c.bot, updates)
	if err != nil {
		c.cancel()
		return fmt.Errorf("failed to create bot handler: %w", err)
	}
	c.bh = bh

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		c.commands.Help(ctx, message)
		return nil
	}, th.CommandEqual("help"))
	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.commands.Start(ctx, message)
	}, th.CommandEqual("start"))

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.commands.Show(ctx, message)
	}, th.CommandEqual("show"))

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.commands.List(ctx, message)
	}, th.CommandEqual("list"))

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleMessage(ctx, &message)
	}, th.AnyMessage())

	c.SetRunning(true)
	logger.InfoCF("telegram", "Telegram bot connected", map[string]any{
		"username": c.bot.Username(),
	})

	go bh.Start()

	return nil
}

func (c *TelegramChannel) Stop(ctx context.Context) error {
	logger.InfoC("telegram", "Stopping Telegram bot...")
	c.SetRunning(false)

	// Stop the bot handler
	if c.bh != nil {
		c.bh.Stop()
	}

	// Cancel our context (stops long polling)
	if c.cancel != nil {
		c.cancel()
	}

	return nil
}

func (c *TelegramChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}

	chatID, err := parseChatID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID %s: %w", msg.ChatID, channels.ErrSendFailed)
	}

	htmlContent := markdownToTelegramHTML(msg.Content)

	// Typing/placeholder handled by Manager.preSend — just send the message
	tgMsg := tu.Message(tu.ID(chatID), htmlContent)
	tgMsg.ParseMode = telego.ModeHTML

	if _, err = c.bot.SendMessage(ctx, tgMsg); err != nil {
		logger.ErrorCF("telegram", "HTML parse failed, falling back to plain text", map[string]any{
			"error": err.Error(),
		})
		tgMsg.ParseMode = ""
		if _, err = c.bot.SendMessage(ctx, tgMsg); err != nil {
			return fmt.Errorf("telegram send: %w", channels.ErrTemporary)
		}
	}

	return nil
}

// StartTyping implements channels.TypingCapable.
// It sends ChatAction(typing) immediately and then repeats every 4 seconds
// (Telegram's typing indicator expires after ~5s) in a background goroutine.
// The returned stop function is idempotent and cancels the goroutine.
func (c *TelegramChannel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	cid, err := parseChatID(chatID)
	if err != nil {
		return func() {}, err
	}

	// Send the first typing action immediately
	_ = c.bot.SendChatAction(ctx, tu.ChatAction(tu.ID(cid), telego.ChatActionTyping))

	typingCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				_ = c.bot.SendChatAction(typingCtx, tu.ChatAction(tu.ID(cid), telego.ChatActionTyping))
			}
		}
	}()

	return cancel, nil
}

// EditMessage implements channels.MessageEditor.
func (c *TelegramChannel) EditMessage(ctx context.Context, chatID string, messageID string, content string) error {
	cid, err := parseChatID(chatID)
	if err != nil {
		return err
	}
	mid, err := strconv.Atoi(messageID)
	if err != nil {
		return err
	}
	htmlContent := markdownToTelegramHTML(content)
	editMsg := tu.EditMessageText(tu.ID(cid), mid, htmlContent)
	editMsg.ParseMode = telego.ModeHTML
	_, err = c.bot.EditMessageText(ctx, editMsg)
	return err
}

// SendPlaceholder implements channels.PlaceholderCapable.
// It sends a placeholder message (e.g. "Thinking... 💭") that will later be
// edited to the actual response via EditMessage (channels.MessageEditor).
func (c *TelegramChannel) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	phCfg := c.config.Channels.Telegram.Placeholder
	if !phCfg.Enabled {
		return "", nil
	}

	text := phCfg.Text
	if text == "" {
		text = "Thinking... 💭"
	}

	cid, err := parseChatID(chatID)
	if err != nil {
		return "", err
	}

	pMsg, err := c.bot.SendMessage(ctx, tu.Message(tu.ID(cid), text))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", pMsg.MessageID), nil
}

// SendMedia implements the channels.MediaSender interface.
func (c *TelegramChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}

	chatID, err := parseChatID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID %s: %w", msg.ChatID, channels.ErrSendFailed)
	}

	store := c.GetMediaStore()
	if store == nil {
		return fmt.Errorf("no media store available: %w", channels.ErrSendFailed)
	}

	for _, part := range msg.Parts {
		localPath, err := store.Resolve(part.Ref)
		if err != nil {
			logger.ErrorCF("telegram", "Failed to resolve media ref", map[string]any{
				"ref":   part.Ref,
				"error": err.Error(),
			})
			continue
		}

		file, err := os.Open(localPath)
		if err != nil {
			logger.ErrorCF("telegram", "Failed to open media file", map[string]any{
				"path":  localPath,
				"error": err.Error(),
			})
			continue
		}

		switch part.Type {
		case "image":
			params := &telego.SendPhotoParams{
				ChatID:  tu.ID(chatID),
				Photo:   telego.InputFile{File: file},
				Caption: part.Caption,
			}
			_, err = c.bot.SendPhoto(ctx, params)
		case "audio":
			params := &telego.SendAudioParams{
				ChatID:  tu.ID(chatID),
				Audio:   telego.InputFile{File: file},
				Caption: part.Caption,
			}
			_, err = c.bot.SendAudio(ctx, params)
		case "video":
			params := &telego.SendVideoParams{
				ChatID:  tu.ID(chatID),
				Video:   telego.InputFile{File: file},
				Caption: part.Caption,
			}
			_, err = c.bot.SendVideo(ctx, params)
		default: // "file" or unknown types
			params := &telego.SendDocumentParams{
				ChatID:   tu.ID(chatID),
				Document: telego.InputFile{File: file},
				Caption:  part.Caption,
			}
			_, err = c.bot.SendDocument(ctx, params)
		}

		file.Close()

		if err != nil {
			logger.ErrorCF("telegram", "Failed to send media", map[string]any{
				"type":  part.Type,
				"error": err.Error(),
			})
			return fmt.Errorf("telegram send media: %w", channels.ErrTemporary)
		}
	}

	return nil
}

func (c *TelegramChannel) handleMessage(ctx context.Context, message *telego.Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	user := message.From
	if user == nil {
		return fmt.Errorf("message sender (user) is nil")
	}

	platformID := fmt.Sprintf("%d", user.ID)
	sender := bus.SenderInfo{
		Platform:    "telegram",
		PlatformID:  platformID,
		CanonicalID: identity.BuildCanonicalID("telegram", platformID),
		Username:    user.Username,
		DisplayName: user.FirstName,
	}

	// check allowlist to avoid downloading attachments for rejected users
	if !c.IsAllowedSender(sender) {
		logger.DebugCF("telegram", "Message rejected by allowlist", map[string]any{
			"user_id": platformID,
		})
		return nil
	}

	chatID := message.Chat.ID
	c.chatIDs[platformID] = chatID

	content := ""
	mediaPaths := []string{}

	chatIDStr := fmt.Sprintf("%d", chatID)
	messageIDStr := fmt.Sprintf("%d", message.MessageID)
	scope := channels.BuildMediaScope("telegram", chatIDStr, messageIDStr)

	// Helper to register a local file with the media store
	storeMedia := func(localPath, filename string) string {
		if store := c.GetMediaStore(); store != nil {
			ref, err := store.Store(localPath, media.MediaMeta{
				Filename: filename,
				Source:   "telegram",
			}, scope)
			if err == nil {
				return ref
			}
		}
		return localPath // fallback: use raw path
	}

	if message.Text != "" {
		content += message.Text
	}

	if message.Caption != "" {
		if content != "" {
			content += "\n"
		}
		content += message.Caption
	}

	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		photoPath := c.downloadPhoto(ctx, photo.FileID)
		if photoPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(photoPath, "photo.jpg"))
			if content != "" {
				content += "\n"
			}
			content += "[image: photo]"
		}
	}

	if message.Voice != nil {
		voicePath := c.downloadFile(ctx, message.Voice.FileID, ".ogg")
		if voicePath != "" {
			mediaPaths = append(mediaPaths, storeMedia(voicePath, "voice.ogg"))

			if content != "" {
				content += "\n"
			}
			content += "[voice]"
		}
	}

	if message.Audio != nil {
		audioPath := c.downloadFile(ctx, message.Audio.FileID, ".mp3")
		if audioPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(audioPath, "audio.mp3"))
			if content != "" {
				content += "\n"
			}
			content += "[audio]"
		}
	}

	if message.Document != nil {
		docPath := c.downloadFile(ctx, message.Document.FileID, "")
		if docPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(docPath, "document"))
			if content != "" {
				content += "\n"
			}
			content += "[file]"
		}
	}

	if content == "" {
		content = "[empty message]"
	}

	// In group chats, apply unified group trigger filtering
	if message.Chat.Type != "private" {
		isMentioned := c.isBotMentioned(message)
		if isMentioned {
			content = c.stripBotMention(content)
		}
		respond, cleaned := c.ShouldRespondInGroup(isMentioned, content)
		if !respond {
			return nil
		}
		content = cleaned
	}

	logger.DebugCF("telegram", "Received message", map[string]any{
		"sender_id": sender.CanonicalID,
		"chat_id":   fmt.Sprintf("%d", chatID),
		"preview":   utils.Truncate(content, 50),
	})

	// Placeholder is now auto-triggered by BaseChannel.HandleMessage via PlaceholderCapable

	peerKind := "direct"
	peerID := fmt.Sprintf("%d", user.ID)
	if message.Chat.Type != "private" {
		peerKind = "group"
		peerID = fmt.Sprintf("%d", chatID)
	}

	peer := bus.Peer{Kind: peerKind, ID: peerID}
	messageID := fmt.Sprintf("%d", message.MessageID)

	metadata := map[string]string{
		"user_id":    fmt.Sprintf("%d", user.ID),
		"username":   user.Username,
		"first_name": user.FirstName,
		"is_group":   fmt.Sprintf("%t", message.Chat.Type != "private"),
	}

	c.HandleMessage(c.ctx,
		peer,
		messageID,
		platformID,
		fmt.Sprintf("%d", chatID),
		content,
		mediaPaths,
		metadata,
		sender,
	)
	return nil
}

func (c *TelegramChannel) downloadPhoto(ctx context.Context, fileID string) string {
	file, err := c.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		logger.ErrorCF("telegram", "Failed to get photo file", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	return c.downloadFileWithInfo(file, ".jpg")
}

func (c *TelegramChannel) downloadFileWithInfo(file *telego.File, ext string) string {
	if file.FilePath == "" {
		return ""
	}

	url := c.bot.FileDownloadURL(file.FilePath)
	logger.DebugCF("telegram", "File URL", map[string]any{"url": url})

	// Use FilePath as filename for better identification
	filename := file.FilePath + ext
	return utils.DownloadFile(url, filename, utils.DownloadOptions{
		LoggerPrefix: "telegram",
	})
}

func (c *TelegramChannel) downloadFile(ctx context.Context, fileID, ext string) string {
	file, err := c.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		logger.ErrorCF("telegram", "Failed to get file", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	return c.downloadFileWithInfo(file, ext)
}

// SendStreaming implements channels.StreamingCapable.
// It sends an initial placeholder message ("✍️ _thinking..._"), then
// progressively edits it as the streamFn calls the edit callback.
// A background goroutine throttles edits to ≤1 per 600ms to stay well
// within Telegram's ~20 edits/min rate limit.
// On completion a final edit commits the fully-formatted HTML response.
func (c *TelegramChannel) SendStreaming(ctx context.Context, chatID string, streamFn func(edit func(text string)) error) error {
	cid, err := parseChatID(chatID)
	if err != nil {
		return fmt.Errorf("SendStreaming: invalid chat ID %s: %w", chatID, err)
	}

	// Send placeholder
	sent, err := c.bot.SendMessage(ctx, tu.Message(tu.ID(cid), "✍️ _thinking..._").WithParseMode(telego.ModeMarkdown))
	if err != nil {
		return fmt.Errorf("SendStreaming: failed to send placeholder: %w", err)
	}
	msgID := sent.MessageID

	var mu sync.Mutex
	var buf strings.Builder
	lastEdit := time.Now().Add(-1 * time.Second) // allow first edit immediately

	doEdit := func() {
		mu.Lock()
		content := buf.String()
		mu.Unlock()
		if content == "" {
			return
		}
		htmlContent := markdownToTelegramHTML(content)
		editParams := &telego.EditMessageTextParams{
			ChatID:    tu.ID(cid),
			MessageID: msgID,
			Text:      htmlContent,
			ParseMode: telego.ModeHTML,
		}
		if _, editErr := c.bot.EditMessageText(ctx, editParams); editErr != nil {
			// Silently swallow edit errors (e.g. message not modified) — not fatal
			logger.DebugCF("telegram", "SendStreaming edit error (non-fatal)", map[string]any{
				"error": editErr.Error(),
			})
		}
	}

	editFn := func(text string) {
		mu.Lock()
		buf.Reset()
		buf.WriteString(text)
		mu.Unlock()
	}

	// Periodic edit goroutine — fires every 600ms
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if time.Since(lastEdit) >= 500*time.Millisecond {
					doEdit()
					lastEdit = time.Now()
				}
			case <-done:
				return
			}
		}
	}()

	streamErr := streamFn(editFn)
	close(done)

	// Final edit with fully-accumulated content
	doEdit()

	return streamErr
}

func parseChatID(chatIDStr string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(chatIDStr, "%d", &id)
	return id, err
}

func markdownToTelegramHTML(text string) string {
	if text == "" {
		return ""
	}

	// Step 1: Extract code blocks (protect from all transformations)
	var cbCodes []string
	cbIdx := 0
	text = reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		match := reCodeBlock.FindStringSubmatch(m)
		cbCodes = append(cbCodes, match[1])
		p := fmt.Sprintf("\x00CB%d\x00", cbIdx)
		cbIdx++
		return p
	})

	// Step 2: Extract inline code
	var icCodes []string
	icIdx := 0
	text = reInlineCode.ReplaceAllStringFunc(text, func(m string) string {
		match := reInlineCode.FindStringSubmatch(m)
		icCodes = append(icCodes, match[1])
		p := fmt.Sprintf("\x00IC%d\x00", icIdx)
		icIdx++
		return p
	})

	// Step 3: Extract links before HTML escaping so URLs aren't mangled
	type linkEntry struct{ label, url string }
	var links []linkEntry
	text = reLink.ReplaceAllStringFunc(text, func(m string) string {
		match := reLink.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		links = append(links, linkEntry{match[1], match[2]})
		return fmt.Sprintf("\x00LK%d\x00", len(links)-1)
	})

	// Step 4: Extract headings (strip ## markers, restore as <b> after escaping)
	type headingEntry struct{ content string }
	var headings []headingEntry
	text = reHeading.ReplaceAllStringFunc(text, func(m string) string {
		match := reHeading.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		headings = append(headings, headingEntry{match[1]})
		return fmt.Sprintf("\x00HB%d\x00", len(headings)-1)
	})

	// Step 5: Extract blockquotes
	type bqEntry struct{ content string }
	var bqs []bqEntry
	text = reBlockquote.ReplaceAllStringFunc(text, func(m string) string {
		match := reBlockquote.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		bqs = append(bqs, bqEntry{match[1]})
		return fmt.Sprintf("\x00BQ%d\x00", len(bqs)-1)
	})

	// Step 6: Escape HTML special chars in remaining plain text
	text = escapeHTML(text)

	// Step 7: Apply inline markdown → HTML (bold before italic to avoid conflicts)
	text = reBoldStar.ReplaceAllString(text, "<b>$1</b>")
	text = reBoldUnder.ReplaceAllString(text, "<b>$1</b>")
	text = reItalic.ReplaceAllString(text, "<i>$1</i>")
	text = reStrike.ReplaceAllString(text, "<s>$1</s>")

	// Step 8: Convert list items
	text = reListItem.ReplaceAllString(text, "• ")

	// Step 9: Restore headings (escape their content too)
	for i, h := range headings {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00HB%d\x00", i), "<b>"+escapeHTML(h.content)+"</b>")
	}

	// Step 10: Restore blockquotes
	for i, bq := range bqs {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00BQ%d\x00", i), "<i>"+escapeHTML(bq.content)+"</i>")
	}

	// Step 11: Restore links
	for i, lk := range links {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00LK%d\x00", i), fmt.Sprintf(`<a href="%s">%s</a>`, lk.url, escapeHTML(lk.label)))
	}

	// Step 12: Restore inline code
	for i, code := range icCodes {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), fmt.Sprintf("<code>%s</code>", escapeHTML(code)))
	}

	// Step 13: Restore code blocks
	for i, code := range cbCodes {
		text = strings.ReplaceAll(
			text,
			fmt.Sprintf("\x00CB%d\x00", i),
			fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(code)),
		)
	}

	return text
}


func escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// isBotMentioned checks if the bot is mentioned in the message via entities.
func (c *TelegramChannel) isBotMentioned(message *telego.Message) bool {
	botUsername := c.bot.Username()
	if botUsername == "" {
		return false
	}

	entities := message.Entities
	if entities == nil {
		entities = message.CaptionEntities
	}

	for _, entity := range entities {
		if entity.Type == "mention" {
			// Extract the mention text from the message
			text := message.Text
			if text == "" {
				text = message.Caption
			}
			runes := []rune(text)
			end := entity.Offset + entity.Length
			if end <= len(runes) {
				mention := string(runes[entity.Offset:end])
				if strings.EqualFold(mention, "@"+botUsername) {
					return true
				}
			}
		}
		if entity.Type == "text_mention" && entity.User != nil {
			if entity.User.Username == botUsername {
				return true
			}
		}
	}
	return false
}

// stripBotMention removes the @bot mention from the content.
func (c *TelegramChannel) stripBotMention(content string) string {
	botUsername := c.bot.Username()
	if botUsername == "" {
		return content
	}
	// Case-insensitive replacement
	re := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(botUsername))
	content = re.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

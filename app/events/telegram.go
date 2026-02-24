package events

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Romancha/server-healthcheck-telegram-bot/app/checks"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Server represents a monitored server with its URL and display name.
type Server struct {
	URL  string
	Name string
}

// Callback action constants for inline keyboard buttons.
const (
	ActionRemove          = "remove"
	ActionSetResponseTime = "set_response_time"
	ActionSetContent      = "set_content"
	ActionSetSSLThreshold = "set_ssl_threshold"
	ActionDetails         = "details"
)

// ListenTelegramUpdates starts listening for Telegram updates and processes them until the context is canceled.
func ListenTelegramUpdates(ctx context.Context, bot *tgbotapi.BotAPI, superUsers SuperUser) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			log.Printf("[INFO] Stopped receiving Telegram updates")
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			processUpdate(bot, update, superUsers)
		}
	}
}

func processUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update, superUsers SuperUser) {
	// Handle callback queries (inline button clicks)
	if update.CallbackQuery != nil {
		handleCallbackQuery(bot, update.CallbackQuery, superUsers)
		return
	}

	// Ignore if no message
	if update.Message == nil {
		return
	}

	// check if is not superuser, ignore
	if update.Message.From == nil || !superUsers.IsSuper(update.Message.From.UserName) {
		return
	}

	if update.Message.IsCommand() {
		command := strings.ToLower(update.Message.Command())

		switch command {
		case "add":
			handleAddCommand(bot, update)

		case "remove":
			handleRemoveCommand(bot, update)

		case "removeall":
			handleRemoveAllCommand(bot, update)

		case "list":
			sendServerList(bot, update.Message.Chat.ID)

		case "stats":
			sendServerStats(bot, update.Message.Chat.ID)

		case "help":
			sendHelpMessage(bot, update.Message.Chat.ID)

		case "setresponsetime":
			handleSetResponseTime(bot, update)

		case "setcontent":
			handleSetContent(bot, update)

		case "setsslthreshold":
			handleSetSSLThreshold(bot, update)

		case "setglobalsslthreshold":
			handleSetGlobalSSLThreshold(bot, update)

		case "details":
			serverName := update.Message.CommandArguments()
			if serverName == "" {
				_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
					"Usage: /details [server_name]"))
				return
			}

			sendServerDetails(bot, update.Message.Chat.ID, serverName)
		}
	}
}

func handleAddCommand(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	if update.Message.CommandArguments() == "" {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Please provide a URL. Usage: /add [url] [name]"))
		return
	}

	server := getServer(update.Message)

	if server.URL == "" || server.URL == "https://" || server.URL == "http://" {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"URL cannot be empty. Usage: /add [url] [name]"))
		return
	}

	checksData := checks.ReadChecksData()

	if _, ok := checksData.HealthChecks[server.Name]; ok {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Server already exists"))
		return
	}

	if checksData.HealthChecks == nil {
		checksData.HealthChecks = make(map[string]checks.ServerCheck)
	}

	checksData.HealthChecks[server.Name] = checks.ServerCheck{
		Name: server.Name,
		URL:  server.URL,
		IsOk: false,
	}

	if saveErr := checks.SaveChecksData(checksData); saveErr != nil {
		log.Printf("[ERROR] Failed to save checks data: %v", saveErr)
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			fmt.Sprintf("Failed to add server %s [%s]", server.Name, server.URL)))
		return
	}

	_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
		fmt.Sprintf("Server %s [%s] added", server.Name, server.URL)))
}

func handleRemoveCommand(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	if update.Message.CommandArguments() == "" {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Please provide a server name. Usage: /remove [name]"))
		return
	}

	server := getServer(update.Message)

	if server.Name == "" {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Server name cannot be empty. Usage: /remove [name]"))
		return
	}

	checksData := checks.ReadChecksData()

	if _, ok := checksData.HealthChecks[server.Name]; !ok {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			fmt.Sprintf("Server %s not exists", server.Name)))
		return
	}

	delete(checksData.HealthChecks, server.Name)

	if saveErr := checks.SaveChecksData(checksData); saveErr != nil {
		log.Printf("[ERROR] Failed to save checks data: %v", saveErr)
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			fmt.Sprintf("Failed to remove server %s", server.Name)))
		return
	}

	_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
		fmt.Sprintf("Server %s removed", server.Name)))
}

func handleRemoveAllCommand(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	emptyData := checks.Data{
		HealthChecks: make(map[string]checks.ServerCheck),
	}

	if saveErr := checks.SaveChecksData(emptyData); saveErr != nil {
		log.Printf("[ERROR] Failed to save checks data: %v", saveErr)
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Failed to remove all servers"))
		return
	}

	_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "All servers removed"))
}

func updateServerField(bot *tgbotapi.BotAPI, chatID int64, serverName string, updateFn func(*checks.ServerCheck), successMsg string) {
	checksData := checks.ReadChecksData()

	server, ok := checksData.HealthChecks[serverName]
	if !ok {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Server %s not found", serverName)))
		return
	}

	updateFn(&server)
	checksData.HealthChecks[serverName] = server

	if err := checks.SaveChecksData(checksData); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Failed to save: %v", err)))
		return
	}

	_, _ = bot.Send(tgbotapi.NewMessage(chatID, successMsg))
}

func handleSetResponseTime(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	args := strings.Split(update.Message.CommandArguments(), " ")
	if len(args) < 2 {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Usage: /setresponsetime [server_name] [threshold_ms]"))
		return
	}

	var threshold int
	if _, err := fmt.Sscanf(args[1], "%d", &threshold); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Invalid threshold value. Please provide a number in milliseconds."))
		return
	}

	updateServerField(bot, update.Message.Chat.ID, args[0],
		func(s *checks.ServerCheck) { s.ResponseTimeThreshold = threshold },
		fmt.Sprintf("Response time threshold for %s set to %dms", args[0], threshold))
}

func handleSetContent(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	args := strings.Split(update.Message.CommandArguments(), " ")
	if len(args) < 2 {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Usage: /setcontent [server_name] [expected_content]"))
		return
	}

	serverName := args[0]
	expectedContent := strings.Join(args[1:], " ")

	updateServerField(bot, update.Message.Chat.ID, serverName,
		func(s *checks.ServerCheck) { s.ExpectedContent = expectedContent },
		fmt.Sprintf("Expected content for %s set to: %s", serverName, expectedContent))
}

func handleSetSSLThreshold(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	args := strings.Split(update.Message.CommandArguments(), " ")
	if len(args) < 2 {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Usage: /setsslthreshold [server_name] [days]"))
		return
	}

	var threshold int
	if _, err := fmt.Sscanf(args[1], "%d", &threshold); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Invalid threshold value. Please provide a number of days."))
		return
	}

	updateServerField(bot, update.Message.Chat.ID, args[0],
		func(s *checks.ServerCheck) { s.SSLExpiryThreshold = threshold },
		fmt.Sprintf("SSL expiry threshold for %s set to %d days", args[0], threshold))
}

func handleSetGlobalSSLThreshold(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	thresholdStr := update.Message.CommandArguments()
	if thresholdStr == "" {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Usage: /setglobalsslthreshold [days]"))
		return
	}

	var threshold int
	if _, err := fmt.Sscanf(thresholdStr, "%d", &threshold); err != nil {
		_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
			"Invalid threshold value. Please provide a number of days."))
		return
	}

	checks.SetGlobalSSLExpiryThreshold(threshold)
	_, _ = bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
		fmt.Sprintf("Global SSL expiry threshold set to %d days", threshold)))
}

func handleCallbackQuery(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery, superUsers SuperUser) {
	// Answer callback query to remove loading indicator
	callback := tgbotapi.NewCallback(query.ID, "")
	_, _ = bot.Request(callback)

	// Check if user is authorized
	if !superUsers.IsSuper(query.From.UserName) {
		return
	}

	// Parse callback data
	parts := strings.Split(query.Data, ":")
	if len(parts) < 2 {
		return
	}

	action := parts[0]
	serverName := parts[1]

	switch action {
	case ActionRemove:
		// Remove server
		checksData := checks.ReadChecksData()
		if _, ok := checksData.HealthChecks[serverName]; ok {
			delete(checksData.HealthChecks, serverName)
			_ = checks.SaveChecksData(checksData)

			// Update message
			msg := tgbotapi.NewEditMessageText(
				query.Message.Chat.ID,
				query.Message.MessageID,
				fmt.Sprintf("Server %s removed", serverName),
			)
			_, _ = bot.Send(msg)
		}

	case ActionDetails:
		// Show server details
		sendServerDetails(bot, query.Message.Chat.ID, serverName)

	case ActionSetResponseTime:
		// Send message asking for response time threshold
		msg := tgbotapi.NewMessage(
			query.Message.Chat.ID,
			fmt.Sprintf("Please use /setresponsetime %s [threshold_ms] to set response time threshold", serverName),
		)
		_, _ = bot.Send(msg)

	case ActionSetContent:
		// Send message asking for expected content
		msg := tgbotapi.NewMessage(
			query.Message.Chat.ID,
			fmt.Sprintf("Please use /setcontent %s [expected_content] to set expected content", serverName),
		)
		_, _ = bot.Send(msg)

	case ActionSetSSLThreshold:
		// Send message asking for SSL threshold
		msg := tgbotapi.NewMessage(
			query.Message.Chat.ID,
			fmt.Sprintf("Please use /setsslthreshold %s [days] to set SSL expiry threshold", serverName),
		)
		_, _ = bot.Send(msg)
	}
}

func sendServerList(bot *tgbotapi.BotAPI, chatID int64) {
	var checksData = checks.ReadChecksData()

	if len(checksData.HealthChecks) == 0 {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "No servers"))
		return
	}

	for _, serverCheck := range checksData.HealthChecks {
		var serverStatus string
		if serverCheck.IsOk {
			serverStatus = "✅"
		} else {
			serverStatus = "❌"
		}

		msg := tgbotapi.NewMessage(chatID,
			fmt.Sprintf("%s %s [%s]", serverStatus, serverCheck.Name, serverCheck.URL))

		// Add inline buttons
		var keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Details", fmt.Sprintf("%s:%s", ActionDetails, serverCheck.Name)),
				tgbotapi.NewInlineKeyboardButtonData("Remove", fmt.Sprintf("%s:%s", ActionRemove, serverCheck.Name)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Set Response Time", fmt.Sprintf("%s:%s", ActionSetResponseTime, serverCheck.Name)),
				tgbotapi.NewInlineKeyboardButtonData("Set Content", fmt.Sprintf("%s:%s", ActionSetContent, serverCheck.Name)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Set SSL Threshold", fmt.Sprintf("%s:%s", ActionSetSSLThreshold, serverCheck.Name)),
			),
		)
		msg.ReplyMarkup = keyboard

		_, _ = bot.Send(msg)
	}
}

func sendServerStats(bot *tgbotapi.BotAPI, chatID int64) {
	var checksData = checks.ReadChecksData()

	if len(checksData.HealthChecks) == 0 {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "No servers"))
		return
	}

	var statsMessage string
	statsMessage = "📊 *Server Statistics*\n\n"

	for _, serverCheck := range checksData.HealthChecks {
		var statusEmoji string
		if serverCheck.IsOk {
			statusEmoji = "✅"
		} else {
			statusEmoji = "❌"
		}

		statsMessage += fmt.Sprintf("*%s* %s\n", serverCheck.Name, statusEmoji)
		statsMessage += fmt.Sprintf("URL: %s\n", serverCheck.URL)
		statsMessage += fmt.Sprintf("Availability: %.1f%%\n", serverCheck.Availability)
		statsMessage += fmt.Sprintf("Last success: %s\n", checks.FormatTimeAgo(serverCheck.LastSuccess))

		if !serverCheck.IsOk {
			statsMessage += fmt.Sprintf("Last failure: %s\n", checks.FormatTimeAgo(serverCheck.LastFailure))
		}

		if serverCheck.LastResponseTime > 0 {
			statsMessage += fmt.Sprintf("Response time: %dms\n", serverCheck.LastResponseTime)
		}

		if !serverCheck.SSLExpiryDate.IsZero() {
			daysToExpiry := int(time.Until(serverCheck.SSLExpiryDate).Hours() / 24)
			statsMessage += fmt.Sprintf("SSL expires in: %d days\n", daysToExpiry)

			if !serverCheck.LastSSLNotification.IsZero() {
				statsMessage += fmt.Sprintf("Last SSL notification: %s\n", checks.FormatTimeAgo(serverCheck.LastSSLNotification))
			}
		}

		statsMessage += "\n"
	}

	msg := tgbotapi.NewMessage(chatID, statsMessage)
	msg.ParseMode = "Markdown"
	_, _ = bot.Send(msg)
}

func sendServerDetails(bot *tgbotapi.BotAPI, chatID int64, serverName string) {
	checksData := checks.ReadChecksData()

	server, ok := checksData.HealthChecks[serverName]
	if !ok {
		_, _ = bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Server %s not found", serverName)))
		return
	}

	statusEmoji := "❌"
	if server.IsOk {
		statusEmoji = "✅"
	}

	detailsMsg := fmt.Sprintf("*%s* %s\n\n", server.Name, statusEmoji)
	detailsMsg += fmt.Sprintf("URL: %s\n", server.URL)
	detailsMsg += fmt.Sprintf("Status: %s\n", statusEmoji)
	detailsMsg += fmt.Sprintf("Availability: %.1f%%\n", server.Availability)
	detailsMsg += fmt.Sprintf("Total checks: %d\n", server.TotalChecks)
	detailsMsg += fmt.Sprintf("Successful checks: %d\n", server.SuccessfulChecks)
	detailsMsg += fmt.Sprintf("Last success: %s\n", checks.FormatTimeAgo(server.LastSuccess))
	detailsMsg += fmt.Sprintf("Last failure: %s\n", checks.FormatTimeAgo(server.LastFailure))

	if server.LastResponseTime > 0 {
		detailsMsg += fmt.Sprintf("Last response time: %dms\n", server.LastResponseTime)
	}

	if server.ResponseTimeThreshold > 0 {
		detailsMsg += fmt.Sprintf("Response time threshold: %dms\n", server.ResponseTimeThreshold)
	}

	if server.ExpectedContent != "" {
		detailsMsg += fmt.Sprintf("Expected content: %s\n", server.ExpectedContent)
	}

	if !server.SSLExpiryDate.IsZero() {
		detailsMsg += fmt.Sprintf("SSL expiry date: %s\n", server.SSLExpiryDate.Format("2006-01-02"))
		daysToExpiry := int(time.Until(server.SSLExpiryDate).Hours() / 24)
		detailsMsg += fmt.Sprintf("SSL expires in: %d days\n", daysToExpiry)

		sslThreshold := "global"
		if server.SSLExpiryThreshold > 0 {
			sslThreshold = fmt.Sprintf("%d days", server.SSLExpiryThreshold)
		}
		detailsMsg += fmt.Sprintf("SSL threshold: %s\n", sslThreshold)

		if !server.LastSSLNotification.IsZero() {
			detailsMsg += fmt.Sprintf("Last SSL notification: %s\n", checks.FormatTimeAgo(server.LastSSLNotification))
		}
	}

	msg := tgbotapi.NewMessage(chatID, detailsMsg)
	msg.ParseMode = "Markdown"
	_, _ = bot.Send(msg)
}

func sendHelpMessage(bot *tgbotapi.BotAPI, chatID int64) {
	helpMsg := `*Server Health Check Bot Help*

*Commands:*
/add [url] [name] - Add server to monitor
/remove [name] - Remove server from monitor
/removeall - Remove all servers from monitor
/list - Show list of monitored servers with actions
/stats - Show detailed statistics for all servers
/details [name] - Show detailed information for a specific server
/setresponsetime [name] [threshold_ms] - Set response time threshold in milliseconds
/setcontent [name] [text] - Set expected content that should be present in the response
/setsslthreshold [name] [days] - Set SSL expiry threshold for specific server
/setglobalsslthreshold [days] - Set global SSL expiry threshold
/help - Show this help message

*Features:*
• HTTP status code monitoring (2xx is considered success)
• Response time monitoring
• Expected content verification
• SSL certificate expiration monitoring
• Availability statistics
• Inline buttons for easy management`

	msg := tgbotapi.NewMessage(chatID, helpMsg)
	msg.ParseMode = "Markdown"
	_, _ = bot.Send(msg)
}

func getServer(message *tgbotapi.Message) Server {
	userArg := strings.Split(message.CommandArguments(), " ")

	// Check if arguments are empty
	if len(userArg) == 0 || userArg[0] == "" {
		return Server{}
	}

	originalURL := userArg[0]
	fullURL := getFullServerURL(userArg[0])

	var serverName string
	if len(userArg) > 1 {
		serverName = userArg[1]
	}

	if serverName == "" {
		serverName = originalURL
	}

	return Server{
		URL:  fullURL,
		Name: serverName,
	}
}

func getFullServerURL(serverURL string) string {
	if serverURL == "" {
		return ""
	}

	if !strings.HasPrefix(serverURL, "https://") && !strings.HasPrefix(serverURL, "http://") {
		serverURL = "https://" + serverURL
	}

	return serverURL
}

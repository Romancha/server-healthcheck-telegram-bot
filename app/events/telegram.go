package events

import (
	"fmt"
	"github.com/Romancha/server-healthcheck-telegram-bot/app/checks"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"strings"
)

type Server struct {
	Url  string
	Name string
}

func ListenTelegramUpdates(bot *tgbotapi.BotAPI, superUsers SuperUser) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		// check if is not superuser, ignore
		if !superUsers.IsSuper(update.Message.From.UserName) {
			continue
		}

		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "add":
				var server = getServer(update.Message)
				var checksData = checks.ReadChecksData()

				if _, ok := checksData.HealthChecks[server.Name]; ok {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Server already exists")
					bot.Send(msg)
					continue
				} else {
					if checksData.HealthChecks == nil {
						checksData.HealthChecks = make(map[string]checks.ServerCheck)
					}

					checksData.HealthChecks[server.Name] = checks.ServerCheck{
						Name: server.Name,
						Url:  server.Url,
						IsOk: false,
					}
				}

				saveError := checks.SaveChecksData(checksData)
				if saveError != nil {
					log.Printf("[ERROR] Failed to save checks data: %v", saveError)
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("Failed to add server %s [%s]", server.Name, server.Url)),
					)
					continue
				}

				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(
					"Server %s [%s] added", server.Name, server.Url)),
				)

			case "remove":
				var server = getServer(update.Message)
				var checksData = checks.ReadChecksData()

				if _, ok := checksData.HealthChecks[server.Name]; ok {
					delete(checksData.HealthChecks, server.Name)
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf(
						"Server %s removed", server.Name),
					)
					bot.Send(msg)
				} else {
					msg := tgbotapi.NewMessage(
						update.Message.Chat.ID, fmt.Sprintf("Server %s not exists", server.Name),
					)
					bot.Send(msg)
					continue
				}

				saveError := checks.SaveChecksData(checksData)
				if saveError != nil {
					log.Printf("[ERROR] Failed to save checks data: %v", saveError)
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("Failed to remove server %s", server)),
					)
					continue
				}

			case "removeAll":
				var emptyData = checks.Data{
					HealthChecks: make(map[string]checks.ServerCheck),
				}

				saveError := checks.SaveChecksData(emptyData)
				if saveError != nil {
					log.Printf("[ERROR] Failed to save checks data: %v", saveError)
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID,
						fmt.Sprintf("Failed to remove all servers")),
					)
					continue
				}

				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "All servers removed"))

			case "list":
				var checksData = checks.ReadChecksData()

				var serverList string
				for _, serverCheck := range checksData.HealthChecks {
					var serverStatus string
					if serverCheck.IsOk {
						serverStatus = "✅"
					} else {
						serverStatus = "❌"
					}

					serverList += fmt.Sprintf("%s %s [%s]\n", serverStatus, serverCheck.Name, serverCheck.Url)
				}

				if serverList == "" {
					serverList = "No servers"
				}

				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, serverList))
			}
		}
	}
}

func getServer(message *tgbotapi.Message) Server {
	var userArg = strings.Split(message.CommandArguments(), " ")

	var originalUrl = userArg[0]
	var fullUrl = getFullServerUrl(userArg[0])

	var serverName string
	if len(userArg) > 1 {
		serverName = userArg[1]
	}

	if serverName == "" {
		serverName = originalUrl
	}

	return Server{
		Url:  fullUrl,
		Name: serverName,
	}

}

func getFullServerUrl(serverUrl string) string {
	if (strings.HasPrefix(serverUrl, "https://") ||
		strings.HasPrefix(serverUrl, "http://")) == false {
		serverUrl = "https://" + serverUrl
	}

	return serverUrl
}

package checks

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
	"net/http"
	"time"
)

type Data struct {
	HealthChecks map[string]ServerCheck `json:"healthChecks"`
}
type ServerCheck struct {
	Name        string    `json:"name"`
	Url         string    `json:"url"`
	LastFailure time.Time `json:"lastFailure"`
	LastSuccess time.Time `json:"lastSuccess"`
	IsOk        bool      `json:"isOk"`
}

var failuresToInform = map[string]int{}

func PerformCheck(bot *tgbotapi.BotAPI, chatId int64, alertThreshold int) {
	log.Printf("[INFO] Cron job started")

	var checksData = ReadChecksData()

	for _, serverCheck := range checksData.HealthChecks {
		var serverAvailable = serverStatusIsOk(serverCheck.Url)
		var checkTime = time.Now()

		if serverAvailable {
			serverCheck.LastSuccess = checkTime
		} else {
			serverCheck.LastFailure = checkTime
		}
		serverCheck.IsOk = serverAvailable

		// append new check to server checks
		checksData.HealthChecks[serverCheck.Name] = serverCheck

		if !serverAvailable {
			failuresToInform[serverCheck.Name]++

			log.Printf("[INFO] Server %s is down %v times", serverCheck.Url, failuresToInform[serverCheck.Url])
			if failuresToInform[serverCheck.Name] >= alertThreshold {
				msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("❗❗❗ Server %s is down ❗❗❗", serverCheck.Url))
				_, err := bot.Send(msg)
				if err != nil {
					log.Printf("[ERROR] Failed to send message: %v", err)
				}

				failuresToInform[serverCheck.Name] = 0
			}
		} else {
			if failuresToInform[serverCheck.Name] > 0 {
				msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("✅ Server %s is up 🎉", serverCheck.Url))
				_, err := bot.Send(msg)
				if err != nil {
					log.Printf("[ERROR] Failed to send message: %v", err)
				}
			}

			failuresToInform[serverCheck.Name] = 0
		}

		// save checks data
		err := SaveChecksData(checksData)
		if err != nil {
			log.Printf("[ERROR] Error while saving checks data: %v", err)
			continue
		}
	}
}

func serverStatusIsOk(serverUrl string) bool {
	resp, err := http.Get(serverUrl)
	if err != nil {
		log.Printf("[DEBUG] Failed to get server status: %v", err)
		return false
	}
	defer resp.Body.Close()

	var code = resp.StatusCode

	log.Printf("[DEBUG] server %v, code: %v", serverUrl, code)

	return code == http.StatusOK
}

package checks

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Data struct {
	HealthChecks map[string]ServerCheck `json:"healthChecks"`
}

type ServerCheck struct {
	Name                  string    `json:"name"`
	Url                   string    `json:"url"`
	LastFailure           time.Time `json:"lastFailure"`
	LastSuccess           time.Time `json:"lastSuccess"`
	IsOk                  bool      `json:"isOk"`
	ExpectedContent       string    `json:"expectedContent,omitempty"`
	ResponseTimeThreshold int       `json:"responseTimeThreshold,omitempty"` // in milliseconds
	LastResponseTime      int64     `json:"lastResponseTime"`                // in milliseconds
	SSLExpiryDate         time.Time `json:"sslExpiryDate,omitempty"`
	SSLExpiryThreshold    int       `json:"sslExpiryThreshold,omitempty"`  // in days
	LastSSLNotification   time.Time `json:"lastSSLNotification,omitempty"` // when the last SSL expiry notification was sent
	Availability          float64   `json:"availability"`                  // percentage of successful checks
	TotalChecks           int       `json:"totalChecks"`
	SuccessfulChecks      int       `json:"successfulChecks"`
}

type CheckResult struct {
	IsOk           bool
	ResponseTime   int64
	StatusCode     int
	ContentMatched bool
	SSLExpiryDate  time.Time
	ErrorMessage   string
}

var serverFailureCount = map[string]int{}
var serverSendFaultMessage = map[string]bool{}
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

// Global threshold for SSL certificates (in days)
var globalSSLExpiryThreshold = 30

// ResetState clears failure counters and fault message flags (used in tests)
func ResetState() {
	serverFailureCount = map[string]int{}
	serverSendFaultMessage = map[string]bool{}
}

// ConfigureHttpClient sets the timeout for the HTTP client
func ConfigureHttpClient(timeout time.Duration) {
	httpClient.Timeout = timeout
	transport, ok := httpClient.Transport.(*http.Transport)
	if ok {
		transport.TLSHandshakeTimeout = timeout / 2
	}
	log.Printf("[DEBUG] HTTP client timeout set to %v", timeout)
}

// SetGlobalSSLExpiryThreshold sets the global threshold for SSL certificate expiration checks
func SetGlobalSSLExpiryThreshold(days int) {
	globalSSLExpiryThreshold = days
	log.Printf("[DEBUG] Global SSL expiry threshold set to %d days", days)
}

// shouldSendSSLNotification checks if we should send an SSL notification based on the last notification time
func shouldSendSSLNotification(lastNotification time.Time) bool {
	// If never notified before, or it's been more than 24 hours since the last notification
	return lastNotification.IsZero() || time.Since(lastNotification) > 24*time.Hour
}

func PerformCheck(bot *tgbotapi.BotAPI, chatId int64, alertThreshold int) {
	log.Printf("[DEBUG] Cron job started")
	log.Printf("[DEBUG] serverFailureCount: %v", serverFailureCount)
	log.Printf("[DEBUG] serverSendFaultMessage: %v", serverSendFaultMessage)

	var checksData = ReadChecksData()

	for _, serverCheck := range checksData.HealthChecks {
		var checkResult = checkServerStatus(serverCheck)
		var checkTime = time.Now()

		// Update server check data
		serverCheck.TotalChecks++
		if checkResult.IsOk {
			serverCheck.LastSuccess = checkTime
			serverCheck.SuccessfulChecks++
		} else {
			serverCheck.LastFailure = checkTime
		}
		serverCheck.IsOk = checkResult.IsOk
		serverCheck.LastResponseTime = checkResult.ResponseTime

		// Calculate availability percentage
		if serverCheck.TotalChecks > 0 {
			serverCheck.Availability = float64(serverCheck.SuccessfulChecks) / float64(serverCheck.TotalChecks) * 100
		}

		// Update SSL expiry date if available
		if !checkResult.SSLExpiryDate.IsZero() {
			serverCheck.SSLExpiryDate = checkResult.SSLExpiryDate

			// Use individual threshold if set, otherwise use global threshold
			sslThreshold := globalSSLExpiryThreshold
			if serverCheck.SSLExpiryThreshold > 0 {
				sslThreshold = serverCheck.SSLExpiryThreshold
			}

			// Check if SSL certificate is about to expire
			daysToExpiry := int(serverCheck.SSLExpiryDate.Sub(time.Now()).Hours() / 24)
			if daysToExpiry < sslThreshold && daysToExpiry >= 0 {
				// Check if we should send a notification (not more than once per day)
				if shouldSendSSLNotification(serverCheck.LastSSLNotification) {
					msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("⚠️ SSL certificate for %s will expire in %d days (threshold: %d days)",
						serverCheck.Url, daysToExpiry, sslThreshold))
					_, err := bot.Send(msg)
					if err != nil {
						log.Printf("[ERROR] Failed to send SSL expiry message: %v", err)
					} else {
						// Update the last notification time
						serverCheck.LastSSLNotification = checkTime
						log.Printf("[INFO] Sent SSL expiry notification for %s, will expire in %d days", serverCheck.Url, daysToExpiry)
					}
				} else {
					log.Printf("[DEBUG] Skipping SSL notification for %s, last notification was %s",
						serverCheck.Url, FormatTimeAgo(serverCheck.LastSSLNotification))
				}
			}
		}

		// append new check to server checks
		checksData.HealthChecks[serverCheck.Name] = serverCheck

		if !checkResult.IsOk {
			serverFailureCount[serverCheck.Name]++

			log.Printf("[INFO] Server %s is down %v times", serverCheck.Url, serverFailureCount[serverCheck.Name])
			if serverFailureCount[serverCheck.Name] >= alertThreshold {
				msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("❗❗❗ Server %s is down ❗❗❗\nReason: %s", serverCheck.Url, checkResult.ErrorMessage))
				_, err := bot.Send(msg)
				if err != nil {
					log.Printf("[ERROR] Failed to send message: %v", err)
				}

				serverSendFaultMessage[serverCheck.Name] = true
				serverFailureCount[serverCheck.Name] = 0
			}
		} else {
			// Check response time threshold
			if serverCheck.ResponseTimeThreshold > 0 && checkResult.ResponseTime > int64(serverCheck.ResponseTimeThreshold) {
				msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("⚠️ Server %s response time is slow: %dms (threshold: %dms)",
					serverCheck.Url, checkResult.ResponseTime, serverCheck.ResponseTimeThreshold))
				_, err := bot.Send(msg)
				if err != nil {
					log.Printf("[ERROR] Failed to send slow response message: %v", err)
				}
			}

			if serverSendFaultMessage[serverCheck.Name] {
				msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("✅ Server %s is up 🎉", serverCheck.Url))
				_, err := bot.Send(msg)
				if err != nil {
					log.Printf("[ERROR] Failed to send message: %v", err)
				}

				serverSendFaultMessage[serverCheck.Name] = false
			}

			serverFailureCount[serverCheck.Name] = 0
		}

		// save checks data
		err := SaveChecksData(checksData)
		if err != nil {
			log.Printf("[ERROR] Error while saving checks data: %v", err)
			continue
		}
	}
}

func checkServerStatus(server ServerCheck) CheckResult {
	result := CheckResult{
		IsOk: false,
	}

	startTime := time.Now()

	// Create request
	req, err := http.NewRequest("GET", server.Url, nil)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to create request: %v", err)
		return result
	}

	// Execute request
	resp, err := httpClient.Do(req)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to get server status: %v", err)
		return result
	}
	defer resp.Body.Close()

	// Calculate response time
	responseTime := time.Since(startTime)
	result.ResponseTime = responseTime.Milliseconds()
	result.StatusCode = resp.StatusCode

	// Check if status code is 2xx (success)
	isStatusOk := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !isStatusOk {
		result.ErrorMessage = fmt.Sprintf("Status code %d is not successful", resp.StatusCode)
		return result
	}

	// Check content if expected content is set
	if server.ExpectedContent != "" {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to read response body: %v", err)
			return result
		}

		bodyString := string(bodyBytes)
		if !strings.Contains(bodyString, server.ExpectedContent) {
			result.ErrorMessage = "Expected content not found in response"
			result.ContentMatched = false
			return result
		}
		result.ContentMatched = true
	}

	// Check SSL certificate expiration
	if strings.HasPrefix(server.Url, "https://") {
		host := strings.TrimPrefix(server.Url, "https://")
		if idx := strings.Index(host, "/"); idx != -1 {
			host = host[:idx]
		}
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}

		conn, err := tls.Dial("tcp", host+":443", &tls.Config{})
		if err == nil {
			defer conn.Close()
			certs := conn.ConnectionState().PeerCertificates
			if len(certs) > 0 {
				result.SSLExpiryDate = certs[0].NotAfter
			}
		}
	}

	result.IsOk = true
	return result
}

func FormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%d seconds ago", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	}
}

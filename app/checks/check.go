package checks

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Data holds all monitored servers and their health check state.
type Data struct {
	HealthChecks map[string]ServerCheck `json:"healthChecks"`
}

// ServerCheck stores the configuration and state for a single monitored server.
type ServerCheck struct {
	Name                  string    `json:"name"`
	URL                   string    `json:"url"`
	LastFailure           time.Time `json:"lastFailure"`
	LastSuccess           time.Time `json:"lastSuccess"`
	IsOk                  bool      `json:"isOk"`
	ExpectedContent       string    `json:"expectedContent,omitempty"`
	ResponseTimeThreshold int       `json:"responseTimeThreshold,omitempty"` // in milliseconds
	LastResponseTime      int64     `json:"lastResponseTime"`                // in milliseconds
	SSLExpiryDate         time.Time `json:"sslExpiryDate,omitzero"`
	SSLExpiryThreshold    int       `json:"sslExpiryThreshold,omitempty"` // in days
	LastSSLNotification   time.Time `json:"lastSSLNotification,omitzero"` // when the last SSL expiry notification was sent
	Availability          float64   `json:"availability"`                 // percentage of successful checks
	TotalChecks           int       `json:"totalChecks"`
	SuccessfulChecks      int       `json:"successfulChecks"`
}

// CheckResult contains the outcome of a single health check request.
type CheckResult struct {
	IsOk           bool
	ResponseTime   int64
	StatusCode     int
	ContentMatched bool
	SSLExpiryDate  time.Time
	ErrorMessage   string
}

var stateMu sync.Mutex
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

// resetState clears failure counters and fault message flags (used in tests)
func resetState() {
	stateMu.Lock()
	defer stateMu.Unlock()
	serverFailureCount = map[string]int{}
	serverSendFaultMessage = map[string]bool{}
}

// ConfigureHTTPClient sets the timeout for the HTTP client
func ConfigureHTTPClient(timeout time.Duration) {
	stateMu.Lock()
	defer stateMu.Unlock()
	httpClient.Timeout = timeout
	transport, ok := httpClient.Transport.(*http.Transport)
	if ok {
		transport.TLSHandshakeTimeout = timeout / 2
	}
	log.Printf("[DEBUG] HTTP client timeout set to %v", timeout)
}

// getHTTPClientTimeout returns the current HTTP client timeout (used in tests).
func getHTTPClientTimeout() time.Duration {
	stateMu.Lock()
	defer stateMu.Unlock()
	return httpClient.Timeout
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

// PerformCheck runs health checks on all monitored servers and sends alerts via Telegram.
func PerformCheck(bot *tgbotapi.BotAPI, chatID int64, alertThreshold int) {
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
			checkSSLExpiry(bot, chatID, &serverCheck, checkTime)
		}

		// append new check to server checks
		checksData.HealthChecks[serverCheck.Name] = serverCheck

		if !checkResult.IsOk {
			handleServerDown(bot, chatID, serverCheck, checkResult, alertThreshold)
		} else {
			handleServerUp(bot, chatID, serverCheck, checkResult)
		}

		// save checks data
		err := SaveChecksData(checksData)
		if err != nil {
			log.Printf("[ERROR] Error while saving checks data: %v", err)
			continue
		}
	}
}

func checkSSLExpiry(bot *tgbotapi.BotAPI, chatID int64, serverCheck *ServerCheck, checkTime time.Time) {
	sslThreshold := globalSSLExpiryThreshold
	if serverCheck.SSLExpiryThreshold > 0 {
		sslThreshold = serverCheck.SSLExpiryThreshold
	}

	daysToExpiry := int(time.Until(serverCheck.SSLExpiryDate).Hours() / 24)
	if daysToExpiry >= sslThreshold || daysToExpiry < 0 {
		return
	}

	if !shouldSendSSLNotification(serverCheck.LastSSLNotification) {
		log.Printf("[DEBUG] Skipping SSL notification for %s, last notification was %s",
			serverCheck.URL, FormatTimeAgo(serverCheck.LastSSLNotification))
		return
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ SSL certificate for %s will expire in %d days (threshold: %d days)",
		serverCheck.URL, daysToExpiry, sslThreshold))
	if _, err := bot.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send SSL expiry message: %v", err)
		return
	}

	serverCheck.LastSSLNotification = checkTime
	log.Printf("[INFO] Sent SSL expiry notification for %s, will expire in %d days", serverCheck.URL, daysToExpiry)
}

func handleServerDown(bot *tgbotapi.BotAPI, chatID int64, serverCheck ServerCheck, checkResult CheckResult, alertThreshold int) {
	stateMu.Lock()
	serverFailureCount[serverCheck.Name]++
	count := serverFailureCount[serverCheck.Name]
	stateMu.Unlock()

	log.Printf("[INFO] Server %s is down %v times", serverCheck.URL, count)
	if count >= alertThreshold {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❗❗❗ Server %s is down ❗❗❗\nReason: %s", serverCheck.URL, checkResult.ErrorMessage))
		if _, err := bot.Send(msg); err != nil {
			log.Printf("[ERROR] Failed to send message: %v", err)
		}

		stateMu.Lock()
		serverSendFaultMessage[serverCheck.Name] = true
		serverFailureCount[serverCheck.Name] = 0
		stateMu.Unlock()
	}
}

func handleServerUp(bot *tgbotapi.BotAPI, chatID int64, serverCheck ServerCheck, checkResult CheckResult) {
	if serverCheck.ResponseTimeThreshold > 0 && checkResult.ResponseTime > int64(serverCheck.ResponseTimeThreshold) {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("⚠️ Server %s response time is slow: %dms (threshold: %dms)",
			serverCheck.URL, checkResult.ResponseTime, serverCheck.ResponseTimeThreshold))
		if _, err := bot.Send(msg); err != nil {
			log.Printf("[ERROR] Failed to send slow response message: %v", err)
		}
	}

	stateMu.Lock()
	wasFault := serverSendFaultMessage[serverCheck.Name]
	if wasFault {
		serverSendFaultMessage[serverCheck.Name] = false
	}
	serverFailureCount[serverCheck.Name] = 0
	stateMu.Unlock()

	if wasFault {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Server %s is up 🎉", serverCheck.URL))
		if _, err := bot.Send(msg); err != nil {
			log.Printf("[ERROR] Failed to send message: %v", err)
		}
	}
}

func checkServerStatus(server ServerCheck) CheckResult {
	result := CheckResult{
		IsOk: false,
	}

	startTime := time.Now()

	// Create request
	req, err := http.NewRequest("GET", server.URL, http.NoBody)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to create request: %v", err)
		return result
	}

	// Execute request
	resp, err := httpClient.Do(req) //nolint:gosec // URL is provided by trusted superuser via bot command
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
	result.SSLExpiryDate = getSSLExpiryDate(server.URL)

	result.IsOk = true
	return result
}

func getSSLExpiryDate(serverURL string) time.Time {
	host, ok := strings.CutPrefix(serverURL, "https://")
	if !ok {
		return time.Time{}
	}

	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	conn, err := tls.Dial("tcp", host+":443", &tls.Config{MinVersion: tls.VersionTLS12})
	if err != nil {
		return time.Time{}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) > 0 {
		return certs[0].NotAfter
	}

	return time.Time{}
}

// FormatTimeAgo returns a human-readable string representing how long ago the given time was.
func FormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%d seconds ago", int(duration.Seconds()))
	case duration < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	}
}

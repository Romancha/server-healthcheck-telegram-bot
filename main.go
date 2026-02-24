package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Romancha/server-healthcheck-telegram-bot/app/checks"
	"github.com/Romancha/server-healthcheck-telegram-bot/app/events"
	"github.com/Romancha/server-healthcheck-telegram-bot/app/healthcheck"
	"github.com/go-pkgz/lgr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jessevdk/go-flags"
	"github.com/robfig/cron/v3"
)

var opts struct {
	Telegram struct {
		Token string `long:"token" env:"TOKEN" description:"Telegram bot token" required:"true"`
		Chat  int64  `long:"chat" env:"CHAT" description:"Telegram chat id" required:"true"`
		Proxy string `long:"proxy" env:"PROXY" description:"HTTP proxy URL for Telegram API (e.g. http://user:pass@host:port)"`
	} `group:"Telegram" namespace:"telegram" env-namespace:"TELEGRAM"`

	AlertThreshold      int              `long:"alert-threshold" env:"ALERT_THRESHOLD" description:"Alert threshold" default:"3"`
	ChecksCron          string           `long:"checks-cron" env:"CHECKS_CRON" description:"Cron spec for checks" default:"*/30 * * * * *"`
	SuperUsers          events.SuperUser `long:"super" description:"Users names who can manage bot"`
	HTTPTimeout         int              `long:"http-timeout" env:"HTTP_TIMEOUT" description:"HTTP request timeout in seconds" default:"10"`
	SSLExpiryAlertDays  int              `long:"ssl-expiry-alert" env:"SSL_EXPIRY_ALERT" description:"Days before SSL expiry to start alerting" default:"30"`
	DefaultResponseTime int              `long:"default-response-time" env:"DEFAULT_RESPONSE_TIME" description:"Default response time threshold in milliseconds (0 to disable)" default:"0"`
	HealthPort          string           `long:"health-port" env:"HEALTH_PORT" description:"Port for health check HTTP server" default:"8081"`

	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

func main() {
	fmt.Println("Server health check bot started")
	if _, err := flags.Parse(&opts); err != nil {
		log.Printf("[ERROR] failed to parse flags: %v", err)
		os.Exit(1)
	}

	setupLog(opts.Debug)
	checks.InitStorage()

	// Configure HTTP client
	checks.ConfigureHTTPClient(time.Duration(opts.HTTPTimeout) * time.Second)

	// Configure SSL expiry threshold
	checks.SetGlobalSSLExpiryThreshold(opts.SSLExpiryAlertDays)

	var (
		bot *tgbotapi.BotAPI
		err error
	)
	if opts.Telegram.Proxy != "" {
		proxyURL, parseErr := url.Parse(opts.Telegram.Proxy)
		if parseErr != nil {
			log.Fatalf("failed to parse proxy URL: %v", parseErr)
		}
		proxyClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
		bot, err = tgbotapi.NewBotAPIWithClient(opts.Telegram.Token, tgbotapi.APIEndpoint, proxyClient)
		log.Printf("[INFO] Using proxy for Telegram API: %s", proxyURL.Host)
	} else {
		bot, err = tgbotapi.NewBotAPI(opts.Telegram.Token)
	}
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}
	bot.Debug = opts.Debug

	// Set up bot commands menu
	setupBotCommands(bot)

	_, err = bot.Send(tgbotapi.NewMessage(opts.Telegram.Chat, "Server health check bot started"))
	if err != nil {
		log.Printf("[ERROR] Failed to send start message: %v", err)
	}

	// Context for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start health check HTTP server
	go func() {
		if startErr := healthcheck.Start(ctx, ":"+opts.HealthPort, bot); startErr != nil {
			log.Printf("[ERROR] Health check server failed: %v", startErr)
		}
	}()

	// Start cron scheduler
	c := cron.New(cron.WithSeconds())
	if _, cronErr := c.AddFunc(opts.ChecksCron, func() {
		checks.PerformCheck(bot, opts.Telegram.Chat, opts.AlertThreshold)
	}); cronErr != nil {
		log.Printf("[ERROR] failed to add cron: %v", cronErr)
		cancel()
		return
	}
	c.Start()

	// Start listening for Telegram updates (blocks until context is canceled)
	go events.ListenTelegramUpdates(ctx, bot, opts.SuperUsers)

	// Wait for shutdown signal
	<-ctx.Done()
	log.Printf("[INFO] Shutdown signal received, stopping...")

	// Stop cron scheduler
	c.Stop()
	log.Printf("[INFO] Cron scheduler stopped")

	// Send shutdown message
	_, err = bot.Send(tgbotapi.NewMessage(opts.Telegram.Chat, "Server health check bot stopped"))
	if err != nil {
		log.Printf("[ERROR] Failed to send stop message: %v", err)
	}

	log.Printf("[INFO] Bot stopped gracefully")
}

// setupBotCommands configures the commands menu shown in Telegram
func setupBotCommands(bot *tgbotapi.BotAPI) {
	commands := []tgbotapi.BotCommand{
		{Command: "add", Description: "Add server to monitor"},
		{Command: "remove", Description: "Remove server from monitor"},
		{Command: "removeall", Description: "Remove all servers from monitor"},
		{Command: "list", Description: "Show list of monitored servers"},
		{Command: "stats", Description: "Show detailed statistics for all servers"},
		{Command: "details", Description: "Show detailed information for a server"},
		{Command: "setresponsetime", Description: "Set response time threshold"},
		{Command: "setcontent", Description: "Set expected content in response"},
		{Command: "setsslthreshold", Description: "Set SSL expiry threshold for server"},
		{Command: "setglobalsslthreshold", Description: "Set global SSL expiry threshold"},
		{Command: "help", Description: "Show help message with all commands"},
	}

	setMyCommandsConfig := tgbotapi.NewSetMyCommands(commands...)
	_, err := bot.Request(setMyCommandsConfig)
	if err != nil {
		log.Printf("[ERROR] Failed to set bot commands: %v", err)
	} else {
		log.Printf("[INFO] Bot commands menu successfully configured")
	}
}

func setupLog(dbg bool) {
	logOpts := []lgr.Option{lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}
	lgr.SetupStdLogger(logOpts...)
}

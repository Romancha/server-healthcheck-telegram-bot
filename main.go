package main

import (
	"fmt"
	"github.com/Romancha/server-healthcheck-telegram-bot/app/checks"
	"github.com/Romancha/server-healthcheck-telegram-bot/app/events"
	"github.com/go-pkgz/lgr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jessevdk/go-flags"
	"github.com/robfig/cron/v3"
	"log"
	"os"
)

var opts struct {
	Telegram struct {
		Token string `long:"token" env:"TOKEN" description:"Telegram bot token" required:"true"`
		Chat  int64  `long:"chat" env:"CHAT" description:"Telegram chat id" required:"true"`
	} `group:"Telegram" namespace:"telegram" env-namespace:"TELEGRAM"`

	AlertThreshold int              `long:"alert-threshold" env:"ALERT_THRESHOLD" description:"Alert threshold" default:"3"`
	ChecksCron     string           `long:"checks-cron" env:"CHECKS_CRON" description:"Cron spec for checks" default:"*/30 * * * * *"`
	SuperUsers     events.SuperUser `long:"super-users" env:"SUPER_USERS" description:"Users names who can manage bot"`

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

	bot, err := tgbotapi.NewBotAPI(opts.Telegram.Token)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}
	bot.Debug = opts.Debug

	_, err = bot.Send(tgbotapi.NewMessage(opts.Telegram.Chat, "Server health check bot started"))
	if err != nil {
		log.Printf("[ERROR] Failed to send start message: %v", err)
	}

	c := cron.New(cron.WithSeconds())
	_, err = c.AddFunc(opts.ChecksCron, func() {
		checks.PerformCheck(bot, opts.Telegram.Chat, opts.AlertThreshold)
	})
	if err != nil {
		log.Fatalf("failed to add cron: %v", err)
	}
	c.Start()

	events.ListenTelegramUpdates(bot, opts.SuperUsers)
}

func setupLog(dbg bool) {
	logOpts := []lgr.Option{lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}
	lgr.SetupStdLogger(logOpts...)
}

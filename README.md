# Server Health Check Telegram Bot

![GitHub release (with filter)](https://img.shields.io/github/v/release/Romancha/server-healthcheck-telegram-bot)
![GitHub Release Date - Published_At](https://img.shields.io/github/release-date/romancha/server-healthcheck-telegram-bot)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/Romancha/server-healthcheck-telegram-bot/blob/master/LICENSE)

## Introduction

The Server Health Check Telegram Bot monitors the status of your servers and sends notifications to Telegram if the
server is unavailable.

Bot sends requests to the servers and checks the response code. If the response code is not 2xx, the bot
sends a message to the specified chat.

<img src="images/server_check_screen.jpg" width="600px">

## Features

- HTTP status code monitoring (2xx is considered success)
- Response time monitoring with configurable thresholds
- Expected content verification in responses
- SSL certificate expiration monitoring with configurable alert thresholds
  - SSL expiry notifications are limited to once per day to prevent spam
- Availability statistics tracking
- Human-readable time formats for last checks
- Inline buttons for easy server management
- Detailed server statistics
- Telegram commands menu for easy access to all commands

## Installation and Usage

### Docker (Recommended)

1. Install [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
2. Create your bot and get a token from [@BotFather](https://t.me/BotFather)
3. Get `chat_id` from [@userinfobot](https://t.me/userinfobot)
4. Set mandatory env in [docker-compose.yml](/docker/docker-compose.yml):
   - `TELEGRAM_TOKEN` - your bot token
   - `TELEGRAM_CHAT` - your chat id
   - Configure the volumes to persist servers list
5. Run: `docker-compose up -d`

### From source

You can also run the bot from source code, build Go binary and run it.

## Configuration

| Param               | Description                                                                                                 |
|---------------------|-------------------------------------------------------------------------------------------------------------|
| TELEGRAM_TOKEN      | Telegram bot token, take from [@BotFather](https://t.me/BotFather)                                          |
| TELEGRAM_CHAT       | Chat ID where the bot will send messages. [@userinfobot](https://t.me/userinfobot) Can help to get chat id  |
| ALERT_THRESHOLD     | The number of failed requests after which the bot will send a notification. Default ``3``                   |
| CHECKS_CRON         | [Cron](https://en.wikipedia.org/wiki/Cron) with seconds to checks server status. Default ``*/30 * * * * *`` |
| HTTP_TIMEOUT        | HTTP request timeout in seconds. Default ``10``                                                             |
| SSL_EXPIRY_ALERT    | Days before SSL expiry to start alerting. Default ``30``                                                    |
| DEFAULT_RESPONSE_TIME | Default response time threshold in milliseconds (0 to disable). Default ``0``                             |
| DEBUG               | Enable debug mode. Default ``false``                                                                        |

## Commands

All commands are available in the Telegram commands menu (/) for easy access.

| Command                           | Description                                                                |
|-----------------------------------|----------------------------------------------------------------------------|
| /add [url] [name]                 | Add server to monitor. For example: ``/add github.com github``             |
| /remove [name]                    | Remove server from monitor. For example: ``/remove github``                |
| /removeall                        | Remove all servers from monitor                                            |
| /list                             | Show list of monitored servers with action buttons                         |
| /stats                            | Show detailed statistics for all servers                                   |
| /details [name]                   | Show detailed information for a specific server                            |
| /setresponsetime [name] [ms]      | Set response time threshold in milliseconds                                |
| /setcontent [name] [text]         | Set expected content that should be present in the response                |
| /setsslthreshold [name] [days]    | Set SSL expiry threshold for specific server                               |
| /setglobalsslthreshold [days]     | Set global SSL expiry threshold for all servers                            |
| /help                             | Show help message with all available commands                              |

## Contributing

We welcome contributions to improve this project.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
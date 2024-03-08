package main

import (
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yearn/ydaemon/common/helpers"
	"github.com/yearn/ydaemon/common/logs"
)

var initializedCounter = 0

func triggerTgMessage(message string) {
	telegramToken, ok := os.LookupEnv("TELEGRAM_BOT")
	if !ok {
		logs.Error(`TELEGRAM_BOT environment variable not set`)
		return
	}
	telegramChatStr, ok := os.LookupEnv("TELEGRAM_CHAT")
	if !ok {
		logs.Error(`TELEGRAM_CHAT environment variable not set`)
		return
	}
	telegramChat, err := strconv.ParseInt(telegramChatStr, 10, 64)
	if err != nil {
		logs.Error(`TELEGRAM_CHAT environment variable is not a valid chat ID`)
		return
	}
	bot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		logs.Error(`Error initializing Telegram bot: ` + err.Error())
		return
	}
	m, err := bot.Send(tgbotapi.NewMessage(telegramChat, message))
	if err != nil {
		logs.Error(`Error sending message to Telegram: ` + err.Error())
	}
	_ = m
}

func triggerInitializedStatus(chainID uint64) {
	initializedCounter++
	triggerTgMessage(`✅ - yDaemon V2 initialized for chain ` + strconv.FormatUint(chainID, 10) + ` (` + strconv.Itoa(initializedCounter) + `/` + strconv.Itoa(len(chains)) + `)`)
}

func listenToSignals() {
	telegramToken, ok := os.LookupEnv("TELEGRAM_BOT")
	if !ok {
		logs.Error(`TELEGRAM_BOT environment variable not set`)
		return
	}
	telegramWhitelistedUsers, ok := os.LookupEnv("TELEGRAM_WHITELIST")
	if !ok {
		logs.Error(`TELEGRAM_WHITELIST environment variable not set`)
		return
	}
	telegramWhitelistedUsersArray := strings.Split(telegramWhitelistedUsers, `,`)
	bot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		logs.Error(`Error initializing Telegram bot: ` + err.Error())
		return
	}
	u := tgbotapi.NewUpdate(0)
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		if !update.Message.IsCommand() {
			continue
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
		lowercaseUserName := strings.ToLower(update.Message.From.UserName)
		if !helpers.Contains(telegramWhitelistedUsersArray, lowercaseUserName) {
			logs.Error(`Unauthorized user: ` + update.Message.From.UserName)
			msg.Text = "You are not authorized to use this bot."
			bot.Send(msg)
			continue
		}
		// Extract the command from the Message.
		switch update.Message.Command() {
		case "help":
			msg.Text = "I understand /restart."
			bot.Send(msg)
		case "restart":
			triggerTgMessage(`🔴 - ` + update.Message.From.UserName + ` asked for a restart`)
			os.Exit(1)
		default:
			msg.Text = "I don't know that command"
			bot.Send(msg)
		}
	}
}
package main

import (
	"fmt"
	"ftp2sms/internal/filewatcher"
	"ftp2sms/internal/wg2"
	"github.com/alecthomas/kong"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log"
	"log/slog"
)

type Context struct {
	Debug bool
}

type RunCmd struct {
	MqttServer string `help:"MQTT Server"`
}

var cli struct {
	Debug bool `help:"Enable debug mode."`

	Run RunCmd `cmd:"" help:"Start server"`
}

func main() {
	ctx := kong.Parse(&cli)
	err := ctx.Run(&Context{Debug: cli.Debug})
	ctx.FatalIfErrorf(err)
}

var CLI struct {
	Run struct {
		MqttServer string `help:"MQTT Server"`
	} `cmd:"" help:"Run the server."`
}

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func (r *RunCmd) Run(ctx *Context) error {
	if r.MqttServer == "" {
		logger.Warn("MQTT Server is required")
		os.Exit(1)
	}
	folder := "data"

	fw, err := filewatcher.NewFileWatcher(logger)
	if err != nil {
		log.Fatal(err)
	}

	fw.Watch(folder)

	wg2Client := wg2.NewClient(logger, r.MqttServer)

	fw.OnUpdate(func(phoneNumber, filePath, content string) {
		if strings.HasSuffix(filePath, "-sent.txt") || strings.HasSuffix(filePath, "-received.txt") {
			logger.Info("Skip processed", "filePath", filePath)
			return
		}

		logger.Info("Received file", "filePath", filePath, "phoneNumber", phoneNumber)
		rename(filePath)

		sms := wg2.Sms{
			From:    "+4790658023",
			To:      phoneNumber,
			Content: strings.TrimSpace(content),
		}
		if err := wg2Client.SendSms(sms); err != nil {
			logger.Error("Failed to send sms", "error", err)
		}
	})

	wg2Client.OnSms(func(sms wg2.Sms) {
		// Store to file
		filePath := fmt.Sprintf("%s/%s/%d-sms-received.txt", folder, sms.From, time.Now().Unix())
		if !strings.HasSuffix(sms.Content, "\n") {
			sms.Content += "\n"
		}
		bytes := []byte(sms.Content)
		err := os.WriteFile(filePath, bytes, 0644)
		if err != nil {
			logger.Warn("Failed to write file", "filePath", filePath, "error", err)
			return
		}
		logger.Info("Stored incoming SMS", "from", sms.From, "filePath", filePath)
	})

	// Block the main thread indefinitely
	select {}
}

func rename(filePath string) {
	dir := filepath.Dir(filePath)

	newFilename := fmt.Sprintf("%d-sms-sent.txt", time.Now().Unix())
	newFilePath := filepath.Join(dir, newFilename)

	if err := os.Rename(filePath, newFilePath); err != nil {
		logger.Warn("Failed to rename file", "error", err)
	} else {
		logger.Info("File renamed", "oldFilePath", filePath, "newFilePath", newFilePath)
	}
}

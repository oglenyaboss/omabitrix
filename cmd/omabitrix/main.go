// Команда omabitrix — TUI-клиент Bitrix24 (foot по SUPER+B, тосты, бар).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"omabitrix/internal/b24"
	"omabitrix/internal/config"
	"omabitrix/internal/tui"
)

func main() {
	chat := flag.String("chat", "", "dialog_id для открытия (например chat10839)")
	taskID := flag.Int64("task", 0, "id задачи для открытия карточки")
	flag.Parse()

	path := os.Getenv("OMABITRIX_CONFIG")
	if path == "" {
		path = os.ExpandEnv("$HOME/.config/omabitrix/config.toml")
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "omabitrix:", err)
		os.Exit(1)
	}

	client := b24.New(cfg.WebhookURL())
	client.RateLimit = 500 * time.Millisecond
	api := &tui.API{C: client, My: int64(cfg.UserID)}

	ctx := context.Background()
	m := tui.NewModel(api, ctx)
	if *chat != "" {
		m.OpenChat(*chat)
	}
	if *taskID != 0 {
		m.OpenTask(*taskID)
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "omabitrix:", err)
		os.Exit(1)
	}
}

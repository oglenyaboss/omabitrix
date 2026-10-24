// Команда omabitrix — TUI-клиент Bitrix24 (foot по SUPER+B, тосты, бар).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oglenyaboss/omabitrix/internal/b24"
	"github.com/oglenyaboss/omabitrix/internal/config"
	"github.com/oglenyaboss/omabitrix/internal/demo"
	"github.com/oglenyaboss/omabitrix/internal/tui"
)

func main() {
	chat := flag.String("chat", "", "dialog_id для открытия (например chat10839)")
	taskID := flag.Int64("task", 0, "id задачи для открытия карточки")
	demoMode := flag.Bool("demo", false, "демо: встроенный фейковый портал, без конфига и сети")
	flag.Parse()

	var api *tui.API
	if *demoMode {
		dir, err := os.MkdirTemp("", "omabitrix-demo-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "omabitrix:", err)
			os.Exit(1)
		}
		defer os.RemoveAll(dir)
		os.Setenv("XDG_CACHE_HOME", dir)
		srv := demo.Start(demo.Options{PingAfter: 12 * time.Second})
		defer srv.Close()
		client := b24.New(srv.URL + demo.WebhookPath)
		client.RateLimit = 150 * time.Millisecond
		api = &tui.API{C: client, My: demo.MyID, Portal: demo.Portal}
	} else {
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
		api = &tui.API{C: client, My: int64(cfg.UserID), Portal: cfg.Portal}
	}

	ctx := context.Background()
	m := tui.NewModel(api, ctx)
	if *chat != "" {
		m.OpenChat(*chat)
	}
	if *taskID != 0 {
		m.OpenTask(*taskID)
	}
	if *chat == "" && *taskID == 0 {
		m.PlayIntro()
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "omabitrix:", err)
		os.Exit(1)
	}
}

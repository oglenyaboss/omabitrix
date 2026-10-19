// omabitrixd — демон-компаньон Bitrix24 для Omarchy.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"omabitrix/internal/b24"
	"omabitrix/internal/config"
	"omabitrix/internal/daemon"
	"omabitrix/internal/notify"
	"omabitrix/internal/state"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "путь к config.toml")
	once := flag.Bool("once", false, "один цикл и выход")
	doctor := flag.Bool("doctor", false, "проверить вебхук и выйти")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	client := b24.New(cfg.WebhookURL())

	if *doctor {
		doctorMode(client)
		return
	}

	st, err := state.Load(cfg.StatePath)
	if err != nil {
		log.Fatal(err)
	}

	lock, err := acquireLock(filepath.Join(filepath.Dir(cfg.StatePath), "omabitrix.lock"))
	if err != nil {
		log.Fatal(err)
	}
	defer lock.Close()

	d := &daemon.Daemon{Cfg: cfg, Client: client, State: st, Send: notify.NewOmarchy(), Loc: cfg.Location()}

	if *once {
		if err := d.Cycle(context.Background()); err != nil {
			log.Fatal(err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := d.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
	log.Print("остановлен")
}

func doctorMode(client *b24.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, admin, err := client.Profile(ctx)
	if err != nil {
		fmt.Println("❌ профиль:", err)
		os.Exit(1)
	}
	ct, err := client.Counters(ctx)
	if err != nil {
		fmt.Println("❌ счётчики:", err)
		os.Exit(1)
	}
	fmt.Printf("✅ портал отвечает: юзер %d (admin=%v), уведомлений %d, чатов %d+%d\n",
		id, admin, ct.Notify, ct.Chat.Total, ct.Dialog.Total)
}

func acquireLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("omabitrixd уже запущен: %w", err)
	}
	return f, nil
}

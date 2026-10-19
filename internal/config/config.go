package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type DND struct {
	Start string `toml:"start"` // "23:00"
	End   string `toml:"end"`   // "08:00"
}

type Config struct {
	Portal          string   `toml:"portal"`
	WebhookUser     int      `toml:"webhook_user"`
	WebhookSecret   string   `toml:"webhook_secret"`
	UserID          int      `toml:"user_id"`
	TZ              string   `toml:"tz"`
	PollIntervalSec int      `toml:"poll_interval"`
	ToastOpen       string   `toml:"toast_open"` // tui | web — куда ведёт клик по тосту
	DND             DND      `toml:"dnd"`
	CountLines      bool     `toml:"count_lines"`
	IgnoreChats     []string `toml:"ignore_chats"`
	StatePath       string   `toml:"state_path"`
	SnapshotPath    string   `toml:"snapshot_path"`
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func DefaultPath() string { return filepath.Join(home(), ".config", "omabitrix", "config.toml") }
func DefaultStatePath() string {
	return filepath.Join(home(), ".local", "state", "omabitrix", "state.json")
}
func DefaultSnapshotPath() string {
	return filepath.Join(home(), ".local", "state", "omabitrix", "snapshot.json")
}

func Load(path string) (Config, error) {
	c := Config{PollIntervalSec: 10, StatePath: DefaultStatePath(), SnapshotPath: DefaultSnapshotPath()}
	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("читаю конфиг %s: %w (скопируй config.example.toml)", path, err)
	}
	if _, err := toml.Decode(string(data), &c); err != nil {
		return c, fmt.Errorf("парсинг %s: %w", path, err)
	}
	c.normalize()
	return c, c.Validate()
}

func (c *Config) normalize() {
	if c.ToastOpen == "" {
		c.ToastOpen = "tui" // tui | web
	}
	if c.UserID == 0 {
		c.UserID = c.WebhookUser
	}
	if c.PollIntervalSec <= 0 {
		c.PollIntervalSec = 10
	}
	if c.StatePath == "" {
		c.StatePath = DefaultStatePath()
	}
	if c.SnapshotPath == "" {
		c.SnapshotPath = DefaultSnapshotPath()
	}
	c.Portal = strings.TrimRight(c.Portal, "/")
}

func (c Config) Validate() error {
	if c.Portal == "" {
		return fmt.Errorf("portal не задан")
	}
	if c.WebhookUser <= 0 || c.WebhookSecret == "" {
		return fmt.Errorf("webhook_user / webhook_secret не заданы")
	}
	for _, w := range []struct{ name, v string }{{"dnd.start", c.DND.Start}, {"dnd.end", c.DND.End}} {
		if w.v == "" {
			continue
		}
		if _, err := time.Parse("15:04", w.v); err != nil {
			return fmt.Errorf("%s: %q не в формате HH:MM", w.name, w.v)
		}
	}
	return nil
}

func (c Config) WebhookURL() string {
	return fmt.Sprintf("%s/rest/%d/%s", c.Portal, c.WebhookUser, c.WebhookSecret)
}

func (c Config) Location() *time.Location {
	if c.TZ == "" {
		return time.Local
	}
	if l, err := time.LoadLocation(c.TZ); err == nil {
		return l
	}
	return time.Local
}

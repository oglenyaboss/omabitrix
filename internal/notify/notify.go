// Package notify — отправка нативных тостов Omarchy.
package notify

import (
	"context"
	"os/exec"
	"strconv"

	"omabitrix/internal/rules"
)

type Sender interface {
	Send(ctx context.Context, t rules.Toast) error
}

// Omarchy: omarchy-notification-send --exec omarchy-launch-webapp <url>.
// --exec принимает программу и аргументы списком — кавычки не нужны.
type Omarchy struct {
	Command string // omarchy-notification-send
	Opener  string // omarchy-launch-webapp
	Mode    string // tui | web
}

func NewOmarchy() *Omarchy {
	return &Omarchy{Command: "omarchy-notification-send", Opener: "omarchy-launch-webapp", Mode: "tui"}
}

func (o *Omarchy) Send(ctx context.Context, t rules.Toast) error {
	return run(ctx, o.Command, Args(t, o.Opener, o.Mode)...)
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return &SendError{err: err, Output: string(out)}
	}
	return nil
}

type SendError struct {
	err    error
	Output string
}

func (e *SendError) Error() string { return e.err.Error() + ": " + e.Output }

// Args — отдельно от Run ради теста: [--app-name Bitrix24 -g 󰭹 -u urgency -t ms] headline [description] [--exec ...].
// mode: "tui" — клик открывает omabitrix TUI прямо в чате/задаче; "web" — браузер.
// Явный таймаут обязателен: без него dunst висит по своим дефолтам.
func Args(t rules.Toast, opener, mode string) []string {
	urgency := t.Urgency
	if urgency == "" {
		urgency = "normal"
	}
	timeout := 8000
	if t.Urgency != "" && t.Urgency != "normal" && t.Urgency != "low" {
		timeout = 12000
	}
	args := []string{"--app-name", "Bitrix24", "-g", "󰭹", "-u", urgency, "-t", strconv.Itoa(timeout), t.Title}
	if t.Body != "" {
		args = append(args, t.Body)
	}
	if mode == "tui" && (t.DialogID != "" || t.TaskID > 0) {
		args = append(args, "--exec", "foot", "-a", "omabitrix", "omabitrix")
		if t.DialogID != "" {
			args = append(args, "--chat", t.DialogID)
		}
		if t.TaskID > 0 {
			args = append(args, "--task", strconv.FormatInt(t.TaskID, 10))
		}
		return args
	}
	if t.URL != "" {
		args = append(args, "--exec", opener, t.URL)
	}
	return args
}

// Package notify — отправка нативных тостов Omarchy.
package notify

import (
	"context"
	"os/exec"
	"strconv"
	"time"

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
	// ponytail: путь к звуку захардкожен; апгрейд — ключ sound в config.toml.
	Sound   []string         // argv плеера; пусто — без звука
	soundAt time.Time        // когда последний раз запускали звук
	now     func() time.Time // nil → time.Now, для тестов
}

func NewOmarchy() *Omarchy {
	return &Omarchy{
		Command: "omarchy-notification-send",
		Opener:  "omarchy-launch-webapp",
		Mode:    "tui",
		Sound:   []string{"pw-play", "/usr/share/sounds/freedesktop/stereo/message-new-instant.oga"},
	}
}

func (o *Omarchy) Send(ctx context.Context, t rules.Toast) error {
	o.playSound()
	return run(ctx, o.Command, Args(t, o.Opener, o.Mode)...)
}

// playSound запускает плеер в фоне: не ждёт окончания и не возвращает ошибку —
// звук best-effort, тост от него не зависит. Серия тостов за один опрос
// (2с) — один звук.
func (o *Omarchy) playSound() {
	if len(o.Sound) == 0 {
		return
	}
	nowFn := o.now
	if nowFn == nil {
		nowFn = time.Now
	}
	if !o.soundAt.IsZero() && nowFn().Sub(o.soundAt) < 2*time.Second {
		return
	}
	o.soundAt = nowFn()
	cmd := exec.Command(o.Sound[0], o.Sound[1:]...)
	if err := cmd.Start(); err != nil {
		return
	}
	go cmd.Wait()
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

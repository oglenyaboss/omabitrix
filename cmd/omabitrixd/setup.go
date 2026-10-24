// setup — онбординг одной командой: конфиг → проверка портала → бинари →
// systemd → плагин Omarchy. Идемпотентен: существующий конфиг не трогаем,
// повторный запуск обновляет бинари/юнит/плагин.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/oglenyaboss/omabitrix/internal/b24"
	"github.com/oglenyaboss/omabitrix/internal/config"
	"github.com/oglenyaboss/omabitrix/internal/setup"
)

type setupUI struct {
	in  *bufio.Scanner
	out io.Writer
}

func (u setupUI) ask(prompt, def string) string {
	if def != "" {
		prompt += " [" + def + "]"
	}
	fmt.Fprintf(u.out, "%s: ", prompt)
	if !u.in.Scan() {
		return "" // EOF — юзер прервал
	}
	v := strings.TrimSpace(u.in.Text())
	if v == "" {
		return def
	}
	return v
}

// askSecret — без эха; фолбэк на обычный ввод, если терминал не TTY.
func (u setupUI) askSecret(prompt string) string {
	fmt.Fprintf(u.out, "%s: ", prompt)
	if term.IsTerminal(uintptr(os.Stdin.Fd())) {
		b, err := term.ReadPassword(uintptr(os.Stdin.Fd()))
		fmt.Fprintln(u.out)
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	if !u.in.Scan() {
		return ""
	}
	return strings.TrimSpace(u.in.Text())
}

func (u setupUI) confirm(prompt string, yes bool) bool {
	if yes {
		return true
	}
	switch strings.ToLower(u.ask(prompt+" (y/n)", "n")) {
	case "y", "yes", "д", "да":
		return true
	}
	return false
}

func okf(u setupUI, format string, a ...any)   { fmt.Fprintf(u.out, "✅ "+format+"\n", a...) }
func skipf(u setupUI, format string, a ...any) { fmt.Fprintf(u.out, "·  "+format+"\n", a...) }

// setupMode — сценарий -setup. repoDir — каталог с plugin/ (для установки
// плагина); пусто — плагин не ставим (бинарь вне репозитория).
func setupMode(cfgPath string, yes bool, repoDir string) error {
	u := setupUI{in: bufio.NewScanner(os.Stdin), out: os.Stdout}

	// 1. конфиг
	if _, err := os.Stat(cfgPath); err == nil {
		skipf(u, "конфиг %s уже есть — оставляю", cfgPath)
	} else {
		fmt.Fprintf(u.out, "Конфиг не найден. Нужен входящий вебхук Bitrix24:\n")
		fmt.Fprintf(u.out, "  портал → Приложения → Вебхуки → Добавить → права: im, task, disk\n")
		fmt.Fprintf(u.out, "  (Чат и уведомления, Задачи, Диск — вложения ходят через Диск)\n\n")
		portal := u.ask("Портал (https://mycompany.bitrix24.ru)", "")
		userS := u.ask("ID юзера вебхука (число из URL вебхука)", "")
		secret := u.askSecret("Код вебхука (webhook_secret)")
		user, err := strconv.Atoi(userS)
		if err != nil {
			return fmt.Errorf("ID юзера — число из URL вебхука: %w", err)
		}
		if portal == "" || user <= 0 || secret == "" {
			return fmt.Errorf("не хватает данных: портал=%q user=%q", portal, userS)
		}
		portal = strings.TrimRight(portal, "/")
		if !strings.HasPrefix(portal, "http") {
			portal = "https://" + portal
		}
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(cfgPath, []byte(setup.ConfigTOML(portal, user, secret)), 0o600); err != nil {
			return err
		}
		okf(u, "конфиг записан: %s (0600)", cfgPath)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	// 2. портал отвечает
	client := b24.New(cfg.WebhookURL())
	client.RateLimit = 600 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, admin, err := client.Profile(ctx)
	if err != nil {
		return fmt.Errorf("портал не отвечает: %w (проверь вебхук и запусти setup снова)", err)
	}
	okf(u, "портал отвечает: юзер %d (admin=%v)", id, admin)

	// 3. бинари: себя и TUI рядом с собой → ~/.local/bin
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)
	binDir := filepath.Join(homeDir(), ".local", "bin")
	daemonDst := filepath.Join(binDir, "omabitrixd")
	if self != daemonDst {
		if err := copyBin(self, daemonDst); err != nil {
			return err
		}
		okf(u, "демон: %s", daemonDst)
	} else {
		skipf(u, "демон уже на месте: %s", self)
	}
	tuiSrc := filepath.Join(filepath.Dir(self), "omabitrix")
	tuiDst := filepath.Join(binDir, "omabitrix")
	if st, err := os.Stat(tuiSrc); err == nil && !st.IsDir() {
		if err := copyBin(tuiSrc, tuiDst); err != nil {
			return err
		}
		okf(u, "TUI: %s (SUPER+B после бинда)", tuiDst)
	} else if _, err := os.Stat(tuiDst); err != nil {
		skipf(u, "TUI не найден рядом с демоном — собери ./bin/omabitrix и перезапусти setup")
	} else {
		skipf(u, "TUI уже на месте: %s", tuiDst)
	}

	// 4. systemd user-юнит
	unitPath := filepath.Join(homeDir(), ".config", "systemd", "user", "omabitrix.service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(setup.SystemdUnit(daemonDst)), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", "omabitrix"}, {"restart", "omabitrix"}} {
		if out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), out)
		}
	}
	okf(u, "systemd: юнит установлен, демон запущен (status: systemctl --user status omabitrix)")

	// 5. плагин Omarchy
	omarchy := filepath.Join(homeDir(), ".config", "omarchy")
	shellJSON := filepath.Join(omarchy, "shell.json")
	pluginSrc := filepath.Join(repoDir, "plugin", setup.PluginID)
	pluginDst := filepath.Join(omarchy, "plugins", setup.PluginID)
	if repoDir == "" {
		skipf(u, "плагин: запусти setup из репозитория, чтобы поставить виджет бара")
	} else if _, err := os.Stat(omarchy); err != nil {
		skipf(u, "плагин: Omarchy не найден (%s) — пропускаю", omarchy)
	} else if u.confirm("Поставить виджет в бар Omarchy?", yes) {
		if err := setup.InstallPlugin(pluginSrc, pluginDst); err != nil {
			return err
		}
		added, err := setup.RegisterWidget(shellJSON)
		switch {
		case err != nil:
			return err
		case added:
			okf(u, "плагин: %s (зарегистрирован в shell.json, бэкап рядом)", pluginDst)
		default:
			okf(u, "плагин: %s (обновлён, в shell.json уже был)", pluginDst)
		}
	}

	fmt.Fprintf(u.out, "\nГотово. Осталось руками: хоткей SUPER+B → ~/.config/hypr/bindings.lua:\n")
	fmt.Fprintf(u.out, "  bind = { \"SUPER\", \"B\", \"exec\", \"foot -a omabitrix omabitrix\" }\n")
	fmt.Fprintf(u.out, "  и float-правило: windowrule = { \"float\", \"class:^(omabitrix)$\" }\n")
	return nil
}

func copyBin(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, st.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// ensureStdinTTY — setup интерактивный: без TTY сразу объясняем, что делать.
func ensureStdinTTY(cfgPath string) error {
	if term.IsTerminal(uintptr(os.Stdin.Fd())) {
		return nil
	}
	if _, err := os.Stat(cfgPath); err == nil {
		return nil // конфиг есть — вопросы не нужны, работаем
	}
	return fmt.Errorf("setup интерактивный: запусти в терминале (или создай %s заранее)", cfgPath)
}

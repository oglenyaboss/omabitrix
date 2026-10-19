package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	p := writeCfg(t, `
portal = "https://crm.example.ru/"
webhook_user = 75
webhook_secret = "sekret"
tz = "Asia/Yekaterinburg"
poll_interval = 5
[dnd]
start = "23:00"
end = "08:00"
ignore_chats = ["chat1"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Portal != "https://crm.example.ru" {
		t.Errorf("portal не обрезан: %q", c.Portal)
	}
	if c.UserID != 75 { // дефолт от webhook_user
		t.Errorf("user_id = %d, хочу 75", c.UserID)
	}
	if c.PollIntervalSec != 5 {
		t.Errorf("poll_interval = %d", c.PollIntervalSec)
	}
	if got := c.WebhookURL(); got != "https://crm.example.ru/rest/75/sekret" {
		t.Errorf("webhook url = %q", got)
	}
	if l := c.Location(); l.String() != "Asia/Yekaterinburg" {
		t.Errorf("tz = %v", l)
	}
}

func TestValidateMissing(t *testing.T) {
	for _, body := range []string{``, `portal = "https://x"`, `portal = "https://x"` + "\nwebhook_user = 1"} {
		if _, err := Load(writeCfg(t, body)); err == nil {
			t.Errorf("ожидал ошибку для %q", body)
		}
	}
}

func TestValidateDNFormat(t *testing.T) {
	p := writeCfg(t, `
portal = "https://x"
webhook_user = 1
webhook_secret = "s"
[dnd]
start = "25:99"
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "HH:MM") {
		t.Errorf("ожидал ошибку формата DND, got %v", err)
	}
}

package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigTOML(t *testing.T) {
	s := ConfigTOML("https://mycompany.bitrix24.ru", 7, "s3cret")
	for _, want := range []string{
		`portal = "https://mycompany.bitrix24.ru"`,
		"webhook_user = 7",
		`webhook_secret = "s3cret"`,
		`toast_open = "tui"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("нет %q в:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\t") {
		t.Error("табы в TOML")
	}
}

func TestSystemdUnit(t *testing.T) {
	u := SystemdUnit("/home/x/.local/bin/omabitrixd")
	for _, want := range []string{
		"ExecStart=/home/x/.local/bin/omabitrixd",
		"WantedBy=default.target",
		"Restart=on-failure",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("нет %q в юните", want)
		}
	}
}

func TestInstallPlugin(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "plugins", PluginID)
	for _, f := range PluginFiles {
		if err := os.WriteFile(filepath.Join(src, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := InstallPlugin(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, f := range PluginFiles {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("%s не установлен: %v", f, err)
		}
	}
	// повтор — идемпотентен
	if err := InstallPlugin(src, dst); err != nil {
		t.Fatalf("повтор: %v", err)
	}
}

func TestRegisterWidget(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell.json")
	base := `{"bar": {"left": [{"id": "omarchy.menu"}]}, "plugins": [{"id": "fkcodes.key-promoter"}], "version": 1}`
	if err := os.WriteFile(shell, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := RegisterWidget(shell)
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	var after struct {
		Plugins []struct {
			ID string `json:"id"`
		} `json:"plugins"`
		Bar json.RawMessage `json:"bar"`
	}
	data, _ := os.ReadFile(shell)
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatalf("итог не JSON: %v", err)
	}
	if len(after.Plugins) != 2 || after.Plugins[1].ID != PluginID {
		t.Fatalf("plugins: %+v", after.Plugins)
	}
	// чужие ключи прошли нетронутыми
	var bar struct {
		Left []struct {
			ID string `json:"id"`
		} `json:"left"`
	}
	if json.Unmarshal(after.Bar, &bar) != nil || len(bar.Left) != 1 || bar.Left[0].ID != "omarchy.menu" {
		t.Fatalf("bar изменился: %s", after.Bar)
	}
	// бэкап создан и равен оригиналу
	bak, _ := filepath.Glob(shell + ".bak.*")
	if len(bak) != 1 {
		t.Fatalf("бэкапы: %v", bak)
	}
	orig, _ := os.ReadFile(bak[0])
	if string(orig) != base {
		t.Fatal("бэкап не равен оригиналу")
	}

	// повтор — уже зарегистрирован, файл не трогаем
	before, _ := os.Stat(shell)
	added, err = RegisterWidget(shell)
	if err != nil || added {
		t.Fatalf("повтор: added=%v err=%v", added, err)
	}
	after2, _ := os.Stat(shell)
	if !before.ModTime().Equal(after2.ModTime()) {
		t.Fatal("повторная регистрация перезаписала файл")
	}
}

func TestRegisterWidgetNoPluginsKey(t *testing.T) {
	shell := filepath.Join(t.TempDir(), "shell.json")
	os.WriteFile(shell, []byte(`{"bar": {}}`), 0o644)
	added, err := RegisterWidget(shell)
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	data, _ := os.ReadFile(shell)
	if !strings.Contains(string(data), `"lenya.omabitrix"`) {
		t.Fatalf("плагин не добавлен: %s", data)
	}
}

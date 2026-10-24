// Package setup — онбординг `omabitrixd -setup`: генерация конфига,
// systemd-юнит, установка плагина Omarchy. Побочные эффекты разложены
// по маленьким функциям — тестируются без сети и системы.
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PluginID — идентификатор плагина Omarchy (манифест и каталог совпадают).
const PluginID = "lenya.omabitrix"

// PluginFiles — что кладём в каталог плагина (QML-панель отключена, не деплоится).
var PluginFiles = []string{"Omabitrix.qml", "manifest.json"}

// ConfigTOML — рабочий конфиг из ответов юзера: портал, вебхук и тихие
// дефолты; остальные опции — в config.example.toml как справочник.
func ConfigTOML(portal string, user int, secret string) string {
	return fmt.Sprintf(`# omabitrix — конфиг (создан omabitrixd -setup)
# Все опции и комментарии: config.example.toml в репозитории.

portal = %q

# Входящий вебхук: портал → Приложения → Вебхуки (права: im, task, disk)
webhook_user = %d
webhook_secret = %q

poll_interval = 10
toast_open = "tui"
`, portal, user, secret)
}

// SystemdUnit — user-юнит демона; binPath — абсолютный путь omabitrixd.
func SystemdUnit(binPath string) string {
	return fmt.Sprintf(`[Unit]
Description=omabitrix — Bitrix24-компаньон: тосты, счётчики бара, TUI

[Service]
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, binPath)
}

// InstallPlugin — копирует файлы плагина из srcDir репозитория в dst
// (~/.config/omarchy/plugins/<id>). Перезапись безопасна: setup идемпотентен.
func InstallPlugin(srcDir, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range PluginFiles {
		data, err := os.ReadFile(filepath.Join(srcDir, f))
		if err != nil {
			return fmt.Errorf("плагин: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RegisterWidget — добавить {"id": PluginID} в plugins[] shell.json, если
// его нет. Значения остальных ключей проходят байт-в-байт (RawMessage),
// меняются только форматирование и порядок ключей. Перед правкой — бэкап
// shell.json.bak.<unix>, запись атомарная.
func RegisterWidget(shellJSON string) (added bool, err error) {
	data, err := os.ReadFile(shellJSON)
	if err != nil {
		return false, err
	}
	// разбор в два прохода: plugins отдельно, остальное — как есть
	var full map[string]json.RawMessage
	if err := json.Unmarshal(data, &full); err != nil {
		return false, fmt.Errorf("shell.json: %w", err)
	}
	for _, p := range rawList(full["plugins"]) {
		var v struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(p, &v) == nil && v.ID == PluginID {
			return false, nil // уже зарегистрирован
		}
	}
	ids, err := json.Marshal(append(rawList(full["plugins"]), json.RawMessage(`{"id":"`+PluginID+`"}`)))
	if err != nil {
		return false, err
	}
	full["plugins"] = ids
	out, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return false, err
	}
	bak := fmt.Sprintf("%s.bak.%d", shellJSON, time.Now().Unix())
	if err := os.WriteFile(bak, data, 0o644); err != nil {
		return false, fmt.Errorf("бэкап shell.json: %w", err)
	}
	return true, saveAtomic(shellJSON, out)
}

// rawList — распаковать массив RawMessage (nil при отсутствии ключа).
func rawList(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	return list
}

func saveAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

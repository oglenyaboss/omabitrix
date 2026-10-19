# omabitrix

Компаньон Bitrix24 для Omarchy: задачи и чаты портала — в нативных тостах, счётчиках бара и **TUI-клиенте** (SUPER+B). Демон прокидывает события на десктоп, TUI закрывает чаты/задачи/уведы не выходя из терминала.

Спека: [docs/SPEC.md](docs/SPEC.md) · Ресёрч: [docs/research/](docs/research/)

## Состав

| Кусок | Что делает |
|---|---|
| `cmd/omabitrixd` | Go-демон: один входящий вебхук, polling-цикл (batch ≤50 команд раз в 10 с), правила тостов с идемпотентностью, пишет снапшот для бара |
| `cmd/omabitrix` | TUI (bubbletea): вкладки чаты/задачи/уведы, ответы в чатах, смена статусов задач, сплэш с ASCII-арто́м; запуск `SUPER+B` (foot тайлом) или кликом по иконке |
| `plugin/lenya.omabitrix` | QML-плагин бара Omarchy: одна иконка 󰭹 с бейджем непрочитанных (клик → TUI, ПКМ → портал), 󰅚 при смерти демона. Исходники выключенной QML-панели лежат рядом, не деплоятся |
| `internal/*` | `b24` (REST-клиент), `rules` (классификация/DND/идемпотентность), `state` (атомарный state+snapshot), `notify` (omarchy-notification-send), `daemon` (цикл) |

## Установка

```bash
go build -o bin/omabitrixd ./cmd/omabitrixd
cp config.example.toml ~/.config/omabitrix/config.toml  # заполнить вебхук
./bin/omabitrixd -doctor                                  # проверка портала
mkdir -p ~/.config/systemd/user
# юнит: ExecStart=%h/.../bin/omabitrixd, WantedBy=default.target
systemctl --user enable --now omabitrix
# плагин: только виджет и манифест (QML-панель отключена — её файлы не деплоить)
mkdir -p ~/.config/omarchy/plugins/lenya.omabitrix
cp plugin/lenya.omabitrix/{Omabitrix.qml,manifest.json} ~/.config/omarchy/plugins/lenya.omabitrix/
# и добавить {"id": "lenya.omabitrix"} в shell.json
install -m 755 bin/omabitrix ~/.local/bin/omabitrix          # TUI на PATH
# SUPER+B (бинд) и float-правило: ~/.config/hypr/bindings.lua (пример в docs/SPEC.md)
```

## Проверка

```bash
go test ./...
systemctl --user status omabitrix
cat ~/.local/state/omabitrix/snapshot.json
```

Тост: чужое сообщение/упоминание/комментарий в твоей задаче/дедлайн ≤1ч/просрочка (раз в сутки). Клик по тосту — карточка. Ночь (DND) — тишина, всё в счётчике.

## Известные потолки (v0)

- Задержка = интервал опроса; push (исходящий вебхук/OAuth) — в бэклоге.
- Комментарии realtime через `im.v2` (очередь живёт 24 ч); catch-up — по `newCommentsCount`.
- Один юзер на портал (вебхук = его права); очередь im.v2 эксклюзивна на юзера.
- Телефония, кнопки в тостах, реакции/файлы в TUI — бэклог.

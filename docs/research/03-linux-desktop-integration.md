# Исследование: десктоп-интеграция Bitrix24 на Omarchy (Arch + Hyprland + Wayland)

> Обзор подходов к интеграции веб-сервиса (Bitrix24 CRM) с десктопом Omarchy: уведомления, трей, «клик по уведомлению → карточка», click-to-call. Кода ещё нет — это выбор архитектуры.
> Дата исследования: 2026. Все ключевые утверждения — с URL. Неподтверждённое помечено явно.

## Summary

Omarchy 4.x не использует ни dunst, ни mako: нотификации рендерит встроенный в shell (Quickshell) демон `omarchy.notifications`, который умеет клик по тосту (exec-hint / default action / фокус окна по классу), историю (10 записей) и DND. Waybar в Omarchy уже несёт tray (в раскрывающейся группе) и custom-модули с сигналами — счётчик Bitrix24 впишется без переделки бар. Рекомендованный путь: **(1)** старт с PWA-обёртки (Chromium `--app` через `omarchy-launch-webapp`) + фоновый демон с systemd user unit, который поллит REST/Push&Pull, шлёт уведомления и обновляет счётчик в waybar; **(2)** при потребности в глубокой интеграции (свой трей, свои действия, deep-link из коробки) — полноценная обёртка Electron (паттерн teams-for-linux) или Tauri v2 (меньше вес, но WebKitGTK-нюансы на Wayland). Нюанс: официальная deep-link-схема Bitrix24 по докам — `bx://`, а не `bx24://` из ТЗ — требует фактической проверки.

---

## 1. Omarchy из коробки

1. **Нотификатор — встроенный в shell, не dunst и не mako.** Начиная с Omarchy 4.0.0 «Quattro» демон уведомлений — плагин `omarchy.notifications` внутри Quickshell-shell; он заявляет `org.freedesktop.Notifications` на сессионной шине, и всё, что говорит по freedesktop-протоколу (notify-send, libnotify, Chromium web-apps), попадает в тосты shell. «There is no dunst or mako» — прямая цитата из docs/notifications.md.
   **Sources:** [docs/notifications.md (basecamp/omarchy)](https://github.com/basecamp/omarchy/blob/fa955bfa/docs/notifications.md), [omarchylinux.org — Notifications not showing](https://omarchylinux.org/fix/notifications-not-showing/). **Support:** direct evidence. **Confidence:** high.
   - В Omarchy 3.x нотификатором был **mako** (в мануале Omarchy 3 и в default-конфиге репозитория: `default/mako/core.ini` с `default-timeout=5000`, группировкой и DND-модой). [The Omarchy 3 Manual — Common tweaks](https://learn.omacom.io/2/the-omarchy-manual/102/common-tweaks), [default/mako/core.ini](https://github.com/basecamp/omarchy/blob/df4ffe99/default/mako/core.ini). **Support:** direct evidence. **Confidence:** high. Dunst в Omarchy не используется ни в одной версии (не найдено упоминаний в дефолтах).
2. **Waybar: tray и custom-модули есть из коробки.** В дефолтном `config.jsonc` справа стоит `group/tray-expander` → раскрывающаяся группа с модулем `tray` (`icon-size: 12`); в центре — пачка custom-модулей (`custom/update` с `signal: 7` и `interval: 21600`, `custom/weather` с `return-type: json`, индикаторы DND/idle/скринрекордера с `signal` и JSON-выдачей). То есть и трей, и паттерн «скрипт + счётчик + клик» — родные для Omarchy.
   **Sources:** [config/waybar/config.jsonc (basecamp/omarchy, dev)](https://github.com/basecamp/omarchy/blob/dev/config/waybar/config.jsonc), [waybar-tray(5)](https://man.archlinux.org/man/extra/waybar/waybar-tray.5.en), [waybar-custom(5)](https://man.archlinux.org/man/extra/waybar/waybar-custom.5.en). **Support:** direct evidence. **Confidence:** high.
   - Механика custom-модуля: `exec` + `interval`/`signal` (обновление через `pkill -RTMIN+N waybar`) + `return-type: json` (`{"text","tooltip","class","percentage"}`) + `on-click`. **Sources:** [waybar-custom(5)](https://man.archlinux.org/man/extra/waybar/waybar-custom.5.en). **Support:** direct evidence. **Confidence:** high.
3. **Hyprland window rules.** Правила задаются `windowrule` / `windowrulev2` с regex-матчингом по `class`, `title` и др.; правила типа `float`, `pin`, `workspace`, `size` применимы к окнам web-app по их class (для Chromium --app класс содержит id приложения).
   **Sources:** [Hyprland Wiki — Window Rules](https://wiki.hyprland.org/hyprland-wiki/pages/Configuring/Window-Rules/). **Support:** direct evidence (синтаксис); конкретные правила для web-app в текущем Omarchy — **не проверял**. **Confidence:** high (синтаксис) / n/a (дефолтные правила).
4. **Web-apps — штатный механизм Omarchy.** Меню «Install > Web App» создаёт лаунчер, который запускает URL в отдельном безрамочном окне через `omarchy-launch-webapp`; тот резолвит дефолтный браузер (Chromium/Chrome/Brave/Edge/Opera/Vivaldi, fallback `chromium.desktop`) и запускает `<browser> --app=<URL>` через `uwsm-app`, прокидывая доп. аргументы (например `--user-data-dir=...` для изоляции рабочего профиля). Omarchy уже поставляет десятки web-app'ов (WhatsApp, Basecamp, HEY, Zoom, Discord, Google-сервисы, Microsoft 365), т.е. обёртка «сайт как приложение» — первый класс гражданства в этой системе.
   **Sources:** [manual/25-web-apps.md (quattro)](https://github.com/omacom/omarchy/blob/quattro/manual/25-web-apps.md), [bin/omarchy-launch-webapp](https://github.com/basecamp/omarchy/blob/dev/bin/omarchy-launch-webapp), [omarchy.org/manual/guis](https://omarchy.org/manual/guis/). **Support:** direct evidence. **Confidence:** high.
   - Известный дефект лаунчера: при браузере вне allowlist тихо падает на `chromium.desktop` и может failing молча ([issue #11225](https://github.com/omacom/omarchy/issues/11225), [issue #14163](https://github.com/omacom/omarchy/issues/14163)). **Support:** direct evidence. **Confidence:** high.
5. **Хелпер фокуса окна по классу.** `omarchy-hyprland-focus-app` (case-insensitive матч по классу Hyprland) используется самим нотификатором как fallback при клике на тост — готовый кирпичик для «клик → карточка».
   **Source:** [shell/plugins/notifications/Service.qml](https://raw.githubusercontent.com/omacom/omarchy/quattro/shell/plugins/notifications/Service.qml). **Support:** direct evidence. **Confidence:** high.

> Примечание: `wiki.omarchy.org` при проверке не открылся (ошибка сертификата); данные взяты из omarchy.org/manual, GitHub-репозитория (basecamp/omarchy → omacom/omarchy) и omarchylinux.org.

---

## 2. Уведомления с действиями

1. **Время жизни тостов в Omarchy 4:** low ≥5 c, normal ≥8 c, critical — пока не закроешь; запрошенный `expire_timeout` учитывается, но максимум 30 c; hover ставит таймер на паузу; `replaces_id` обновляет тост на месте. Клик слева вызывает действие, правый клик — закрывает. Каждый тост пишется в `~/.local/state/omarchy/notifications/` (история — последние 10), тосты переживают рестарт shell (а `omarchy-update` его рестартует).
   **Sources:** [docs/notifications.md](https://github.com/basecamp/omarchy/blob/fa955bfa/docs/notifications.md), [Service.qml](https://raw.githubusercontent.com/omacom/omarchy/quattro/shell/plugins/notifications/Service.qml). **Support:** direct evidence. **Confidence:** high.
2. **Клик по тосту (Omarchy 4):** приоритет такой — (а) hint `omarchy-exec` (`omarchy-notification-send --exec '<команда>'`) — shell сам исполняет команду detached, переживает рестарт shell и восстановление тоста из истории; (б) libnotify-action с идентификатором `default`, пока отправитель жив; (в) fallback — фокус окна отправителя по классу через `omarchy-hyprland-focus-app`. Для «клик → открыть карточку» достаточно любого из трёх.
   **Sources:** [docs/notifications.md](https://github.com/basecamp/omarchy/blob/fa955bfa/docs/notifications.md), [Service.qml — `invokePopupDefault`](https://raw.githubusercontent.com/omacom/omarchy/quattro/shell/plugins/notifications/Service.qml). **Support:** direct evidence. **Confidence:** high.
   - Пример: Omarchy использует этот контракт для crash-уведомлений (`--exec` открывает `omarchy-agent-crash`) и миграций. **Support:** direct evidence. **Confidence:** high.
3. **Кнопки действий (actions) сторонних приложений в Omarchy 4:** Quickshell в принципе умеет объявлять поддержку actions/inline-reply ([server.hpp](https://github.com/quickshell-mirror/quickshell/blob/783c9539/src/services/notifications/server.hpp)), сервис Omarchy мапит список actions ([Service.qml](https://raw.githubusercontent.com/omacom/omarchy/quattro/shell/plugins/notifications/Service.qml)), и тикет [#8346](https://github.com/basecamp/omarchy/issues/8346) показывает существующую кликабельную action-кнопку («Click to diagnose with AI»). Но **отрисовка произвольных (не default) кнопок libnotify для сторонних отправителей — не подтверждена**; в доступной части Service.qml явно отрабатываются только `default` и omarchy-exec. Требует проверки на живой системе. **Confidence:** low (по этой части).
4. **Reply-кнопки (inline reply):** в Omarchy shell — не подтверждено (скорее всего нет); **mako — нет** (нет inline-reply UI; действия — через `on-button-*`/`makoctl invoke|menu`); **dunst — нет**; **SwayNotificationCenter — да**, поддерживает inline replies, но с открытым багом: текст ответа не доезжает по D-Bus ([issue #551, открыт с 2025-05](https://github.com/ErikReider/SwayNotificationCenter/issues/551)).
   **Sources:** [mako(5)](https://man.archlinux.org/man/mako.5), [SwayNotificationCenter README](https://github.com/ErikReider/SwayNotificationCenter/blob/main/README.md), [dunst docs](https://dunst-project.org/documentation/). **Support:** direct evidence. **Confidence:** high (mako/dunst/swaync), medium (Omarchy shell — отсутствие reply не подтверждено напрямую, но в документации и коде сервиса reply не упоминается).
5. **dunst (если кто-то поставит сам):** клик по тосту **не** вызывает действие — действия и URL открываются через context menu (`dunstctl context`, дефолтный хоткей Ctrl+Shift+Period, меню через dmenu). Это слабейший вариант под «клик → карточка».
   **Sources:** [dunst docs](https://dunst-project.org/documentation/), [dunst issue #786](https://github.com/dunst-project/dunst/issues/786), [issue #163](https://github.com/dunst-project/dunst/issues/163). **Support:** direct evidence. **Confidence:** high.
6. **mako (актуально для Omarchy 3.x):** ЛКМ по умолчанию `invoke-default-action`; именованные действия — `on-button-left=invoke-action <name>` (с xdg-activation для фокуса) или `makoctl menu` с rofi/wmenu; `actions=1` по умолчанию; `default-timeout=0` = без таймаута (в дефолтном конфиге Omarchy 3 было 5000 мс); inline-reply нет.
   **Sources:** [mako(5)](https://man.archlinux.org/man/mako.5), [default/mako/core.ini](https://github.com/basecamp/omarchy/blob/df4ffe99/default/mako/core.ini). **Support:** direct evidence. **Confidence:** high.
7. **Практический вывод для Bitrix24:** отправлять уведомления из своего кода через `omarchy-notification-send --exec 'omarchy-launch-webapp <url карточки>'` — клик откроет/сфокусирует нужное окно, поведение переживает рестарты shell. Для Chromium-пушей напрямую из web-клиента клик тоже попадает в shell (Chromium шлёт freedesktop-нотификации), но семантика «какой URL откроется» — зависит от обработчика `notificationclick` на стороне сайта; **на практике не проверено**.
   **Support:** researcher inference на основе пп. 1–3 и docs/notifications.md. **Confidence:** medium.

---

## 3. Способы обёртки (сравнение)

| Подход | Вес/память | Wayland | Уведомления/трей | Deep-link (`bx…://`) | Зрелость на Arch |
|---|---|---|---|---|---|
| **PWA / Chromium `--app`** | 0 байт кода; память = экземпляр браузера (свой при отдельном `--user-data-dir`, общий — дешевле) | нативно (Chromium на Wayland — Ozon) | веб-пуши → freedesktop → shell Omarchy; своего трея нет | только `web+*`-схемы через manifest `protocol_handlers`; `bx24:`/`bx:` нельзя — нужен ОС-хендлер | родной для Omarchy (`omarchy-launch-webapp`, «Install > Web App») |
| **Electron** | бандл ~244 MiB (замер-образец 2025); RAM ~409 MB на 6 окон (macOS-тест); Ubuntu USS/PSS ~118/207 MB | нативно (Chromium/Ozone; флаги `--ozone-platform-hint=auto`) | полный контроль: свои actions, tray через SNI/AppIndicator | через `.desktop` с `MimeType=x-scheme-handler/...` (API Electron на Linux не регистрирует) | огромная; паттерн teams-for-linux, AUR-гайдлайн по системному Electron |
| **Tauri v2** | бандл ~8.6 MiB (тот же замер-образец); RAM ~172 MB на 6 окон (macOS-тест); на Ubuntu разница с Electron почти нулевая (USS 125 vs 118 MB) | работает, но WebKitGTK-нюансы: NVIDIA+DMABUF (blank window/`Error 71`, лечится `WEBKIT_DISABLE_DMABUF_RENDERER=1`), ресайз/блюр в Hyprland ([wry#1727](https://github.com/tauri-apps/wry/issues/1727)) | notification через плагин; tray-icon (libappindicator) | официальный плагин deep-link; известный баг бандлера: `Exec` без `%u` → ссылка молча теряется ([#15928](https://github.com/tauri-apps/tauri/issues/15928)) | хорошая, но Linux-крайние случаи надо тестировать на Hyprland |
| **WebKitGTK напрямую (C/Python)** | меньше всего (системный webkit2gtk 4.1) | работает, но те же DMABUF/NVIDIA риски, свой слой за всё | всё руками | руками через .desktop | живых примеров всё меньше: WasIstLos (WhatsApp) **ARCHIVED**; активнее делают на Qt WebEngine |

**Числа и источники по пунктам таблицы:**

- Chromium `--app`: один `--user-data-dir` = один процесс-экземпляр; при том же профиле Chromium переиспользует процессы. Фиксированного «MB на приложение» нет, суммировать private memory процессов некорректно (shared libs). **Sources:** [chromium manpage](https://chromium.googlesource.com/chromium/chromium/%2B/refs/heads/main/chrome/app/resources/manpage.1.in), [Process Model and Site Isolation](https://chromium.googlesource.com/chromium/src/%2B/main/docs/process_model_and_site_isolation.md), [Memory Usage Backgrounder](https://www.chromium.org/developers/memory-usage-backgrounder/). **Support:** direct evidence. **Confidence:** high.
- Electron vs Tauri (2025-замер-образец): 244 MiB vs 8.6 MiB бандл; ~409 MB vs ~172 MB RAM на 6 окон. **Source:** [gethopp.app/blog/tauri-vs-electron](https://www.gethopp.app/blog/tauri-vs-electron). **Support:** direct evidence (один замер, не универсалия). **Confidence:** medium.
- Контр-замер на Ubuntu: Electron 118 MB USS / 207 MB PSS против Tauri 125/185 — почти паритет; сами мейнтейнеры Tauri называют свои бенчмарки smoke-тестами. **Sources:** [tauri#5889](https://github.com/tauri-apps/tauri/issues/5889), [tauri benchmark_results](https://tauri-apps.github.io/benchmark_results/). **Support:** direct evidence. **Confidence:** medium.
- Tauri на Linux: системный WebKitGTK (API 4.1), риски Wayland описаны официально (NVIDIA DMABUF, blank windows, software rendering fallback). **Sources:** [v2.tauri.app/start/prerequisites](https://v2.tauri.app/start/prerequisites/), [Linux Graphics Issues](https://v2.tauri.app/develop/debug/linux-graphics/). **Support:** direct evidence. **Confidence:** high.
- AppImage от Tauri до bundler 2.10.0 мог форсить `GDK_BACKEND=x11` (XWayland). **Source:** [tauri-bundler v2.10.0](https://v2.tauri.app/release/tauri-bundler/v2.10.0/). **Support:** direct evidence. **Confidence:** high.
- Трей: Electron на Linux использует StatusNotifierItem (SNI), fallback на legacy GTK status icon (X11-only); нужен SNI-host в панели — waybar его имеет. **Sources:** [electron tray docs](https://github.com/electron/electron/blob/main/docs/api/tray.md), [waybar-tray(5)](https://man.archlinux.org/man/extra/waybar/waybar-tray.5.en). **Support:** direct evidence. **Confidence:** high.
- **Живые проекты-обёртки (мессенджеры/CRM):**
  - [teams-for-linux](https://github.com/ismaelmartinez/teams-for-linux) — Electron-обёртка Teams, ~5.1k звёзд, активна; пакеты tarball/deb/rpm/**pacman**; [AUR: teams-for-linux](https://aur.archlinux.org/packages/teams-for-linux). Лучший референс по фичам интеграции (трей, deep-link, кастомные нотификации). **Confidence:** high.
  - [ZapZap](https://github.com/rafatosta/zapzap) — WhatsApp Web-клиент на **PyQt6 + QtWebEngine**, ~1.6k звёзд, активен (релизы 7.x). Пример «лёгкой» обёртки на Qt, не Electron. **Confidence:** high.
  - [WasIstLos/whatsapp-for-linux](https://github.com/xeco23/WasIstLos) — WebKitGTK+gtkmm, портирован на webkit2gtk 4.1, но **ARCHIVED** — показателен как анти-пример жизнеспособности «своего WebKitGTK-клиента». **Confidence:** high.
  - Специфично для Bitrix24: [buglloc/brick](https://github.com/buglloc/brick/) (открытый клиент, ~117 звёзд) — **ARCHIVED**, [smallcms/infinitrix](https://github.com/smallcms/infinitrix) (PyQt5+PyQtWebEngine) — последний коммит 2023. Живой open-source-обёртки Bitrix24 **не найдено**. **Confidence:** high (по состоянию поиска).
- **Официальный клиент Bitrix24 Desktop теперь есть и для Linux** (функции как на Win/macOS: чаты, звонки, задачи, диск, двухоконный режим), пакеты **DEB/RPM** (Ubuntu 20–24, Mint, Astra; Fedora, ALT, ROSA, openSUSE). Arch не заявлен в официальном списке, но **AUR-пакет есть**: [AUR bitrix24](https://aur.archlinux.org/packages/bitrix24) (23.0.32.91-2, перепаковка официального DEB, поддерживается, обновлён 2026-07 — проверено через AUR RPC; детали в файле 01, раздел 2.1); snap `bitrix` (beta) не обновлялся с 2022.
  **Sources:** [Приложение Битрикс24 для Linux (helpdesk)](https://helpdesk.bitrix24.ru/open/16171806/), [bitrix24.com/apps/desktop.php](https://www.bitrix24.com/apps/desktop.php), [snapcraft.io/install/bitrix/arch](https://snapcraft.io/install/bitrix/arch). **Support:** direct evidence. **Confidence:** high. (Дата на странице helpdesk — июль 2026, свежо.)
  - Как это ведёт себя на Hyprland/Wayland (XWayland? трей? клик по уведомлению?) — **не проверено**, требует живого теста.

---

## 4. Фоновый демон + waybar (лёгкая альтернатива)

**Схема:** systemd **user** unit (`systemctl --user enable --now b24-daemon.service`) → демон на Python/Go/Rust опрашивает Bitrix24 REST (вебхук-токен) или держит долгое соединение → при событиях шлёт `notify-send` / `omarchy-notification-send --exec …` → параллельно пишет счётчик непрочитанного для waybar custom-модуля. **Sources:** [Arch Wiki — systemd/User](https://wiki.archlinux.org/title/Systemd/User). **Support:** standard practice / interpretation. **Confidence:** high.

**Данные от Bitrix24 — два режима:**

1. **Поллинг REST через вебхук** — просто и без регистрации приложения; минус — задержка равна интервалу опроса и тратится rate limit. Вебхуки: [bitrix24.com/apps/webhooks.php](https://www.bitrix24.com/apps/webhooks.php). **Confidence:** high.
2. **Push&Pull (real-time)** — официальный протокол кастомных клиентов: серверная часть публикует событие через `pull.application.event.add`, клиент читает канал по **websocket или long polling** (`wss://rtc-cloud-ms1.bitrix.info/subws/?CHANNEL_ID=...&format=json`). Официально заявлено применение: «desktop application» и «нотификация пользователя вне интерфейса Bitrix24» (`pull.application.push.add`). **Критическое ограничение: вебхук не создаёт контекст** — нужен установленный «приложение»-интеграция с OAuth-токеном и scope `pull` (`pull.application.config.get`). Обязательные части клиента: обработка сервис-команд (`channel_expire`, `config_expire`, `server_restart`), reconnect-backoff (0.5c → 60c + джиттер), правило «нет сообщений 20 c = реконнект», история через `mid`/`tag`.
   **Source:** [b24restdocs — Custom Push&Pull Client](https://github.com/bitrix24/b24restdocs/blob/main/settings/interactivity/custom-push-and-pull-client.md). **Support:** direct evidence. **Confidence:** high.

**Счётчик в waybar:** custom-модуль с `exec` → JSON `{"text": "3", "tooltip": "...", "class": "urgent"}`, обновление либо по `interval`/`signal`, либо демоном через `pkill -RTMIN+8 waybar`; `on-click` — сфокусировать окно (`omarchy-hyprland-focus-app`) или открыть веб-клиент. Прямой образец — уже существующие в Omarchy bar-модули с тем же паттерном (`custom/update`, индикаторы с signal). **Sources:** [waybar-custom(5)](https://man.archlinux.org/man/extra/waybar/waybar-custom.5.en), [config.jsonc](https://github.com/basecamp/omarchy/blob/dev/config/waybar/config.jsonc). **Support:** direct evidence. **Confidence:** high.

**Примеры реальных проектов такого класса (поллер + нотификации + бар-счётчик):**

- [lfrancke/gh-notifier](https://github.com/lfrancke/gh-notifier/) — демон GitHub-уведомлений, D-Bus-нотификации, кнопка «Open» открывает issue/PR; группировка обновлений по репозиторию.
- [bnema/gh-notify](https://github.com/bnema/gh-notify) — CLI-монитор GitHub-нотификаций с десктоп-алертами и systemd-сервисом.
- [benelan/gh-notify-desktop](https://github.com/benelan/gh-notify-desktop/blob/main/gh-notify-desktop) — bash-скрипт под systemd timer/cron, уважает `X-Poll-Interval` и `Last-Modified` API.
- [kouiso/github-notify](https://github.com/kouiso/github-notify) — Tauri-приложение: фоновый поллинг + tray + счётчик непрочитанных (гибридный вариант).
- Счётчики для бара: [mizzunet/waybar-unread-email](https://github.com/mizzunet/waybar-unread-email) (Go), [crabvk/bar-gmail](https://github.com/crabvk/bar-gmail) (есть в AUR), [hoblin/thunderbird-waybar-email](https://github.com/hoblin/thunderbird-waybar-email).
- **Прямого аналога «демон Bitrix24 → notify-send + waybar» не найдено** — ниша свободна. **Confidence:** high (по состоянию поиска).

**Оценка ресурсов:** демон без WebView — порядка единиц-десятков МБ RAM (Go/Rust — единицы; Python — десятки); точные цифры зависят от реализации — это **researcher inference**, не измеренное значение.

---

## 5. Протокол-хендлер (x-scheme-handler)

**Важная поправка к ТЗ:** официальная документация Bitrix24 описывает deep-link-схему десктоп-приложения как **`bx://`** (пример `bx://v2/{portal}/botContext/dialogId/{dialogId}/context/{...}`); схема `bitrix24://` лишь выводит приложение на передний план. Упоминания `bx24://` в официальной документации не найдено — имя схемы из ТЗ **не подтверждено**, сверьте с реальным приложением (см. «Не подтверждено»).
**Sources:** [apidocs.bitrix24.com — bot-context](https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html). **Support:** direct evidence. **Confidence:** high.

1. **Регистрация произвольной схемы на уровне ОС** (работает для любой обёртки и даже для «фейкового» хендлера-скрипта):
   - `.desktop` с `Type=Application`, `Exec=… %u` ( `%u` обязателен — иначе URL теряется) и `MimeType=x-scheme-handler/bx24;`
   - `xdg-mime default bx24-handler.desktop x-scheme-handler/bx24` (+ `update-desktop-database ~/.local/share/applications` при ручной установке);
   - проверка: `xdg-mime query default x-scheme-handler/bx24`, тест `xdg-open 'bx24://…'`.
   **Sources:** [unix.SE — Create a custom URL Protocol Handler](https://unix.stackexchange.com/questions/497146/create-a-custom-url-protocol-handler), [blog.mggross.com — Extending XDG URI scheme handlers](https://blog.mggross.com/extending-scheme-handlers/), [blog.za3k.com — URI handlers](https://blog.za3k.com/url-handlers-in-linux/), [Arch Wiki — Desktop entry](https://wiki.archlinux.org/title/Desktop_entry). **Support:** direct evidence. **Confidence:** high.
   - Хендлером может быть и скрипт-транслятор: `bx24://…`/`bx://…` → https-URL карточки → `omarchy-launch-webapp "<url>"`. Это позволяет ловить ссылки Bitrix24, даже если обёртка — чистый PWA.
2. **Chromium PWA — произвольные схемы зарегистрировать нельзя.** Manifest `protocol_handlers` (Chrome 96+) принимает только safelist-схемы (`mailto`, `magnet`, …) и кастомные с префиксом **`web+`**; `bx24:`/`bx:` — нельзя. Т.е. для чистой PWA-обёртки deep-link работает только через ОС-хендлер (п. 1), а не через манифест.
   **Sources:** [developer.chrome.com — URL protocol handler registration for PWAs](https://developer.chrome.com/docs/web-platform/best-practices/url-protocol-handler), [New in Chrome 96](https://developer.chrome.com/blog/new-in-chrome-96/), [Chromium OS integration docs](https://chromium.googlesource.com/chromium/src/%2B/fd2cb3f55a752f3d452d5f7c619dd20ff37f1f66/chrome/browser/web_applications/docs/os_integration.md). **Support:** direct evidence. **Confidence:** high.
3. **Electron:** `app.setAsDefaultProtocolClient` на Linux фактически не работает (AppImage/пакеты) — регистрация делается инсталлятором: `.desktop` с `MimeType=x-scheme-handler/…` + `update-desktop-database` (electron-builder умеет через `linux.desktop`). Внутри приложения deep-link приходит как аргумент командной строки второго инстанса — нужен single-instance lock.
   **Sources:** [electron/electron#6440](https://github.com/electron/electron/issues/6440), [electron-builder#4035](https://github.com/electron-userland/electron-builder/issues/4035), [electron.build — Linux](https://www.electron.build/docs/linux/), [electron docs app](https://electronjs.org/docs/latest/api/app). **Support:** direct evidence. **Confidence:** high.
4. **Tauri v2:** плагин [deep-link](https://v2.tauri.app/plugin/deep-linking/) (`desktop.schemes: ["bx24"]`, `register_all()` на Linux), в паре с `tauri-plugin-single-instance` (deep-link feature). Известный баг: сгенерированный `.desktop` может объявить `MimeType`, но забыть `%u` в `Exec` — ссылки молча теряются ([tauri#15928](https://github.com/tauri-apps/tauri/issues/15928)); лечится кастомным шаблоном desktop-файла (`Exec={{exec}} %u`).
   **Support:** direct evidence. **Confidence:** high.
5. **Click-to-call:** «родной» путь Bitrix24 — не URI-схема, а REST-телефония (`telephony.externalCall.register` и др.); в веб-интерфейсе звонки идут через встроенный WebRTC (микрофон браузера). Для обёртки это значит: click-to-call из карточки в Chromium --app работает «сам», вопрос только в разрешении микрофона для origin.
   **Sources:** [telephony.externalCall.register](https://apidocs.bitrix24.com/api-reference/telephony/telephony-external-call-register.html). **Support:** direct evidence (REST), interpretation (поведение в --app окне не проверено). **Confidence:** medium.

---

## 6. Пакетирование

1. **AUR/PKGBUILD** — канонический путь для Arch:
   - Общие правила: [Arch package guidelines](https://wiki.archlinux.org/title/Arch_package_guidelines) (имена строчными, `pkgver`/`pkgrel`, каталоги `/usr/bin`, `/usr/lib/<pkg>`, `/opt/<pkg>` для крупных самодостаточных, `optdepends` для опциональных, namcap для проверки).
   - Для Electron — [Electron package guidelines](https://wiki.archlinux.org/title/Electron_package_guidelines): использовать **системный electron** (`depends=('electron')` + wrapper `exec /usr/bin/electron<ver> /usr/lib/<appname>/`), либо для bin-пакетов electron-builder-сборку класть в `/opt/<pkg>`; настройки `electronDist`/`electronVersion` для сборки против системного Electron.
   - Реальные примеры: [AUR teams-for-linux](https://aur.archlinux.org/packages/teams-for-linux) (сборка) и [teams-for-linux-electron-bin](https://aur.archlinux.org/packages/teams-for-linux-electron-bin) (бинарь). **Support:** direct evidence. **Confidence:** high.
2. **Бинарный релиз (GitHub Releases)** — самый быстрый путь до пользователя до AUR: tarball/deb/rpm/pacman-пакет (так делает teams-for-linux). Для Tauri — `tauri bundler` даёт deb/rpm/AppImage (AppImage корректно работает на Wayland начиная с bundler 2.10.0).
   **Sources:** [teams-for-linux releases/README](https://github.com/ismaelmartinez/teams-for-linux), [tauri-bundler 2.10.0](https://v2.tauri.app/release/tauri-bundler/v2.10.0/). **Support:** direct evidence. **Confidence:** high.
3. **PWA-подход паковать почти нечего:** `.desktop` + иконка + (опционально) скрипт-хендлер схемы; это либо несколько строк в PKGBUILD, либо штатное «Install > Web App» Omarchy (вообще без пакета). Демон ставится как `/usr/lib/systemd/user/<name>.service` + `systemctl --user enable`. **Support:** interpretation на основе [[Arch package guidelines]](https://wiki.archlinux.org/title/Arch_package_guidelines) и [[25-web-apps.md]](https://github.com/omacom/omarchy/blob/quattro/manual/25-web-apps.md). **Confidence:** high.
4. **Официальный Bitrix24 DEB/RPM** на Arch напрямую не ставится (pacman не умеет deb); варианты — переупаковка в PKGBUILD (deb→tarball) или snap (устаревший). **Support:** interpretation. **Confidence:** medium.

---

## Вывод: рекомендованные 2–3 подхода

**Подход A — «Omarchy-native минимум» (рекомендую как старт, 1–2 дня работы):**
Chromium `--app` через `omarchy-launch-webapp "https://<portal>.bitrix24.ru/" --user-data-dir=~/.local/share/bx24-profile` (свой `.desktop` с иконкой) + фоновый демон (Go или Python) в systemd user unit:
- уведомления — `omarchy-notification-send --exec 'omarchy-launch-webapp "<url карточки>"'` → клик по тосту открывает/фокусирует карточку и переживает рестарты shell;
- счётчик непрочитанных — waybar `custom/bx24` (JSON + signal + on-click);
- deep-link — свой `.desktop`-хендлер `x-scheme-handler/bx24` (и `bx`), транслирующий ссылку в URL карточки;
- реальное время — поллинг REST через вебхук (просто) или Push&Pull websocket, если готовы завести OAuth-приложение со scope `pull`.
Плюсы: ноль тяжёлых рантаймов, всё в стиле Omarchy (custom-модули и web-app — штатные механизмы), минимум поверхности отказа. Минусы: нет своего трея (только счётчик в баре), нет inline-reply, действия ограничены одним кликом.

**Подход B — «полноценная обёртка» (эволюция A, если мало нативности):**
Electron-обёртка (референс — [teams-for-linux](https://github.com/ismaelmartinez/teams-for-linux)): свой трей (SNI — waybar показывает), собственные уведомления с actions и клик-хендлерами, обработка `bx…://` через `.desktop` + single-instance, перехват клика-звонка. Tauri v2 — тот же функционал при весе ~9 MiB, но закладывайте время на WebKitGTK/Wayland нюансы (NVIDIA DMABUF, ресайз) и тестируйте на Hyprland. Electron стоит выбрать, если приоритет — предсказуемость Chromium-рендера и готовые библиотеки; Tauri — если приоритет вес/память и не смущает отладка графики.

**Подход C — «не писать своё»: официальное приложение Bitrix24 для Linux (DEB/RPM):**
уже есть уведомления, звонки, чаты, задачи; на Arch ставится переупаковкой в PKGBUILD. Использовать как baseline/параллельный вариант: сначала проверить, закрывает ли оно 80% потребностей (особенно Wayland-трей и клик по уведомлению), и только тогда решать, нужен ли кастом.

**Рекомендуемая последовательность:** проверить официальное приложение (C) → параллельно собрать прототип A (демон+PWA) → если увидите потолок (трей, свои actions, deep-link семантика) — переходить к B (Electron-вариант как менее рискованный на Wayland, Tauri — как более лёгкий).

---

## Противоречия (найденные)

1. **Схема deep-link:** ТЗ говорит `bx24://`, официальная документация описывает `bx://` (и `bitrix24://` как «только форграунд»). Не разрешено — требует проверки установленного клиента.
   **Sources:** [apidocs — bot-context](https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html).
2. **Память Tauri vs Electron:** маркетинговый замер даёт Tauri на ~58% легче; контр-замер на Ubuntu (USS/PSS) — почти паритет. Истинная разница зависит от ОС/нагрузки/метода подсчёта.
   **Sources:** [gethopp](https://www.gethopp.app/blog/tauri-vs-electron), [tauri#5889](https://github.com/tauri-apps/tauri/issues/5889).
3. **Нотификатор Omarchy:** старые гайды (и Omarchy 3 manual) называют mako, актуальный Omarchy 4 — Quickshell-shell. Верно версионированно: ≤3.x mako, ≥4.0 shell.
   **Sources:** [learn.omacom.io (3.x)](https://learn.omacom.io/2/the-omarchy-manual/102/common-tweaks), [omarchylinux.org](https://omarchylinux.org/fix/notifications-not-showing/), [docs/notifications.md](https://github.com/basecamp/omarchy/blob/fa955bfa/docs/notifications.md).
4. **Mako и кнопки действий:** один из поисковых пересказов утверждал «mako не рендерит кнопки действий»; man-страница показывает поддержку действий (`actions=1`, `invoke-action`, `makoctl menu`). В выводе опираюсь на man-страницу; точный визуальный рендер кнопок в mako не верифицировал поштучно.

## Не подтверждено / открытые вопросы

- Рендерит ли нотификатор Omarchy 4 **кнопки произвольных** (не `default`) libnotify-действий сторонних приложений — нужен просмотр `shell/plugins/notifications/components/*` (Popup) или живой тест `notify-send -A "Open" -A "Snooze"`.
- Точный **window class** окна Chromium `--app` (нужен для windowrulev2 и `omarchy-hyprland-focus-app`) — снимается на живой системе (`hyprctl clients`).
- Поведение официального Bitrix24 Desktop на Hyprland/Wayland (XWayland vs нативный, трей, клик по уведомлению).
- Фактическая схема (`bx://` vs `bx24://`), которую регистрирует официальный клиент.
- Клик по Chromium web-notification в --app окне: какой URL/обработчик `notificationclick` срабатывает и открывается ли карточка, а не просто фокус.
- Оценки RAM/веса — все цифры это точечные замеры из источников, не гарантия для вашего приложения.

## Источники

**Kept (основные):**
- Omarchy docs/notifications.md — https://github.com/basecamp/omarchy/blob/fa955bfa/docs/notifications.md — контракт нотификаций, клик, история, DND.
- Omarchy Service.qml — https://raw.githubusercontent.com/omacom/omarchy/quattro/shell/plugins/notifications/Service.qml — механика клика/действий/фокуса из кода.
- Omarchy manual 25-web-apps.md — https://github.com/omacom/omarchy/blob/quattro/manual/25-web-apps.md — штатный web-app механизм.
- bin/omarchy-launch-webapp — https://github.com/basecamp/omarchy/blob/dev/bin/omarchy-launch-webapp — как именно запускается --app.
- Omarchy waybar config.jsonc — https://github.com/basecamp/omarchy/blob/dev/config/waybar/config.jsonc — tray + custom-модули.
- omarchylinux.org/fix/notifications-not-showing — https://omarchylinux.org/fix/notifications-not-showing/ — Omarchy 4.0.0 Quattro, omarchy.notifications.
- omarchy.org/manual/guis — https://omarchy.org/manual/guis/ — состав дефолтных приложений.
- Hyprland Wiki — Window Rules — https://wiki.hyprland.org/hyprland-wiki/pages/Configuring/Window-Rules/
- waybar-custom(5) — https://man.archlinux.org/man/extra/waybar/waybar-custom.5.en; waybar-tray(5) — https://man.archlinux.org/man/extra/waybar/waybar-tray.5.en
- mako(5) — https://man.archlinux.org/man/mako.5; dunst docs — https://dunst-project.org/documentation/ ; SwayNotificationCenter — https://github.com/ErikReider/SwayNotificationCenter (+ issue #551)
- quickshell server.hpp — https://github.com/quickshell-mirror/quickshell/blob/783c9539/src/services/notifications/server.hpp — capabilities (actions/inlineReply флаги)
- Tauri: prerequisites — https://v2.tauri.app/start/prerequisites/ ; Linux graphics — https://v2.tauri.app/develop/debug/linux-graphics/ ; deep-link — https://v2.tauri.app/plugin/deep-linking/ ; bundler 2.10.0 — https://v2.tauri.app/release/tauri-bundler/v2.10.0/ ; wry#1727 — https://github.com/tauri-apps/wry/issues/1727 ; tauri#15928 — https://github.com/tauri-apps/tauri/issues/15928 ; #5889 — https://github.com/tauri-apps/tauri/issues/5889 ; benchmarks — https://tauri-apps.github.io/benchmark_results/
- gethopp Tauri vs Electron — https://www.gethopp.app/blog/tauri-vs-electron
- Chromium: manpage/user-data-dir — https://chromium.googlesource.com/chromium/chromium/%2B/refs/heads/main/chrome/app/resources/manpage.1.in ; process model — https://chromium.googlesource.com/chromium/src/%2B/main/docs/process_model_and_site_isolation.md ; os_integration — https://chromium.googlesource.com/chromium/src/%2B/fd2cb3f55a752f3d452d5f7c619dd20ff37f1f66/chrome/browser/web_applications/docs/os_integration.md
- Chrome PWA protocol handler — https://developer.chrome.com/docs/web-platform/best-practices/url-protocol-handler ; https://developer.chrome.com/blog/new-in-chrome-96/
- Electron: app API — https://electronjs.org/docs/latest/api/app ; tray docs — https://github.com/electron/electron/blob/main/docs/api/tray.md ; #6440 — https://github.com/electron/electron/issues/6440 ; electron-builder#4035 — https://github.com/electron-userland/electron-builder/issues/4035 ; electron.build Linux — https://www.electron.build/docs/linux/
- XDG scheme handlers — https://unix.stackexchange.com/questions/497146/create-a-custom-url-protocol-handler ; https://blog.mggross.com/extending-scheme-handlers/ ; https://blog.za3k.com/url-handlers-in-linux/ ; https://wiki.archlinux.org/title/Desktop_entry
- Arch packaging — https://wiki.archlinux.org/title/Arch_package_guidelines ; https://wiki.archlinux.org/title/Electron_package_guidelines ; systemd/User — https://wiki.archlinux.org/title/Systemd/User
- Bitrix24: официальный Linux-клиент — https://helpdesk.bitrix24.ru/open/16171806/ ; desktop.php — https://www.bitrix24.com/apps/desktop.php ; Push&Pull custom client — https://github.com/bitrix24/b24restdocs/blob/main/settings/interactivity/custom-push-and-pull-client.md ; webhooks — https://www.bitrix24.com/apps/webhooks.php ; bx:// — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html ; telephony — https://apidocs.bitrix24.com/api-reference/telephony/telephony-external-call-register.html
- Примеры демонов/счётчиков: https://github.com/lfrancke/gh-notifier/ ; https://github.com/bnema/gh-notify ; https://github.com/benelan/gh-notify-desktop ; https://github.com/kouiso/github-notify ; https://github.com/mizzunet/waybar-unread-email ; https://github.com/crabvk/bar-gmail ; https://github.com/hoblin/thunderbird-waybar-email
- Обёртки-референсы: https://github.com/ismaelmartinez/teams-for-linux (+ https://aur.archlinux.org/packages/teams-for-linux) ; https://github.com/rafatosta/zapzap ; https://github.com/xeco23/WasIstLos ; https://github.com/buglloc/brick/ ; https://github.com/smallcms/infinitrix
- Snap bitrix (устаревший) — https://snapcraft.io/install/bitrix/arch

**Rejected/deprioritized:**
- wiki.omarchy.org — не открылся при проверке (ошибка сертификата), заменён на omarchy.org/manual + GitHub docs.
- Всевозможные SEO-сравнительники (johal.in, buildr.sh, newstack.dev и пр.) — вторичны к первоисточникам (gethopp/tauri issues/официальные доки), оставлены за кадром.
- always-coding.co.uk Omarchy 3.8 Guide — неофициальный, использован только как подтверждение mako в 3.x рядом с primary-источниками.

## Next steps

1. На живой Omarchy: `hyprctl clients` — снять class окна Bitrix24-web-app; `notify-send -A` — проверить отрисовку кнопок действий и reply в shell-нотификаторе; поставить официальный Bitrix24 Desktop (через distrobox или переупаковку) и посмотреть, какую схему он регистрирует (`ls /usr/share/applications | grep -i bitrix` + `xdg-mime query default x-scheme-handler/bx` / `.../bx24`).
2. Прототип демона: поллинг `crm.*`/`im` REST через вебхук → `omarchy-notification-send --exec` + waybar custom; затем оценить переход на Push&Pull (нужен OAuth-app со scope `pull`).
3. Если пойдёт в сторону обёртки — прототип Electron-обёртки по паттерну teams-for-linux с трей/actions/deep-link и замер памяти на вашем железе.

# Исследование: что реально умеет REST API Bitrix24 для внешнего десктоп-приложения (Linux, трей-плагин)

**Дата:** 2026-02. **Метод:** web_search + fetch_content официальных страниц документации (первоисточники прочитаны целиком, ключевые цитаты приведены).

**Примечание об источниках.** Актуальная официальная документация REST API Битрикс24 живёт на порталах **apidocs.bitrix24.ru** (рус.) и **apidocs.bitrix24.com** (англ.) — это преемник раздела REST на dev.1c-bitrix.ru; новые возможности (чат-боты 2.0, im.v2, pull-канал приложения, офлайн-события в новом виде) документируются только там. Старые страницы dev.1c-bitrix.ru/rest_help сохраняются, но по свежести уступают. Ссылки на helpdesk.bitrix24.ru приводятся в тех местах, где официальные страницы сами ссылаются на пользовательскую документацию. Помечено: ✅ — прямое свидетельство из прочитанной официальной страницы; 🔎 — моя интерпретация; ⚠️ — не подтверждено.

---

## Summary

REST API Битрикс24 покрывает все три сценария трей-плагина (уведомления о лидах/сообщениях/звонках, клик-ту-колл, переход в карточку), но с оговорками: **события в реальном времени доставляются только на публичный HTTPS-обработчик** (или через очередь офлайн-событий по опросу), а **для телефонии и подписки на события нужен контекст приложения** (вебхука недостаточно). Самая простая рабочая связка для личного/компанийного плагина — **локальное приложение** (токены приходят одним callback-запросом при установке, дальше обновляются по `refresh_token` без участия сервера приложения) + **опрос** `im.v2.Event.get` (сообщения) и `event.offline.get` (CRM-события). Облако требует коммерческого тарифа с доступом к REST (подписка «Маркетплейс»); лимиты — 2 запроса/с (Enterprise 5/с), batch до 50 команд, `access_token` живёт 1 час, `refresh_token` — 180 дней.

---

## 1. Доступ

### 1.1. Входящий вебхук
- ✅ «Входящий вебхук вызывает методы REST API по секретному URL… Работает в рамках `scope` и прав сотрудника. Часть методов требует контекста приложения и через вебхук недоступна» — https://apidocs.bitrix24.ru/local-integrations/local-webhooks.html
- ✅ Формат URL: `https://example.bitrix24.ru/rest/1/xxxxxxxx/department.get.json?ID=42` (ID пользователя + секретный код); запросы выполняются от имени и с правами создавшего вебхук сотрудника; создать вебхук может админ или сотрудник с разрешённым правом; вебхук может иметь дату окончания. Там же — примеры GET/POST.
- ✅ Ограничение по методам: «часть методов через вебхук недоступна, потому что требует контекста приложения. Например, методы встраивания виджетов, такие как placement.bind, часть методов телефонии и некоторые сценарии чат-ботов» — https://apidocs.bitrix24.ru/local-integrations/local-webhooks.html

### 1.2. Исходящий вебхук (события без приложения)
- ✅ «Исходящий вебхук отправляет событие Битрикс24 на URL вашего обработчика… Обработчик должен быть доступен из внешней сети» — https://apidocs.bitrix24.ru/local-integrations/local-webhooks.html
- ✅ Payload: `event`, `data[FIELDS][ID]`, `ts`, `auth[domain|member_id|application_token]`; проверка подлинности — сравнение `application_token` со значением из настроек вебхука. Там же.
- ✅ Важно: исходящему вебхуку OAuth-токены не передаются — «An outbound webhook receives only `domain`, `client_endpoint`, `server_endpoint`, `member_id`, and `application_token` — OAuth 2.0 tokens… are not passed to a webhook» — https://apidocs.bitrix24.com/api-reference/events/index.html
- ✅ Исходящий вебхук в UI подписывается только на «онлайн»-события из списка вебхуков; приложение через `event.bind` может подписаться на все события, включая офлайн — https://apidocs.bitrix24.com/api-reference/events/index.html

### 1.3. Локальное приложение (ключи выдаются в самом Битрикс24)
- ✅ «Локальное приложение создают и настраивают на конкретном Битрикс24 — работает оно только там»; три вида: статичное (код в браузере), серверное с интерфейсом, **серверное без интерфейса**; ключи (`client_id`/`client_secret`) Битрикс24 показывает после сохранения формы в «Приложения > Разработчикам» — https://apidocs.bitrix24.ru/local-integrations/local-apps.html
- ✅ **Callback установки — самый простой способ получить токены:** «Сразу после добавления локального приложения Битрикс24 автоматически обратится к этому URL и передаст туда POST-запрос с данными авторизации OAuth 2.0, включая `access_token` и `refresh_token`… постоянный доступ обеспечивается не бессрочным токеном, а сохранённым `refresh_token`» — https://apidocs.bitrix24.ru/settings/app-installation/local-apps/installation-callback.html (обработчик нужен публичный по HTTPS; `BX24.installFinish()` не требуется).
- ✅ Серверное приложение без интерфейса само продлевает токены; статичное приложение события не получает — https://apidocs.bitrix24.ru/local-integrations/local-apps.html

### 1.4. OAuth 2.0 (полный протокол)
- ✅ «Полный протокол нужен приложению, которое работает за пределами интерфейса Битрикс24 и хранит токены на своей стороне»; сервер авторизации `https://oauth.bitrix24.tech/`; шаги: авторизация пользователя в его Б24 → `code` на `redirect_uri` → обмен на токены; **`code` живёт 30 секунд**; `scope` = права из карточки приложения; вызовы — `https://portal.bitrix24.ru/rest/{method}?auth={access_token}` — https://apidocs.bitrix24.ru/settings/oauth/
- ✅ Ответ токенов: `expires_in: 3600`, `refresh_token`, `client_endpoint`, `member_id`, `scope`, `status` (L — локальное, F — бесплатное тиражное, P — платное) — https://apidocs.bitrix24.ru/settings/oauth/
- ⚠️ Нюанс: в англоязычных страницах и примерах событий фигурирует `oauth.bitrix.info` (например, https://apidocs.bitrix24.com/settings/oauth/auto-renewal.html и пример `auth` в https://apidocs.bitrix24.com/api-reference/telephony/events/on-external-call-start.html). 🔎 Практический вывод: ориентироваться на `server_endpoint`, который реально приходит в ответах/событиях вашего портала, а не хардкодить домен.

### 1.5. Скоупы (права)
- ✅ Скоупы: `crm`, `im` (чат и уведомления), `imbot` (боты), `imopenlines`, `telephony` (телефония), `call` (voximplant.infocall.*), `pull` (Push&Pull), `placement`, `intranet` (виджеты), `user` (варианты user_brief/user_basic) и др.; вызов вне скоупа → `insufficient_scope`; часть методов требует контекста приложения → вебхук получает `WRONG_AUTH_TYPE` — https://apidocs.bitrix24.ru/api-reference/scopes/permissions.html

### 1.6. Тарифные требования
- ✅ «REST is available only on commercial plans… Vibe+ plans include REST API access, while Essentials plans do not» (текст ошибки `ACCESS_DENIED`) — https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify.html (таблица системных ошибок, одинакова на всех страницах методов)
- ✅ Постоянный доступ к REST в облаке = подписка «Маркетплейс» (BitrixGPT + Маркетплейс) на платном тарифе, либо пробный режим, либо NFR-ключ для партнёров — https://apidocs.bitrix24.ru/first-steps/access-to-rest-api.html
- ✅ «Локальное приложение работает, только если у Битрикс24 есть доступ к REST API — по подписке BitrixGPT + Маркетплейс или в пробном режиме» — https://apidocs.bitrix24.ru/local-integrations/local-apps.html
- ✅ Все вызовы к облачному REST — только HTTPS (`INVALID_REQUEST: Https required`) — таблица системных ошибок, например https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify.html

### 1.7. Что проще для личного/компанийного плагина 🔎
- **Личный плагин одного пользователя (только чаты/уведомления):** достаточно одного входящего вебхука со скоупом `im` — `im.v2.Event.subscribe`/`im.v2.Event.get` работают по вебхуку (в доках есть примеры cURL Webhook: https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-get.html, https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-subscribe.html). Без OAuth и без сервера.
- **Компанийный плагин (лиды/сделки/звонки, несколько пользователей):** локальное приложение, серверное без интерфейса, `scope: crm, im, imbot, telephony, pull, user`; токены — один раз через callback установки (нужен любой публичный HTTPS-эндпоинт, можно дешёвый VPS/туннель), дальше всё выполняется с машины пользователя. Подписка на офлайн-события требует прав администратора (см. §2).
- Исходящие вебхуки удобны, когда у вас в принципе есть публичный сервер; для «чистого» десктопа за NAT они напрямую не подходят.

---

## 2. События / realtime

### 2.1. Как доставляются события
- ✅ Событие попадает в очередь на специальном сервере, оттуда POST на URL обработчика; «The handler URL must be accessible from the external network. Do not use addresses on localhost or in a local network»; «If the handler responds slowly, the server reduces the frequency of calls» — https://apidocs.bitrix24.com/api-reference/events/index.html
- ✅ Два ограничения: нагрузку регулировать нельзя (массовое изменение = много вызовов) и **повторных попыток нет** — «If your server does not respond or returns an error, the Bitrix24 queue server will log the failure but will not resend the event» — там же.

### 2.2. event.bind (подписка приложения)
- ✅ `event.bind` работает **только в контексте приложения** (вебхук → `WRONG_AUTH_TYPE`); параметры: `event` (например `ONCRMLEADADD`), `handler` (URL со схемой http/https, «в имени хоста должна быть точка, поэтому адрес localhost не подходит»; обязателен для онлайн-событий), `auth_type` (от имени какого пользователя обработчик получает авторизацию; 0 — пользователь, вызвавший событие), `event_type` (`online`|`offline`), `auth_connector` (ключ источника; тарифно — проверить `feature.get` с кодом `rest_auth_connector`) — https://apidocs.bitrix24.ru/api-reference/events/event-bind.html
- ✅ «офлайн-события недоступны: подписка с `event_type=offline` вернет ошибку `ACCESS_DENIED`» для не-админа — https://apidocs.bitrix24.ru/api-reference/events/event-bind.html
- ✅ Список доступных событий возвращает метод `events` — https://apidocs.bitrix24.ru/api-reference/events/events.html (ссылка из https://apidocs.bitrix24.ru/api-reference/events/event-bind.html)

### 2.3. Пример: ONCRMLEADADD
- ✅ Обработчик получает POST: `data.FIELDS.ID` (ID лида), `ts`, `auth` (токены, если хит привязан к пользователю; «Токены авторизации не всегда передаются… Обязательно проверяйте содержимое ключа auth») — https://apidocs.bitrix24.ru/api-reference/crm/leads/events/on-crm-lead-add.html
- ✅ Полные данные объекта событие не несёт — «An event reports only the fact of a change. To retrieve the object data itself, call the read method» — https://apidocs.bitrix24.com/api-reference/events/index.html

### 2.4. Офлайн-события — канал без публичного сервера (polling)
- ✅ «Офлайн-события — это механизм получения событий, при котором Битрикс24 не вызывает обработчик приложения, а сохраняет изменения в очереди на стороне Битрикс24… Механизм подходит приложениям, которые не могут принимать входящие вызовы: работают за файрволом» — https://apidocs.bitrix24.ru/api-reference/events/offline-events.html
- ✅ Регистрация: `event.bind` с `event_type=offline` **без** `handler`; получение: `event.offline.get` (пакет до 50, `clear=1` удаляет сразу, `clear=0` резервирует пакет с `process_id`), `event.offline.list`, подтверждение `event.offline.clear`, разметка ошибок `event.offline.error`; зарезервированный пакет хранится до 30 дней; в очереди хранится **последнее состояние объекта** (1000 изменений сделки = 1 запись) — https://apidocs.bitrix24.ru/api-reference/events/offline-events.html
- ✅ «Только администратор может работать с очередью офлайн-событий» (`event.offline.*`) — https://apidocs.bitrix24.com/api-reference/events/index.html
- ✅ Расширенный режим (`clear=0`) тарифно зависит от плана: проверить `feature.get` с кодом `rest_offline_extended` (в коробке доступен всегда) — https://apidocs.bitrix24.ru/api-reference/events/offline-events.html
- ✅ Есть событие `onOfflineEvent` «при появлении новых записей в очереди офлайн-событий» (с настраиваемым `minTimeout`) — https://apidocs.bitrix24.com/api-reference/events/index.html ; 🔎 для чистого десктопа без обработчика это уведомление бесполезно (оно тоже идёт на URL), опрашивайте очередь сами.

### 2.5. События чатов для пользователя — polling-очередь im.v2
- ✅ `im.v2.Event.subscribe` подписывает текущего пользователя на журналирование событий; `im.v2.Event.get` возвращает события polling-режимом с подтверждением через `offset`; типы: `ONIMV2MESSAGEADD`, `ONIMV2MESSAGEUPDATE`, `ONIMV2MESSAGEDELETE`, `ONIMV2REACTIONCHANGE`, `ONIMV2JOINCHAT` — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-get.html, https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-subscribe.html
- ✅ «Records are retained for 24 hours or until they are acknowledged through `offset`»; без подписки — пустой массив; **эксклюзивность**: «Events for a specific user can only be retrieved by **one application**» — несколько подписчиков «воруют» события друг у друга — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-get.html
- ✅ Оба метода показаны с примерами cURL **через вебхук** (то есть работают и без OAuth-приложения, от имени владельца вебхука) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-subscribe.html
- ✅ Обзор im.v2: «вызовы выполняются от имени текущего пользователя… нужен скоуп `im` вместо `imbot`… события доставляются только в polling-режиме» — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/index.html

### 2.6. События ботов
- ✅ Чат-боты 2.0: `imbot.v2.Bot.register` с `eventMode: "fetch"` (события забираются `imbot.v2.Event.get` — очередь с `offset`) или `eventMode: "webhook"` (POST на `webhookUrl`); события включают `ONIMBOTV2MESSAGEADD`, `ONIMBOTV2CONTEXTGET` и др. — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/events/event-get.html, https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html, https://apidocs.bitrix24.ru/api-reference/chat-bots/chat-bots-v2/quick-start.html
- ✅ Старое API: `imbot.register` + обработчик и события вида `ONIMBOTMESSAGEADD` (блог платформы чат-ботов) — https://dev.1c-bitrix.ru/community/blogs/marketplace_apps24/we-present-a-platform-for-chatbots-for-bitrix24.php

### 2.7. Push&Pull: websocket/long polling для собственного клиента
- ✅ «Собственный клиент нужен, когда приложение работает отдельно от интерфейса Битрикс24: серверный процесс интеграции, **десктопное приложение**, веб-приложение на своем домене» — https://apidocs.bitrix24.ru/settings/interactivity/custom-push-and-pull-client.html
- ✅ Конфигурацию подключения даёт `pull.application.config.get` (scope `pull`, **только контекст приложения** — «вебхук такой контекст не создает»); подключение по `server.websocket_secure` (wss) или `server.long_pooling_secure` (long polling); обмен односторонний (сервер → клиент); обработка `channel_expire`/`config_expire`/`server_restart`; keep-alive ~20 с, long-polling таймаут 60 с — https://apidocs.bitrix24.ru/settings/interactivity/custom-push-and-pull-client.html, https://apidocs.bitrix24.ru/settings/interactivity/pull-application-config-get.html
- ✅ В канал попадают события, которые публикует **серверная часть приложения** методом `pull.application.event.add` (scope `pull`) — https://apidocs.bitrix24.ru/settings/interactivity/pull-application-event-add.html
- 🔎 **Интерпретация (важно):** документация описывает канал приложения как канал для собственных событий приложения; прямого утверждения, что системные события Битрикс24 (новый лид, входящее сообщение) автоматически попадают в канал `pull.application.*`, в доках нет. Реалистичная схема realtime для десктопа: публичный HTTPS-обработчик получает системное событие → ваш сервер публикует его в pull-канал → плагин читает по websocket. Без публичного сервера — только polling (§2.4–2.5).

### 2.8. Про ONIMMESSAGEADD ⚠️
- ⚠️ Событие `ONIMMESSAGEADD` в актуальном списке REST-событий для `event.bind` не обнаружено (поиск по apidocs.bitrix24.ru/.com его не находит). Рабочие аналоги: `ONIMV2MESSAGEADD` через `im.v2.Event.get` (§2.5) или события бота `ONIMBOT(V2)MESSAGEADD` (§2.6). Проверяйте фактический список методом `events` на своём портале: https://apidocs.bitrix24.ru/api-reference/events/events.html

**Итог по realtime 🔎:** «настоящего» long polling для внешнего приложения к произвольным событиям нет. Варианты: (а) онлайн-обработчик на публичном HTTPS; (б) офлайн-очередь `event.offline.get` + polling; (в) для чатов — `im.v2.Event.get` + polling; (г) websocket pull-канал — но только для событий, опубликованных вашим же сервером.

---

## 3. IM и уведомления

### 3.1. Нотификации пользователю
- ✅ `im.notify` (scope `im`) — «отправить уведомление пользователю»; **нельзя вызывать с session-авторизацией** — только вебхук или OAuth-токен приложения; параметры: `USER_ID`, `TYPE` (`USER`|`SYSTEM`), `MESSAGE`, `MESSAGE_OUT` (текст для внешних каналов, напр. email), `TAG`/`SUB_TAG` (дедупликация; с вебхуком — вместе с `CLIENT_ID`), `ATTACH` (до 60 000 символов) — https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify.html
- ✅ Семейство методов уведомлений: `im.notify.personal.add`, `im.notify.system.add`, `im.notify.get`, `im.notify.schema.get`, `im.notify.read.list`, `im.notify.read`, `im.notify.read.all`, `im.notify.answer` (быстрый ответ), `im.notify.confirm` (кнопки), `im.notify.delete`, `im.notify.history.search` — https://apidocs.bitrix24.com/api-reference/chats/notifications/index.html (список из «Continue Learning» страницы im.notify)

### 3.2. Сообщения в чаты
- ✅ `im.message.add` (scope `im`, «пользователь с правом отправки сообщений в чат»): `DIALOG_ID` (`chatXXX` — чат, `sgXXX` — группа/проект, `XXX` — ID пользователя для личного диалога), `MESSAGE` (форматирование), `ATTACH`, `KEYBOARD` (кнопки), `MENU`, `SYSTEM`, `URL_PREVIEW`, `REPLY_ID`; результат — ID сообщения — https://apidocs.bitrix24.com/api-reference/chats/messages/im-message-add.html
- ✅ Прочее: `im.message.update`, `im.message.delete`, `im.message.like` — https://apidocs.bitrix24.com/api-reference/chats/messages/im-message-add.html (Continue Learning)

### 3.3. Чтение чатов пользователя
- ✅ `im.dialog.messages.get` (scope `im`, «участник чата») — возвращает сообщения диалога, включая системные; `LAST_ID`/`FIRST_ID` для движения по истории, `LIMIT` по умолчанию 20, **максимум 50** — https://apidocs.bitrix24.com/api-reference/chats/messages/im-dialog-messages-get.html
- ✅ `DIALOG_ID` можно получить через `im.chat.get`; пользователей — `user.get`/`user.search` — https://apidocs.bitrix24.com/api-reference/chats/messages/im-message-add.html
- ⚠️ Поиск по сообщениям (`im.dialog.messages.search`) и список последних чатов (`im.recent.*`) в этой сессии напрямую не проверены — см. «Не подтверждено».
- 🔎 Вебхук/токен даёт доступ только к тому, что доступно пользователю-владельцу авторизации: «REST API выполняет запросы от имени пользователя… API не расширяет права доступа» — https://apidocs.bitrix24.ru/first-steps/access-to-rest-api.html. То есть «читать все чаты юзера» можно лишь от его имени и лишь те, где он участник.

### 3.4. Боты
- ✅ Старые `imbot.*` объявлены устаревшими; новое — `imbot.v2.*`: `imbot.v2.Bot.register`, `imbot.v2.Chat.Message.send/read/get`, `imbot.v2.File.upload/download`; чтение содержимого сообщений ботом (`Chat.Message.get`/`getContext`) — только ботам типов `supervisor` и `personal` — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/messages/index.html
- ✅ Передача контекста боту при открытии чата по ссылке — событие `ONIMBOTV2CONTEXTGET` — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html

**Вывод для трея 🔎:** слать нотификации пользователю — тривиально (`im.notify`/`im.message.add`); узнавать о новых сообщениях — `im.v2.Event.get` (§2.5); прочесть текст сообщения — событие несёт данные, плюс `im.dialog.messages.get`.

---

## 4. Deep links

### 4.1. Веб-URL карточек CRM (самый надёжный путь для внешнего приложения)
- ✅ Официальный пример пути карточки: `BX24.openPath('/crm/deal/details/123/')` — https://apidocs.bitrix24.ru/sdk/bx24-js-sdk/additional-functions/bx24-open-path.html
- ⚠️ Общий паттерн `/crm/{lead|deal|contact|company}/details/{ID}/` подтверждён для сделки; для лида/контакта встречается в разборах кода роутера CRM (`bitrix:crm.lead.details` → `crm/lead/details/#ENTITY_ID#/` — https://aclips.ru/bitrix24-api/bx_crm_router_getdefaulturltemplates/, неофициальный источник) — считать рабочей гипотезой, проверить на своём портале.
- 🔎 `BX24.openPath` работает только внутри фрейма приложения в интерфейсе Битрикс24. Для внешнего Linux-приложения практичный вариант — открыть обычный `https://{портал}/crm/lead/details/{ID}/` в браузере (пользователь уже залогинен в вебе) или в десктоп-клиенте Б24, если он перехватывает ссылки.

### 4.2. Схема `bx://` (десктоп-приложение)
- ✅ Единственный официально задокументированный формат: диплинк передачи контекста боту — `bx://v2/{portal}/botContext/dialogId/{dialogId}/context/{urlEncodedJson}` («Deep link of the Bitrix24 desktop application. Opens a chat with the bot directly through the `bx://` protocol handler») — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html
- ✅ Про мобильные: «The `bx://` protocol belongs to the desktop application. The Bitrix24 mobile applications… do not handle it… The `bitrix24://` protocol brings the application to the foreground but does not parse the address inside it» — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html
- ⚠️ Полного публичного списка действий `bx://` (открыть произвольную карточку CRM, звонок и т.п.) в документации REST нет — не подтверждено. В коде ядра существует класс `OpenDesktopApp` с константой `DESKTOP_PROTOCOL` (зеркало исходников: https://www.doc.budagov.ru/opendesktopapp_8php_source.html), но это внутренности ядра, не контракт для разработчиков.

### 4.3. Открытие приложения по ссылке
- ✅ `REST_APP_URI`: ссылка вида `/marketplace/view/#APP_CODE#/` (можно с параметрами → `PLACEMENT_OPTIONS`) открывает приложение слайдером в веб-интерфейсе — https://apidocs.bitrix24.ru/api-reference/widgets/universal/app-url.html
- ✅ Изнутри приложения также доступны `BX24.openPath`, `Messenger.openChat`, `Messenger.startVideoCall`, `Messenger.startPhoneCall` — https://apidocs.bitrix24.ru/sdk/bx24-js-sdk/additional-functions/index.html

**Вывод 🔎:** «переход в карточку» из внешнего плагина реализуемо через обычные веб-URL портала; глубокие ссылки в десктоп-клиент на произвольную страницу официально не документированы.

---

## 5. Телефония

### 5.1. Три REST-сценария
- ✅ «интеграция внешней телефонии — приложение регистрирует линию, сообщает о звонке, управляет карточкой и завершает звонок; управление встроенной телефонией и SIP-коннектором; получение AI Follow-up завершенных звонков» — https://apidocs.bitrix24.ru/api-reference/telephony/index.html

### 5.2. Внешняя телефония (плагин как «телефония»)
- ✅ Порядок: `telephony.externalLine.add` (линия) → `telephony.externalCall.register` (начало звонка) → при необходимости `telephony.externalCall.show` (открыть карточку звонка у пользователя) / `telephony.externalCall.hide` → `telephony.externalCall.finish` (создаёт дело в CRM) → `telephony.externalCall.attachRecord` (запись) → `telephony.call.attachTranscription` (расшифровка) — https://apidocs.bitrix24.ru/api-reference/telephony/index.html
- ✅ `telephony.externalCall.register` (scope `telephony`): `USER_ID`/`USER_PHONE_INNER`, `PHONE_NUMBER`, `TYPE` (1 — исходящий, 2 — входящий, 3 — входящий с переадресацией, 4 — callback, 5 — информационный), `CRM_CREATE`, `CRM_ENTITY_TYPE`/`CRM_ENTITY_ID`, `SHOW` (показать карточку звонка после регистрации, по умолчанию 1), `ADD_TO_CHAT`, `LINE_NUMBER`, `EXTERNAL_CALL_ID`; повторный вызов в течение 30 минут может вернуть существующий `CALL_ID` — передавайте уникальный `EXTERNAL_CALL_ID` — https://apidocs.bitrix24.com/api-reference/telephony/telephony-external-call-register.html
- ✅ **«The method works only in the context of an application»** — `register` (и `finish`) недоступны через вебхук — https://apidocs.bitrix24.com/api-reference/telephony/telephony-external-call-register.html, https://apidocs.bitrix24.ru/api-reference/telephony/index.html («Методы telephony.externalCall.register и telephony.externalCall.finish работают только в контексте приложения»)
- ✅ Поиск объектов CRM по номеру: `telephony.externalCall.searchCrmEntities` — https://apidocs.bitrix24.ru/api-reference/telephony/index.html

### 5.3. Клик-ту-колл из CRM (входящий сигнал в приложение)
- ✅ `ONEXTERNALCALLSTART` — «triggered when a user clicks on a phone number in CRM entities to make an outgoing call through the selected telephony application. To trigger the event, specify the application in the Default Number for Outgoing Calls field in the telephony settings or select it as the default application in the user settings»; в `data`: `PHONE_NUMBER`, `PHONE_NUMBER_INTERNATIONAL`, `EXTENSION`, `USER_ID`, `CALL_LIST_ID`, `LINE_NUMBER`, `IS_MOBILE`, `CALL_ID`, `CRM_ENTITY_TYPE`, `CRM_ENTITY_ID` — https://apidocs.bitrix24.com/api-reference/telephony/events/on-external-call-start.html
- ✅ Туториал интеграции внешней телефонии (включая подписку на `ONEXTERNALCALLSTART` и `ONEXTERNALCALLBACKSTART`) — https://apidocs.bitrix24.com/tutorials/telephony/index
- ✅ Пользовательская документация: «Как начать звонить из Битрикс24» — https://helpdesk.bitrix24.ru/open/6450911/ (ссылка со страницы https://apidocs.bitrix24.ru/api-reference/telephony/index.html)

### 5.4. Клик-ту-колл из внешнего приложения
- 🔎 REST **не набирает номер** во внешней телефонии: плагин сам дозванивается (свой SIP-стек/софтфон) и фиксирует звонок в Б24 (`register` → `show` → `finish` → `attachRecord`). Прямого метода «набрать номер через софтфон пользователя» в REST нет.
- ✅ Через встроенную телефонию можно запустить автозвонок/обратный звонок: `voximplant.callback.start`, `voximplant.infocall.startwithsound`, `voximplant.infocall.startwithtext` (scope `call`) — https://apidocs.bitrix24.ru/api-reference/telephony/index.html, https://apidocs.bitrix24.ru/api-reference/scopes/permissions.html
- ✅ SIP-коннектор: `voximplant.sip.add/get/status/update/delete`, линии `voximplant.line.*`, настройки пользователя `voximplant.user.*`; события `OnVoximplantCallInit/Start/End` — https://apidocs.bitrix24.ru/api-reference/telephony/index.html

---

## 6. Лимиты

### 6.1. Интенсивность запросов
- ✅ Алгоритм Leaky Bucket: «At an intensity of no more than `Y` requests per second, the application will not receive error `QUERY_LIMIT_EXCEEDED`»; Enterprise — 5 req/s (порог 250), остальные тарифы — 2 req/s (порог 50); превышение → HTTP 503 `QUERY_LIMIT_EXCEEDED`; лимит общий для приложений с одного IP в рамках одного портала — https://apidocs.bitrix24.com/settings/performance/limits.html
- ✅ Ориентир объёма: при 2 req/s — до 172 800 запросов/сутки; `*.list` отдают по 50 записей на страницу — там же.

### 6.2. Ресурсоёмкость
- ✅ В ответе `time.operating` (накопленное время выполнения метода) и `time.operating_reset_at`; учёт в 10 минутных корзинах; превышение → HTTP 429 `OPERATION_TIME_LIMIT` (блокируется только конкретный метод конкретного приложения/вебхука); в коробке лимит по умолчанию 420 с, если включён — https://apidocs.bitrix24.com/settings/performance/limits.html

### 6.3. Таймаут и batch
- ✅ Один REST-запрос в облаке должен уложиться в 60 секунд — https://apidocs.bitrix24.com/settings/performance/limits.html
- ✅ `batch`: «A single batch contains no more than 50 subqueries. Nesting is prohibited: you cannot call another `batch` inside `batch`» (с rest 24.0.0 вложенный batch → `ERROR_BATCH_METHOD_NOT_ALLOWED`; лишние команды → `ERROR_BATCH_LENGTH_EXCEEDED`); внешний вызов `batch` не учитывается в `operating`, но вложенные методы — учитываются каждый сам по себе — https://apidocs.bitrix24.com/settings/how-to-call-rest-api/batch.html, https://apidocs.bitrix24.com/settings/performance/limits.html

### 6.4. Токены и продление
- ✅ `access_token` — 1 час (`expires_in: 3600`), `refresh_token` — 180 дней, авторизационный `code` — 30 секунд — https://apidocs.bitrix24.ru/settings/oauth/, https://apidocs.bitrix24.ru/settings/oauth/auto-renewal.html (англ.: https://apidocs.bitrix24.com/settings/oauth/auto-renewal.html)
- ✅ Рекомендуемая логика продления: не обновлять по таймеру; дождаться `401 expired_token`, обменять `refresh_token` на новую пару (приходит **новая** пара — перезаписать обе), повторить вызов; при раз в 180 дней обращении достаточно одного обновления в цикле — https://apidocs.bitrix24.com/settings/oauth/auto-renewal.html
- ⚠️ Суточных квот запросов в актуальной официальной странице лимитов нет (исторические упоминания тарифных суточных лимитов в новых доках не найдены) — считать не подтверждённым.

---

## 7. Вывод: что реально собрать 🔎

**Целевая архитектура трей-плагина под Linux (рекомендация):**

1. **Авторизация:** локальное приложение «серверное без интерфейса» (https://apidocs.bitrix24.ru/local-integrations/local-apps.html) в Б24 компании, `scope: crm, im, imbot, telephony, pull, user`. Первая пара токенов — через callback установки на любой публичный HTTPS-эндпоинт (https://apidocs.bitrix24.ru/settings/app-installation/local-apps/installation-callback.html); далее плагин автономен: `expired_token` → refresh → повтор (https://apidocs.bitrix24.com/settings/oauth/auto-renewal.html). Требуется: коммерческий тариф + подписка Маркетплейс (https://apidocs.bitrix24.ru/first-steps/access-to-rest-api.html) и админ для регистрации офлайн-подписок.
2. **Трей-уведомления о сообщениях:** `im.v2.Event.subscribe` один раз + периодический `im.v2.Event.get` (лимит страницы до 1000, подтверждение `nextOffset`; хранение 24 ч) → `ONIMV2MESSAGEADD` → всплывающее уведомление. Работает даже по вебхуку для одного пользователя; для нескольких пользователей — по токену каждого (внимание: эксклюзивность «одно приложение-читатель на пользователя», https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-get.html).
3. **Трей-уведомления о лидах/сделках:** `event.bind {event_type: "offline"}` на `ONCRMLEADADD`/`ONCRMDEALUPDATE`… + опрос `event.offline.get` (https://apidocs.bitrix24.ru/api-reference/events/offline-events.html). Реалтайм-задержка = интервал опроса; при 2 req/s и batch 50 опрос раз в 5–10 с по бюджету запросов безопасен.
4. **Уведомления о звонках/звонки:** если плагин выступает внешней телефонией — `externalLine.add` + `externalCall.register/show/finish/attachRecord` (только контекст приложения); входящий сигнал «пользователь кликнул номер в CRM» — событие `ONEXTERNALCALLSTART` (нужен публичный HTTPS-обработчик либо офлайн-подписка + опрос). Автозвонок через встроенную телефонию — `voximplant.callback.start`/`infocall.*` (scope `call`).
5. **Клик в карточку:** из уведомления открывать `https://{портал}/crm/lead/details/{ID}/` в браузере (или передавать в десктоп-клиент Б24, если он зарегистрирован как обработчик `bx://` — но официально задокументирован только `bx://v2/.../botContext/...`, §4.2).
6. **Отправка сообщений/нотификаций из плагина:** `im.notify` (вебхук/OAuth) и `im.message.add`.

**Чего собрать нельзя (без доп. инфраструктуры):** push-доставка системных событий B24 прямо в плагин без публичного HTTPS-обработчика; вызов `telephony.externalCall.register/finish` и `event.bind` через голый вебхук; REST на бесплатном/Essentials-тарифе.

---

## 8. Не подтверждено / открытые вопросы

- ⚠️ `ONIMMESSAGEADD` как `event.bind`-событие в актуальных доках не найден; фактический список — методом `events` (https://apidocs.bitrix24.ru/api-reference/events/events.html).
- ⚠️ Полный список действий схемы `bx://` десктоп-клиента (кроме `botContext`) — не документирован.
- ⚠️ Паттерны URL карточек лида/контакта/компании — подтверждён только `/crm/deal/details/{ID}/` (https://apidocs.bitrix24.ru/sdk/bx24-js-sdk/additional-functions/bx24-open-path.html).
- ⚠️ `im.dialog.messages.search`, `im.recent.get` и др. IM-методы (список последних чатов/непрочитанных) — в этой сессии не проверены напрямую; общий указатель методов чатов: https://apidocs.bitrix24.com/api-reference/chats/index.html
- ⚠️ Домен сервера авторизации: `oauth.bitrix24.tech` (ru-доки) vs `oauth.bitrix.info` (en-доки/примеры событий) — использовать значение `server_endpoint` из реальных ответов.
- ⚠️ Возможно ли для OAuth локального приложения указать `redirect_uri` на `localhost` (удобно для десктопа) — в доках не описано; documented `redirect_uri` берётся из настроек приложения (https://apidocs.bitrix24.ru/settings/oauth/).
- ⚠️ Суточные квоты REST-запросов — отсутствуют в актуальной странице лимитов (https://apidocs.bitrix24.com/settings/performance/limits.html).
- ⚠️ Доставляются ли системные события Б24 автоматически в pull-канал приложения — нет прямого подтверждения; документированная модель: канал наполняет сам сервер приложения (`pull.application.event.add`).

## Противоречия

- **Домен OAuth-сервера:** https://apidocs.bitrix24.ru/settings/oauth/ называет `oauth.bitrix24.tech`; https://apidocs.bitrix24.com/settings/oauth/auto-renewal.html и примеры `auth` в событиях (https://apidocs.bitrix24.com/api-reference/telephony/events/on-external-call-start.html) используют `oauth.bitrix.info`. Вероятно региональное/историческое расхождение — брать домен из ответа API.
- **Условия доступа к REST:** текст ошибки говорит «commercial plans; Vibe+ включают, Essentials нет» (https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify.html), а https://apidocs.bitrix24.ru/first-steps/access-to-rest-api.html требует именно подписки «Маркетплейс»/BitrixGPT+Маркетплейс. Тарифная сетка меняется — перед разработкой проверить конкретный тариф портала.
- **Старые vs новые IM-события:** старый имбот-стек (`ONIMBOTMESSAGEADD`, https://dev.1c-bitrix.ru/community/blogs/marketplace_apps24/we-present-a-platform-for-chatbots-for-bitrix24.php) против имбот.v2 (`ONIMBOTV2MESSAGEADD`, fetch/webhook, https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/events/event-get.html); старое объявлено устаревшим (https://apidocs.bitrix24.com/api-reference/chat-bots/outdated/messages/imbot-message-add.html).

---

## Источники

### Использованные (основные)
- Входящие и исходящие вебхуки — https://apidocs.bitrix24.ru/local-integrations/local-webhooks.html
- Локальные приложения — https://apidocs.bitrix24.ru/local-integrations/local-apps.html
- Callback установки локального приложения (токены одним POST) — https://apidocs.bitrix24.ru/settings/app-installation/local-apps/installation-callback.html
- Полный протокол OAuth 2.0 — https://apidocs.bitrix24.ru/settings/oauth/
- Автопродление токенов OAuth 2.0 — https://apidocs.bitrix24.com/settings/oauth/auto-renewal.html
- Доступ к REST API (тарифы/подписка Маркетплейс) — https://apidocs.bitrix24.ru/first-steps/access-to-rest-api.html
- Доступные скоупы — https://apidocs.bitrix24.ru/api-reference/scopes/permissions.html
- События: обзор (доставка, очередь, ограничения, права) — https://apidocs.bitrix24.com/api-reference/events/index.html
- event.bind (параметры, требования к обработчику, ошибки) — https://apidocs.bitrix24.ru/api-reference/events/event-bind.html
- event.get (типы подписок, только контекст приложения) — https://apidocs.bitrix24.ru/api-reference/events/event-get.html
- Список доступных событий — https://apidocs.bitrix24.ru/api-reference/events/events.html
- Офлайн-события (очередь, event.offline.*) — https://apidocs.bitrix24.ru/api-reference/events/offline-events.html
- ONCRMLEADADD — https://apidocs.bitrix24.ru/api-reference/crm/leads/events/on-crm-lead-add.html
- im.v2.Event.get — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-get.html
- im.v2.Event.subscribe — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-subscribe.html
- Обзор работы с чатом im.v2 — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/index.html
- imbot.v2.Event.get (fetch-режим бота) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/events/event-get.html
- imbot.v2: сообщения (устаревание imbot.*) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/messages/index.html
- im.notify — https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify.html
- Обзор методов уведомлений — https://apidocs.bitrix24.com/api-reference/chats/notifications/index.html
- im.message.add — https://apidocs.bitrix24.com/api-reference/chats/messages/im-message-add.html
- im.dialog.messages.get (чтение истории) — https://apidocs.bitrix24.com/api-reference/chats/messages/im-dialog-messages-get.html
- Собственный Push&Pull клиент (websocket/long polling) — https://apidocs.bitrix24.ru/settings/interactivity/custom-push-and-pull-client.html
- pull.application.config.get — https://apidocs.bitrix24.ru/settings/interactivity/pull-application-config-get.html
- pull.application.event.add — https://apidocs.bitrix24.ru/settings/interactivity/pull-application-event-add.html
- Телефония: обзор методов и событий — https://apidocs.bitrix24.ru/api-reference/telephony/index.html
- telephony.externalCall.register — https://apidocs.bitrix24.com/api-reference/telephony/telephony-external-call-register.html
- ONEXTERNALCALLSTART — https://apidocs.bitrix24.com/api-reference/telephony/events/on-external-call-start.html
- Туториал: интеграция внешней телефонии — https://apidocs.bitrix24.com/tutorials/telephony/index
- Лимиты REST API — https://apidocs.bitrix24.com/settings/performance/limits.html
- batch — https://apidocs.bitrix24.com/settings/how-to-call-rest-api/batch.html
- BX24.openPath (пример пути карточки сделки) — https://apidocs.bitrix24.ru/sdk/bx24-js-sdk/additional-functions/bx24-open-path.html
- Диплинк bx:// (botContext) и bitrix24:// (мобильные) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html
- REST_APP_URI (открытие приложения по ссылке) — https://apidocs.bitrix24.ru/api-reference/widgets/universal/app-url.html
- Пользовательская документация helpdesk: вебхуки и приложения — https://helpdesk.bitrix24.ru/open/20886106/; телефония — https://helpdesk.bitrix24.ru/open/6450911/
- Платформа чат-ботов (историческая, imbot-события) — https://dev.1c-bitrix.ru/community/blogs/marketplace_apps24/we-present-a-platform-for-chatbots-for-bitrix24.php

### Отклонённые/деприоритетные
- https://bitrixnotes.su/notes/app-otkryt-kartochku-crm-slaydere-prilozheniya/ — сторонний пересказ доков (SEO-заметка)
- https://aclips.ru/bitrix24-api/bx_crm_router_getdefaulturltemplates/ — неофициальное зеркало кода (использовано только для неподтверждённого паттерна URL)
- https://disweb.ru/…, https://telq.org/…, https://qna.habr.com/q/816493, https://www.linux.org.ru/forum/general/14303692 — форумы/Q&A, не авторитетны для контрактов API
- https://www.doc.budagov.ru/opendesktopapp_8php_source.html — зеркало исходников ядра; внутренняя реализация, не публичный контракт

## Next steps
- На живом портале проверить: фактический список событий (`events`), работу `im.v2.Event.subscribe/get` по вебхуку от имени владельца, паттерны URL карточек лида/контакта, поведение `redirect_uri` для десктоп-OAuth.
- Прототип polling-цикла (im.v2.Event.get + event.offline.get) с учётом 2 req/s и оценки расхода квоты.
- Если нужен строгий realtime — прототип гибрида «публичный HTTPS-эндпоинт → pull.application.event.add → websocket-клиент в плагине».

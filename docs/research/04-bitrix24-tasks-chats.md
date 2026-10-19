# Исследование: задачи и непрочитанные чаты Битрикс24 через входящий вебхук (демон-компаньон)

**Дата:** 2026-02. **Метод:** fetch_content официальных страниц apidocs.bitrix24.ru/.com + web_search; ключевые страницы прочитаны целиком (цитаты/примеры ниже — из прочитанного текста).
**Связка с прошлым исследованием:** docs/research/02-bitrix24-api-capabilities.md уже закрыл вебхуки/OAuth, механизм событий (event.bind, офлайн-очередь, im.v2 polling), лимиты, отправку сообщений и телефонию. Здесь добираются: методы задач, события задач, комментарии, дедлайны, deep links, непрочитанные чатов, упоминания. Дублирование намеренно минимально, на 02 есть ссылки.
**Пометки:** ✅ подтверждено прочитанной официальной страницей · 🔎 интерпретация/неофициальное подтверждение · ⚠️ не подтверждено.

---

## Summary

Задачи и чаты полностью закрываются **одним входящим вебхуком** со скоупами `task` + `im`: `tasks.task.list` даёт дельту по `CHANGED_DATE`, фильтры по ролям (RESPONSIBLE_ID / ACCOMPLICE / AUDITOR), мета-статус просрочки `STATUS=-1` и счётчики новых комментариев; непрочитанные чатов — `im.recent.list`/`im.recent.get` (per-chat `counter`), агрегат для бара — `im.counters.get`, уведомления и упоминания — `im.notify.get`. События задач (`ONTASKADD/UPDATE/DELETE/COMMENTADD`) существуют, но подписка на них — либо исходящий вебхук (нужен публичный HTTPS-обработчик), либо `event.bind` (только контекст приложения) — поэтому для webhook-only десктопа за NAT остаётся **только дельта-поллинг списком**. Гипотетические поля `START_DEADLINE`/`RESULT_DEADLINE` в актуальном API отсутствуют (в `tasks.task.getFields` их нет); реальные поля сроков — `DEADLINE`, `DATE_START`, `START_DATE_PLAN`, `END_DATE_PLAN`.

---

## 1. Задачи: методы и фильтры

### 1.1. tasks.task.list — основной метод дельта-поллинга
- ✅ Скоуп **`task`**, «Who can execute: any user»; доступ по правам (админ видит всё, руководитель — задачи подчинённых, остальные — доступные им). Примеры на странице включают **cURL (Webhook)** → метод работает через входящий вебхук. https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html
- ✅ **Фильтруемые поля** (полный список со страницы): `ID`, `PARENT_ID`, `GROUP_ID`, `CREATED_BY`, `STATUS_CHANGED_BY`, `PRIORITY`, `FORUM_TOPIC_ID`, **`RESPONSIBLE_ID`**, `TITLE` (по шаблону `[%_]`), `TAG`, **`REAL_STATUS`** (2 — ждёт выполнения, 3 — в работе, 4 — ждёт контроля, 5 — завершена, 6 — отложена), **`STATUS`** («статус для сортировки», соответствует полю `subStatus` ответа; те же значения + три мета-статуса: **`-1` — просрочена**, **`-2` — не просмотрена**, **`-3` — почти просрочена**), `MARK`, `SITE_ID`, `ADD_IN_REPORT`, `DATE_START`, **`DEADLINE`**, `CREATED_DATE`, `CLOSED_DATE`, **`CHANGED_DATE`**, **`ACCOMPLICE`** (соисполнитель — в фильтре в **единственном числе**), **`AUDITOR`** (наблюдатель), `DEPENDS_ON`, `ONLY_ROOT_TASKS`, `STAGE_ID`, `SPRINT_ID`, `BACKLOG_ID`, `UF_CRM_TASK`. Там же — URL.
- ✅ **Операторы перед полем:** `!` (не равно), `<`, `<=`, `>`, `>=`. Пример из доков: `filter: {"!STATUS": 6, ">=DEADLINE": "<сегодня>", "RESPONSIBLE_ID": 547, "::SUBFILTER-PARAMS": {"FAVORITE": "Y"}}`.
- ✅ **Пагинация:** параметр `start`; «размер страницы всегда фиксирован — 50 записей»; `start = (N-1)*50`; в ответе `total`. Там же.
- ✅ **select:** без `select` возвращается только `id`; «Always specify fields in select. The default set of fields may change».
- ✅ **params:** `WITH_RESULT_INFO`, `WITH_TIMER_INFO`, `WITH_PARSED_DESCRIPTION`.

### 1.2. tasks.task.get
- ✅ Скоуп `task`, любой пользователь; `taskId` + `select`; поле **`CHAT_ID`** (идентификатор чата задачи в новой карточке) возвращается по умолчанию — ключ к чтению комментариев (§3). Пример cURL (Webhook). https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-get.html

### 1.3. Поля задачи (select / ответ)
Источник — официальные страницы полей и getFields: https://apidocs.bitrix24.com/api-reference/tasks/fields.html , https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-get-fields.html
- ✅ Даты/сроки: `createdDate`, `changedDate` (ISO 8601), `activityDate` (дата последней активности), `dateStart`, **`deadline`**, `startDatePlan`, `endDatePlan` (в add/update передаются парой), `statusChangedDate`, `closedDate`, `viewedDate`.
- ✅ Статусы: `status` = 2/3/4/5/6 (+7 «declined» существует в модели, но может отсутствовать в списке значений getFields); `subStatus` в ответе list.
- ✅ Комментарии: **`commentsCount`** (число комментариев), `serviceCommentsCount` (системные), **`newCommentsCount`** (число **новых** комментариев — для текущего пользователя). В getFields — `COMMENTS_COUNT`, `SERVICE_COMMENTS_COUNT`, `NEW_COMMENTS_COUNT`.
- ✅ Роли: `responsibleId`, `accomplices` (массив), `auditors` (массив), `createdBy`, `group`/`groupId`.
- ✅ Прочее важное для демона: `notViewed` (Y/N — «задачу не смотрели»), `replicate` (Y — повторяющаяся), `isMuted`, `favorite`, `allowChangeDeadline`, `action` (какие действия доступны текущему пользователю).
- ✅ **Счётчики задачи** (в ответе `tasks.task.list` при select `COUNTERS`): объект `counter.counters` с полями `expired`, `newComments`, `projectExpired`, `projectNewComments`, `mutedExpired`, `mutedNewComments` + `color`/`value`. Пример ответа: https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html — готовые данные для «дедлайн просрочен» и «новый комментарий» прямо из списка.
- ⚠️ Полей **`START_DEADLINE`** и **`RESULT_DEADLINE`** в `tasks.task.getFields` (полный JSON-список полей прочитан) **нет**. Реальный набор сроков: `DEADLINE`, `DATE_START`, `START_DATE_PLAN`, `END_DATE_PLAN`. Считать упоминания этих полей в сторонних источниках ошибочными.

### 1.4. Фильтр «я ответственный ИЛИ соисполнитель ИЛИ наблюдатель одним запросом»
- ✅ Официально в актуальных доках `tasks.task.list` задокументирован только подсфильтр **`::SUBFILTER-PARAMS`** (пример с `FAVORITE=Y`): https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html
- 🔎 OR-логика вида `{"::LOGIC":"OR", "::SUBFILTER-1": {"RESPONSIBLE_ID": X}, "::SUBFILTER-2": {"ACCOMPLICE": X}, "::SUBFILTER-3": {"AUDITOR": X}}` документирована только в community-источниках на dev-портале (блог разработчика 1С-Битрикс с примером `::LOGIC`/`::SUBFILTER-1/2` по RESPONSIBLE_ID+ACCOMPLICE — https://dev.1c-bitrix.ru/community/webdev/user/11948/blog/9439/ — это про ядро `CTasks::GetList`/компонент, не REST-доки) и в问答 (https://qna.habr.com/q/1376262). Старая REST-документация dev.1c-bitrix.ru/rest_help теперь редиректит на новые apidocs — прежней страницы с описанием подсфильтров нет.
- 🔎 **Рабочий fallback без OR-логики:** batch из 3 запросов (`RESPONSIBLE_ID=X`, `ACCOMPLICE=X`, `AUDITOR=X`) + дедупликация по ID на клиенте, либо 3 обычных запроса (лимит 2 req/s позволяет). Проверить `::LOGIC` на живом портале одним вызовом.

### 1.5. Дельта-поллинг задач
- ✅ `CHANGED_DATE` — фильтруемое поле; в `order` доступны `CHANGED_DATE`, `ACTIVITY_DATE`, `DEADLINE`, `CREATED_DATE`, `CLOSED_DATE` и др. → дельта: `order: {"CHANGED_DATE":"desc"}, filter: {">CHANGED_DATE": "<последний опрос>", ...}`. https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html
- 🔎 Альтернатива/подстраховка: дельта по `ID` (`order ID desc`, `>ID` — ID фильтруется) — надёжно для «новых» задач; `>CHANGED_DATE` ловит и изменения существующих.
- 🔎 Не подтверждено, меняет ли добавление комментария в новой карточке `CHANGED_DATE` задачи (комментарий теперь — сообщение чата). Для «нового комментария» полагаться на `NEW_COMMENTS_COUNT`/`counter.counters.newComments` (§3), а не на `CHANGED_DATE`.

### 1.6. Скоуп и вебхук
- ✅ Старые методы задач — скоуп `task`; REST 3.0 (`/rest/api/`) — скоуп `tasks` («For old task methods, use the task scope, and for REST 3.0 task methods, the tasks scope»): https://apidocs.bitrix24.com/api-reference/tasks/tasks-new.html
- ✅ REST 3.0 вызывается по URL с сегментом `/rest/api/{user_id}/{rest_app_password}/tasks.task.get` — формат совпадает с вебхук-паттерном (пример на той же странице). То есть даже новые методы задач доступны вебхук-стилем. Там же.
- ⚠️ Наличие отдельного пункта «Задачи (tasks)» рядом с «Задачи (task)» в UI выбора прав входящего вебхука — не документировано; для v0 достаточно старых методов со скоупом `task`.

---

## 2. События задач и webhook-only

### 2.1. Состав событий
- ✅ Раздел «Задачи → события»: `OnTaskAdd`, `OnTaskUpdate`, `OnTaskDelete` (обзор: https://apidocs.bitrix24.com/api-reference/tasks/events-tasks/index.html; страницы: on-task-add, on-task-update, on-task-delete). Скоуп `task`, «Who can subscribe: any user».
- ✅ События комментариев: `OnTaskCommentAdd`, `OnTaskCommentUpdate`, `OnTaskCommentDelete` (обзор: https://apidocs.bitrix24.com/api-reference/tasks/comment-item/events-comment/ ; страница: https://apidocs.bitrix24.com/api-reference/tasks/comment-item/events-comment/on-task-comment-add.html). В новой карточке `OnTaskCommentUpdate/Delete` **не работают** (см. §3).
- ✅ **Как подписаться** (дословно с обзорных страниц событий задач и комментариев): «You can subscribe to task events through: **outbound webhook**; **application** and the method **event.bind**». https://apidocs.bitrix24.com/api-reference/tasks/events-tasks/index.html , https://apidocs.bitrix24.com/api-reference/tasks/comment-item/events-comment/

### 2.2. Ограничения доставки
- ✅ `event.bind` требует контекст приложения (вебхук → `WRONG_AUTH_TYPE`) — установлено в 02 (https://apidocs.bitrix24.ru/api-reference/events/event-bind.html); офлайн-подписка — только админ (02).
- ✅ Исходящий вебхук: «The event handler server must be accessible from the outside» — обработчик должен быть публичным HTTPS; повторных попыток доставки нет (02, https://apidocs.bitrix24.com/api-reference/events/index.html).
- ✅ Payload событий задач **не содержит полей задачи** — только `data.FIELDS_AFTER.ID` (у `ONTASKUPDATE`/`ONTASKADD`) и `IS_ACCESSIBLE_BEFORE/AFTER`; у `ONTASKCOMMENTADD`: `ID` (в новой карточке `0`), `TASK_ID`, `MESSAGE_ID` (новая карточка). Примеры: https://apidocs.bitrix24.com/api-reference/tasks/events-tasks/on-task-update.html , https://apidocs.bitrix24.com/api-reference/tasks/comment-item/events-comment/on-task-comment-add.html — в любом случае нужен довызов `tasks.task.get`/`tasks.task.list`.
- ✅ «Events will not be sent to the application until the installation is complete» (примечание на странице ONTASKCOMMENTADD) — актуально для приложений; у исходящего вебхука установки нет.

### 2.3. Вывод для webhook-only 🔎
Для локального демона без публичного HTTPS-эндпоинта события задач **недоступны** ни в каком виде: `event.bind` — только приложение, исходящий вебхук — только с публичным обработчиком (туннель/VPS — уже инфраструктура, см. 02). Рабочая схема — **дельта-поллинг `tasks.task.list`** (§1.5) с частотой 5–10 с: при 2 req/s (облако) и batch до 50 бюджет запросов позволяет 3–6 вызовов за цикл. Если у пользователя всё же есть публичный HTTPS (или туннель вида cloudflared/ngrok) — исходящий вебхук на `ONTASKADD/UPDATE/COMMENTADD` даст почти реалтайм без OAuth-приложения; payload всё равно требует довызова.

---

## 3. Комментарии

### 3.1. Два режима карточки задачи
- ✅ С версии модуля **`tasks 25.700.0`** действует **новая карточка задач**, где комментарий — это **сообщение чата задачи**: «In it, a comment is retained as a chat message, so some methods of the section do not work. Starting with the same version, development has stopped for the methods that create, read, update, and delete comments». https://apidocs.bitrix24.com/api-reference/tasks/tasks-new.html , https://apidocs.bitrix24.com/api-reference/tasks/comment-item/

### 3.2. Что читать вместо task.commentitem.*
- ✅ Таблица миграции (https://apidocs.bitrix24.com/api-reference/tasks/tasks-new.html):
  - **Читать список комментариев:** `task.commentitem.getlist` — «Does not work» → **`im.dialog.messages.get`** с `DIALOG_ID = chat{CHAT_ID}`.
  - Читать один: `task.commentitem.get` → ошибка; менять/удалять: `task.commentitem.update/delete` → `im.message.update/delete`.
  - Добавить: `task.commentitem.add` формально «works», но учитывает только `POST_MESSAGE` и `AUTHOR_ID`; для новых интеграций — **`tasks.task.chat.message.send`** (REST 3.0, `/rest/api/`, скоуп `tasks`).
- ✅ Поведение в новой карточке явно описано: «`task.commentitem.getlist` returns an empty list without an error, `task.commentitem.get` returns an error, and `update`/`delete` return the error `4/TE/ACTION_NOT_ALLOWED`» — https://apidocs.bitrix24.com/api-reference/tasks/comment-item/
- ✅ `CHAT_ID` — из `tasks.task.get` (возвращается по умолчанию) или `select: ["chat.id"]` в REST 3.0 (`/rest/api/`); в ответе `chat.entityType = "TASKS_TASK"`, `chat.entityId` = ID задачи. https://apidocs.bitrix24.com/api-reference/tasks/tasks-new.html
- ✅ Права: пользователь с правом чтения задачи может читать комментарии/список; `im.dialog.messages.get` требует участия в чате (02: https://apidocs.bitrix24.com/api-reference/chats/messages/im-dialog-messages-get.html — LIMIT max 50, `LAST_ID`/`FIRST_ID` для истории).
- ✅ Методы чатов — скоуп `im`; отправка в чат задачи — `im` (или `tasks` для tasks.task.chat.message.send). https://apidocs.bitrix24.com/api-reference/tasks/tasks-new.html

### 3.3. «В моей задаче новый комментарий» при дельта-поллинге
- ✅ Прямые индикаторы из списка задач: `newCommentsCount` (поле задачи, «Number of new comments» — относительно текущего пользователя, https://apidocs.bitrix24.com/api-reference/tasks/fields.html) и `counter.counters.newComments` (+`mutedNewComments`, `projectNewComments`) в select `COUNTERS` (https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html). Схема: запомнили значения по каждой задаче → при следующем цикле сравнили → тост «новый комментарий в задаче X» → при открытии демоном/юзером счётчик обнулится сам.
- 🔎 Служебные комментарии (`serviceCommentsCount`) учитываются отдельно — их можно отфильтровать, чтобы не ловить системные записи («задача завершена», смена статуса).
- ✅ Альтернативный источник: событие `ONTASKCOMMENTADD` (данные: `TASK_ID` + `MESSAGE_ID` в новой карточке) — но доступно только через исходящий вебхук/приложение (§2), не через входящий.
- ⚠️ Чтение **текста** комментария: `im.dialog.messages.get` по `chat{CHAT_ID}`; выборка «только новых» — по `LAST_ID` (метод возвращает сообщения чата постранично; отдельного фильтра «новые» нет) — 🔎 по описанию метода.

---

## 4. Дедлайны

- ✅ Поля сроков (getFields, полный список): **`DEADLINE`** (datetime), `DATE_START`, **`START_DATE_PLAN`**/`END_DATE_PLAN` (плановые начало/окончание; в add/update передаются вместе), `DURATION_PLAN`/`DURATION_FACT`/`DURATION_TYPE`. `START_DEADLINE`/`RESULT_DEADLINE` — отсутствуют (§1.3, ⚠️). https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-get-fields.html
- ✅ Флаг `ALLOW_CHANGE_DEADLINE` (может ли соисполнитель менять срок) и поле `allowChangeDeadline`; перед изменением срока проверять `action` (https://apidocs.bitrix24.com/api-reference/tasks/fields.html).
- ✅ **Просроченные одним фильтром:** мета-статус `STATUS = -1` («overdue task») в фильтре `tasks.task.list` — https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html . Альтернатива: `{"<DEADLINE": now, "!STATUS": [5,6]}` (в фильтре операторы `!`/`<`; статус 7 «declined» тоже исключать — 🔎 из описания статусов в fields.html).
- ✅ **«Дедлайн сегодня»:** `{"!STATUS": 6, ">=DEADLINE": "YYYY-MM-DD", "<=DEADLINE": "YYYY-MM-DD 23:59:59"}` — операторы диапазона и пример с `>=DEADLINE` в доках (пример официального cURL использует `>=DEADLINE` с сегодняшней датой; замечание в примере SDK: «UTC date; adjust to the account time zone if needed» — таймзону учитывать на клиенте).
- ✅ **Дедлайн-счётчики без фильтра:** select `COUNTERS` → `counter.counters.expired`/`projectExpired` (+`muted*`) и `color`/`value` — можно показывать в баре агрегат прямо из списка задач (https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html, пример ответа).
- ✅ **Повторяющиеся задачи:** флаг `REPLICATE` = Y в полях задачи (getFields); сами регулярные задачи создаются **по шаблонам по расписанию** — «Bitrix24 will generate new tasks based on these settings» (https://helpdesk.bitrix24.com/open/25859627/ , https://helpdesk.bitrix24.com/open/25859915/). Нюансы для демона:
  - ✅ «In recurring tasks, the deadline is calculated from the task creation date… The task deadline does not account for the user's time zone and is calculated based on server time» — https://helpdesk.bitrix24.com/open/25798163/ (FAQ Tasks). Т.е. у нового экземпляра дедлайн считается от даты создания, в серверном времени.
  - 🔎 Каждый новый экземпляр — **новая задача с новым ID** (создаётся по шаблону), поэтому дельта-поллинг по `>ID`/`>CHANGED_DATE` корректно подхватит её как «новую задачу»; отдельной обработки «повторов» не нужно, достаточно не терять старый ID (старый экземпляр после закрытия уходит из фильтра по статусам 5/6/7).
- ✅ В примерах dokов `>=DEADLINE` передаётся датой `YYYY-MM-DD` — сравнение работает и по дате, и полным datetime (страница tasks.task.list).

---

## 5. Deep links

### 5.1. Карточка задачи в вебе
- ✅ Официальный api_help (правила открытия ссылок в слайдере, `BX.SidePanel.Instance.bindAnchors`) перечисляет паттерны страниц задач:
  - `/company/personal/user/{userId}/tasks/task/view/{taskId}/`
  - `/workgroups/group/{groupId}/tasks/task/view/{taskId}/`
  - `/extranet/contacts/personal/user/{userId}/tasks/task/view/{taskId}/`
  — https://training.bitrix24.com/api_help/js_lib/sidepanel/click_links.php (зеркало: https://dev.1c-bitrix.ru/api_help/js_lib/sidepanel/click_links.php). Это официальная документация по разработке (api_help), не REST-справочник — но это первоисточник паттернов роутинга.
- ✅ Косвенное подтверждение из REST-доков: в ответе `tasks.task.list` ссылки на пользователей строятся как `/company/personal/user/547/` (пример ответа: `"link": "/company/personal/user/547/"`) — схема префикса совпадает. https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html
- 🔎 Третьи источники подтверждают вид URL: https://b24.org.ru/ssylki-v-bitriks24-v-2025-instruktsiya-po/ («https://ваш-портал.bitrix24.ru/workgroups/group/1/tasks/task/view/123/»), форум dev.1c-bitrix (/company/personal/user/919/tasks/task/view/87853/). Вывод: в браузере открывать `https://{портал}/company/personal/user/{мойID}/tasks/task/view/{taskID}/` (для групповой задачи — workgroups-вариант; личный префикс тоже работает для любых задач, т.к. это роут «моих задач» — 🔎 проверить на портале).

### 5.2. Открытие конкретного чата
- ✅ Официальный JS SDK: `Promise Messenger.openChat([String dialogId[, Integer messageId]])` — «opens a chat in the Bitrix24 Messenger interface», рекомендуется вместо устаревших `BX24.im.openMessenger`/`openHistory`; форматы dialogId: `userId`, `chatXXX`, `sgXXX` (https://apidocs.bitrix24.ru/sdk/bx24-js-sdk/additional-functions/messenger-open-chat.html; форматы — https://apidocs.bitrix24.com/sdk/bx24-js-sdk/additional-functions/outdated/bx24-im-open-messenger.html). **Но** это работает только внутри интерфейса Б24 (приложение во фрейме; из iframe-приложения — только мост `BX24.im.*`, https://apidocs.bitrix24.com/sdk/bx24-js-sdk/additional-functions/bx24-im-open-messenger.html) — для внешнего демона неприменимо напрямую.
- 🔎 Веб-URL конкретного чата: **`https://{портал}/online/?IM_DIALOG=chat{CHAT_ID}`** (личный диалог — `?IM_DIALOG={userId}`). Подтверждение: исходник ядра Битрикс (модуль socialnetwork, `Collab/Url/UrlManager.php`: `return $site->getDirectory() . 'online/?IM_DIALOG=chat' . $chatId;` и шаблон `'online/?IM_DIALOG=#DIALOG_ID#'` — зеркало https://doc.budagov.ru/socialnetwork_2lib_2_collab_2_url_2urlmanager_8php_source.html) + независимые сторонние инструкции (https://kuzmenov.ru/articles/bitrix/bitriks24-metody-dlya-raboty-s-chatami-cherez-api.html — «`<a href="/online/?IM_DIALOG=chat[CHAT_ID]">Открыть чат</a>`»; https://www.webformat.ru/cases/nuzhen-chat-v-glavnoe-menyu-delaem-za-tri-klika/). В официальных REST-доках этого паттерна нет — ⚠️ считать сильной гипотезой, проверить на своём портале (открыть любой чат в вебе и снять URL).
- ✅ Десктоп-клиент: официально задокументирован только `bx://v2/{portal}/botContext/...` (контекст бота); `bitrix24://` лишь выводит приложение на передний план, адрес не парсит (02: https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html). Прямого диплинка «открыть задачу/чат в десктопе» в доках нет — открывать через браузер; если десктоп-клиент Б24 зарегистрирован обработчиком `bx://`/перехватывает ссылки портала — 🔎 эмпирически (не контракт).

---

## 6. Непрочитанные чатов

### 6.1. im.recent.list — актуальный метод списка чатов
- ✅ Скоуп `im`, «any user»; «retrieves a list of the user's recent conversations **with pagination support**». Пример **cURL (Webhook)** на странице → работает через входящий вебхук. https://apidocs.bitrix24.com/api-reference/chats/im-recent-list.html
- ✅ Параметры: `SKIP_OPENLINES`, `SKIP_DIALOG`, `SKIP_CHAT`, `LAST_MESSAGE_DATE` (курсор), **`UNREAD_ONLY`** (`Y` — только диалоги с непрочитанными), `PARSE_TEXT`, `GET_ORIGINAL_TEXT`, `OFFSET`, **`LIMIT` (default 50, max 200)**.
- ✅ **Элемент содержит:** `id` (число для пользователя, `chatXXX` для чата), `chat_id`, `type` (user/chat), `title`, последнее `message` (id/text/author_id/date), **`counter`** — число непрочитанных в диалоге, `unread` (флаг «диалог помечен непрочитанным»), `pinned`, `has_reminder`, `date_update`, `date_last_activity`, объекты `user`/`chat`.
- ✅ **Ограничения пагинации** (официальные рекомендации): «The same dialogs may repeat between pages…»; `OFFSET` увеличивать на `LIMIT`; `SKIP_OPENLINES=Y` уменьшает пересечения; «If the list is small, request it in a single call with an increased LIMIT, with a maximum value of 200».
- ✅ В ответе также `hasMorePages`/`hasMore`.

### 6.2. im.recent.get — «сокращённый список», жив
- ✅ Метод задокументирован в актуальных доках (не удалён): скоуп `im`, любой пользователь; параметры `SKIP_*`, `LAST_UPDATE`, **`LAST_SYNC_DATE`** (дельта: «load changes that occurred in the list since the specified date», «returns data no older than 7 days»); элементы аналогичные (`counter`, `unread`, `message`, `date_update`); пример cURL (Webhook). https://apidocs.bitrix24.ru/api-reference/chats/im-recent-get.html (англ.: https://apidocs.bitrix24.com/api-reference/chats/im-recent-get.html)
- 🔎 Для демона удобен как «дёшево узнать, что изменилось с прошлого опроса» (`LAST_SYNC_DATE`), но выборка ограничена 7 днями и не даёт стабильной пагинации; для полного снапшота непрочитанных — `im.recent.list`.

### 6.3. im.counters.get — агрегат для бара
- ✅ Скоуп `im`, любой пользователь, без параметров; пример cURL (Webhook). Возвращает:
  - `TYPE`: `ALL` (все непрочитанные + уведомления), **`NOTIFY`** (непрочитанные уведомления), `CHAT` (групповые чаты), `LINES` (открытые линии), `DIALOG` (личные диалоги), `MESSENGER`;
  - `CHAT`: карта `{chatId: кол-во непрочитанных}`, `CHAT_MUTED` (чаты без звука), `CHAT_UNREAD` (чаты в статусе «непрочитанный»), `DIALOG`: `{userId: кол-во}`, `DIALOG_UNREAD`.
  — https://apidocs.bitrix24.com/api-reference/chats/im-counters-get.html . Один вызов = и бар-счётчик «чаты», и «уведомления» (колокольчик), и разбивка по чатам.
- ⚠️ `im.dialog.get` с полем unread как альтернатива не проверялся напрямую; у объекта `Chat` (im.v2/imbot.v2) есть поле `unreadId` — «ID of the first unread message», возвращается только в ответах методов (https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/entities.html). Основной путь — `im.recent.*` + `im.counters.get`.

---

## 7. Упоминания и уведомления

### 7.1. Маркер упоминания в ONIMV2MESSAGEADD
- ✅ Формат события `ONIMV2MESSAGEADD` (доставка через `im.v2.Event.get`, polling): `data = {message, chat, user, language}`; объект `message`: `id`, `chatId`, `authorId`, `date`, `text`, `isSystem`, `uuid`, `forward`, **`params`** («Additional parameters: attach, keyboard, files, and others»), `viewedByOthers`. **Отдельного поля/маркера «упоминание» в документированной структуре нет.** https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/events.html , объекты: https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/entities.html
- ✅ Упоминание в тексте сообщения — BB-код **`[USER=id]Имя[/USER]`** (и `[USER=all]...[/USER]` — все участники чата), официально задокументирован в «Message Formatting»; «When you retrieve the message… it returns the same text with the codes» → в payload события `text` придёт с кодами. https://apidocs.bitrix24.com/api-reference/chats/messages/formatting.html
- 🔎 Детект упоминаний для v0: в сообщении из `ONIMV2MESSAGEADD` искать `[USER={мой_id}]`, `[USER=all]` (в групповых чатах — «все», в личке любое сообщение от другого = «личное упоминание»). `params` в примере события пустой — undocumented содержимое не полагается.
- ⚠️ «Меня упомянули» можно надёжнее получать из уведомлений (§7.2): упоминание генерирует уведомление юзеру.

### 7.2. Упоминания = уведомления; чтение уведомлений через вебхук
- ✅ Упоминание генерирует уведомление: «Если упомянуть сотрудника через @ в открытом канале, в котором он не состоит, ему придет уведомление со ссылкой на конкретный комментарий» — https://helpdesk.bitrix24.ru/open/21297736/ ; центр уведомлений покрывает «новые задачи, комментарии, встречи или изменения в CRM» — https://helpdesk.bitrix24.ru/open/26929068/ . Для личных/групповых чатов упоминание → уведомление — стандартное поведение мессенджера (🔎, в REST-доках явно не сказано).
- ✅ **`im.notify.get`** — скоуп `im`, «any user», **пример cURL (Webhook)** → читается через входящий вебхук. Возвращает `total_count`, **`total_unread_count`**, `chat_id` (системный чат уведомлений), `notifications[]`: `id`, `chat_id`, `author_id`, `date`, `notify_type`, **`notify_module`**, **`notify_event`**, `notify_tag`, `notify_sub_tag`, `notify_title`, `setting_name` (формат `MODULE|EVENT`), `text`, **`notify_read`** (`Y`/`N`), `notify_buttons`, `params`; плюс `users[]` с данными авторов. Пагинация: `LAST_ID` + `LAST_TYPE` (1 — продолжить с «подтверждений», 3 — с обычных уведомлений), `LIMIT` default/max 50, сортировка по дате убыв. https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify-get.html
- ✅ Счётчик непрочитанных уведомлений — `im.counters.get` → `TYPE.NOTIFY` (§6.3) — дёшево детектить «появилось новое в колокольчике», потом дотягивать детали `im.notify.get`.
- ✅ Прочее семейство: `im.notify.personal.add`, `im.notify.system.add`, `im.notify.read.list`, `im.notify.read`, `im.notify.read.all`, `im.notify.delete`, `im.notify.history.search` и др. — https://apidocs.bitrix24.com/api-reference/chats/notifications/index.html (перечислено в 02).
- 🔎 В polling-канале `im.v2.Event.get` типов событий уведомлений нет (только MESSAGE*/REACTION/JOINCHAT, §7.1) — «новое уведомление» демону придётся ловить поллингом `im.notify.get` (по `LAST_ID`/`total_unread_count`) либо `im.counters.get`.
- ⚠️ Точный код `notify_event`/`notify_module` у уведомлений об упоминании в чате (ожидаемо `im`) в доках не зафиксирован — снять с живого портала и закешировать.

---

## Вывод: что реально для v0 «задачи + чаты» через один вебхук

**Авторизация:** один входящий вебхук владельца-юзера с правами **`task` + `im`** (создать может админ или юзер с разрешённым правом; вебхук действует от имени своего владельца и в рамках его прав — 02: https://apidocs.bitrix24.ru/local-integrations/local-webhooks.html). Без OAuth, без публичного сервера. Тариф — коммерческий с REST (02).

**Polling-цикл (каждые 5–10 с, batch-пакеты):**
1. `im.v2.Event.get` (после разового `im.v2.Event.subscribe`, 02) → тосты о новых сообщениях; упоминания — парсингом `[USER=<мой_id>]`/`[USER=all]` в `text`.
2. `im.counters.get` → бар: `TYPE.NOTIFY` (уведомления), `TYPE.CHAT`/`TYPE.DIALOG` (непрочитанные чаты) + разбивка по чатам.
3. `im.recent.list` (`UNREAD_ONLY=Y`, LIMIT 200) → список непрочитанных чатов с `counter` и последним сообщением (для тостов/детализации бара). Альтернатива для дельты — `im.recent.get` c `LAST_SYNC_DATE` (окно 7 дней).
4. `im.notify.get` (`LAST_ID`, `LAST_TYPE`) → тексты уведомлений (упоминания, назначение задач, комментарии) + `total_unread_count`.
5. `tasks.task.list`:
   - **дельта моих задач:** `order {CHANGED_DATE desc}` + `filter {">CHANGED_DATE": T}` в трёх ролях (`RESPONSIBLE_ID`/`ACCOMPLICE`/`AUDITOR` = мой ID) — 3 запроса или batch-50; проверить `::LOGIC:"OR"` + `::SUBFILTER-N` (⚠️ недокументировано);
   - **«назначили задачу»:** в дельте флаг «новая» (`CREATED_DATE > T` или `ID > lastMaxId`) при `RESPONSIBLE_ID = я`;
   - **«комментарий в моей задаче»:** рост `newCommentsCount`/`counter.counters.newComments` относительно прошлого опроса;
   - **«дедлайн сегодня»: `filter {"!STATUS":6, ">=DEADLINE": сегодня, "<=DEADLINE": сегодня 23:59}`; «просрочено»: `filter {"STATUS": -1}`** (мета-статус) — по ролям аналогично;
   - select: `ID, TITLE, STATUS, subStatus, DEADLINE, CHANGED_DATE, RESPONSIBLE_ID, ACCOMPLICES, AUDITORS, GROUP_ID, COMMENTS_COUNT→commentsCount, NEW_COMMENTS_COUNT→newCommentsCount, COUNTERS, notViewed`.
6. Чтение текста комментария: `tasks.task.get` → `CHAT_ID` → `im.dialog.messages.get` (`chat{CHAT_ID}`).

**Бюджет:** ~4–6 запросов/цикл при 2 req/s — с запасом; batch 50 покрывает все вызовы цикла одним HTTP-запросом (02: https://apidocs.bitrix24.com/settings/how-to-call-rest-api/batch.html).

**Deep links для тостов:** задача → `https://{портал}/company/personal/user/{N}/tasks/task/view/{M}/` ✅; чат → `https://{портал}/online/?IM_DIALOG=chat{ID}` 🔎 (проверить live); уведомление → колокольчик `https://{портал}/online/` (чат уведомлений).

**Чего нельзя без приложения/сервера:** push-события задач (ONTASK*) без публичного HTTPS-обработчика; `event.bind`/офлайн-очередь без приложения+админа; маркер упоминания как отдельное поле события; фильтрация списка задач по комментариям (только счётчики в select).

---

## Противоречия
- **`START_DEADLINE`/`RESULT_DEADLINE`:** встречаются в постановке задачи (ожидания) как «поля дедлайнов», но в полном официальном списке полей `tasks.task.getFields` их нет (прочитан целиком: https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-get-fields.html) — есть `DEADLINE`, `START_DATE_PLAN`, `END_DATE_PLAN`, `DATE_START`.
- **OR-логика фильтра:** официально (apidocs) документирован только `::SUBFILTER-PARAMS`; `::LOGIC`/`::SUBFILTER-N` подтверждены лишь community-блогом на dev-портале (https://dev.1c-bitrix.ru/community/webdev/user/11948/blog/9439/) и Q&A (https://qna.habr.com/q/1376262) — для REST `tasks.task.list` не подтверждено документально.
- **Старые comment-методы:** страница `task.commentitem.getlist` в RU-доках помечена «Развитие метода остановлено… не работает в новой карточке задач» (https://apidocs.bitrix24.ru/api-reference/tasks/comment-item/task-comment-item-get-list.html), а в статус-таблице EN-доков `task.commentitem.add` числится «Works» (https://apidocs.bitrix24.com/api-reference/tasks/comment-item/) — т.е. «add» ещё жив, но весь reading-путь уже чат-методы. Различать: писать через старое можно, читать — нельзя.
- **im.recent.get vs im.recent.list:** оба живы и документированы; `get` — без OFFSET/LIMIT (курсор `LAST_UPDATE`/`LAST_SYNC_DATE`, окно 7 дней), `list` — полноценная пагинация + `UNREAD_ONLY`. Рекомендация доков для маленьких списков — один вызов `list` с LIMIT 200.

## Missing evidence
- Официального описания веб-URL `/online/?IM_DIALOG=...` в доках нет — подтверждение только исходником ядра (зеркало) и сторонними статьями; проверить на портале.
- Меняет ли добавление комментария `CHANGED_DATE` задачи в новой карточке (влияет на надёжность дельты по времени) — не подтверждено; заложиться на счётчики.
- `notify_event`/`notify_module` уведомлений об упоминании и о назначении задачи — не зафиксированы в доках (поля есть, значения — снять live).
- Доступность REST 3.0 (`/rest/api/`, скоуп `tasks`) именно через входящий вебхук на реальном портале (пример URL в доках вебхук-формата есть, пункта UI не видели).
- Работоспособность `::LOGIC`/`::SUBFILTER-N` в облачном REST (vs коробка) — проверить одним запросом.
- Реакция `im.recent.*`/`im.counters.get` на вебхук от имени юзера, который не участник каких-то чатов, — ожидаемо отдаёт только своё (02: REST не расширяет права), но не проверено.

## Sources
### Использованные (основные, прочитаны целиком или ключевые разделы)
- tasks.task.list (фильтры, операторы, мета-статусы, пагинация, COUNTERS, вебхук-пример) — https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-list.html (RU: https://apidocs.bitrix24.ru/api-reference/tasks/tasks-task-list.html)
- tasks.task.get (CHAT_ID по умолчанию, select) — https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-get.html
- Поля задачи (статусы, commentsCount/newCommentsCount, replicate, allowChangeDeadline) — https://apidocs.bitrix24.com/api-reference/tasks/fields.html (RU: https://apidocs.bitrix24.ru/api-reference/tasks/fields.html)
- tasks.task.getFields (полный список полей: DEADLINE/COMMENTS_COUNT/NEW_COMMENTS_COUNT/REPLICATE; отсутствие START_DEADLINE/RESULT_DEADLINE) — https://apidocs.bitrix24.com/api-reference/tasks/tasks-task-get-fields.html
- Новая карточка задач: миграция комментариев в чат, CHAT_ID/DIALOG_ID, REST 3.0 /rest/api/, скоупы task/tasks, события OnTaskComment* — https://apidocs.bitrix24.com/api-reference/tasks/tasks-new.html (RU: https://apidocs.bitrix24.ru/api-reference/tasks/tasks-new.html)
- Комментарии: обзор методов и их статус в новой карточке — https://apidocs.bitrix24.com/api-reference/tasks/comment-item/
- task.commentitem.getlist (deprecated; RU-вариант с явным запретом) — https://apidocs.bitrix24.ru/api-reference/tasks/comment-item/task-comment-item-get-list.html
- События задач: обзор + подписка (исходящий вебхук/event.bind) — https://apidocs.bitrix24.com/api-reference/tasks/events-tasks/index.html
- OnTaskAdd / OnTaskUpdate (payload: только ID) — https://apidocs.bitrix24.com/api-reference/tasks/events-tasks/on-task-add.html , https://apidocs.bitrix24.com/api-reference/tasks/events-tasks/on-task-update.html
- OnTaskCommentAdd (payload: TASK_ID, MESSAGE_ID, ID=0 в новой карточке) — https://apidocs.bitrix24.com/api-reference/tasks/comment-item/events-comment/on-task-comment-add.html , обзор событий комментариев — https://apidocs.bitrix24.com/api-reference/tasks/comment-item/events-comment/
- im.recent.list (пагинация, UNREAD_ONLY, counter/unread, лимит 200) — https://apidocs.bitrix24.com/api-reference/chats/im-recent-list.html
- im.recent.get (LAST_SYNC_DATE-дельта, 7 дней) — https://apidocs.bitrix24.com/api-reference/chats/im-recent-get.html (RU: https://apidocs.bitrix24.ru/api-reference/chats/im-recent-get.html)
- im.counters.get (TYPE.NOTIFY/CHAT/DIALOG, карты по чатам, CHAT_MUTED) — https://apidocs.bitrix24.com/api-reference/chats/im-counters-get.html
- im.notify.get (уведомления: total_unread_count, notify_read, notify_module/event, пагинация LAST_ID+LAST_TYPE) — https://apidocs.bitrix24.com/api-reference/chats/notifications/im-notify-get.html
- im.v2.Event.get (типы событий, offset, эксклюзивность) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/event-get.html
- Форматы im.v2 событий (ONIMV2MESSAGEADD: message/chat/user; params без маркера упоминания) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/im.v2/events/events.html
- Объекты im.v2/imbot.v2 (Message.params, Chat.unreadId) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/entities.html
- BB-коды сообщений ([USER=id] — упоминание) — https://apidocs.bitrix24.com/api-reference/chats/messages/formatting.html
- Паттерны URL страниц задач (api_help, bindAnchors) — https://training.bitrix24.com/api_help/js_lib/sidepanel/click_links.php (RU: https://dev.1c-bitrix.ru/api_help/js_lib/sidepanel/click_links.php)
- Messenger.openChat (JS SDK, dialogId/messageId) — https://apidocs.bitrix24.ru/sdk/bx24-js-sdk/additional-functions/messenger-open-chat.html
- Регулярные задачи (создаются по шаблонам по расписанию) — https://helpdesk.bitrix24.com/open/25859627/ , https://helpdesk.bitrix24.com/open/25859915/ ; дедлайн экземпляра от даты создания, серверное время — https://helpdesk.bitrix24.com/open/25798163/
- Упоминание @ → уведомление (каналы) — https://helpdesk.bitrix24.ru/open/21297736/ ; центр уведомлений (задачи/комментарии/CRM) — https://helpdesk.bitrix24.ru/open/26929068/
- OR-логика фильтра задач (community, ::LOGIC/::SUBFILTER) — https://dev.1c-bitrix.ru/community/webdev/user/11948/blog/9439/ ; аналогично https://qna.habr.com/q/1376262

### Отклонённые/деприоритетные
- https://b24.org.ru/ssylki-v-bitriks24-v-2025-instruktsiya-po/ — сторонняя SEO-инструкция (использована только как корроборация URL-паттерна задач)
- https://kuzmenov.ru/…, https://www.webformat.ru/cases/nuzhen-chat-v-glavnoe-menyu-delaem-za-tri-klika/ — сторонние статьи про `/online/?IM_DIALOG=` (корроборация к исходнику ядра)
- https://doc.budagov.ru/socialnetwork_2lib_2_collab_2_url_2urlmanager_8php_source.html — зеркало исходников ядра (не публичный контракт; использовано для IM_DIALOG)
- https://habr.com/ru/articles/427451/ — Хабр-статья (подтверждает только пункт «Задачи (task)» в правах вебхука)
- https://qna.habr.com/q/1330230, https://telq.org/… — форумы/Q&A, не авторитетны
- Старая REST-документация dev.1c-bitrix.ru/rest_help/* — редиректит на новые apidocs, напрямую недоступна (в 02 это уже отмечено)

## Next steps
- На живом портале: один вызов `tasks.task.list` с `{"::LOGIC":"OR","::SUBFILTER-1":{"RESPONSIBLE_ID":X},"::SUBFILTER-2":{"ACCOMPLICE":X},"::SUBFILTER-3":{"AUDITOR":X}}`; проверить, приходит ли `CHANGED_DATE` после добавления комментария; снять `notify_event` уведомления об упоминании; проверить `/online/?IM_DIALOG=chat{ID}` и наличие «tasks» в UI прав вебхука.
- Прототип цикла: `im.v2.Event.subscribe` + batch(`im.counters.get`, `im.recent.list UNREAD_ONLY=Y`, `tasks.task.list` дельты) с учётом 2 req/s.

# Research: Bitrix24 под Linux — карта существующих решений

> Контекст: пользователь на Omarchy (Arch Linux + Hyprland + Wayland) думает писать десктоп-плагин/клиент для Bitrix24. Ниже — карта того, что уже существует (по состоянию на момент исследования; даты — как указаны в источниках).
>
> Метод: web_search + fetch_content по первоисточникам (helpdesk.bitrix24.ru/.com, bitrix24.ru, apidocs.bitrix24.com, AUR, GitHub, форумы). Прямые цитаты и выводы разведены: «подтверждено» = прямое свидетельство из источника; «инференс» = мой вывод; «не подтверждено» = не найдено.

## Summary

Официальный десктоп-клиент Битрикс24 **уже существует и для Linux** (DEB/RPM, релиз — июль 2022, статья helpdesk обновлялась в 2026). Для Arch есть AUR-пакет — перепаковка официального DEB с dl.bitrix24.com. Веб-версия официально работает в Chrome 80+ (включая звонки и демонстрацию экрана), шаринг экрана под Wayland решается через PipeWire + xdg-desktop-portal (в Hyprland — xdg-desktop-portal-hyprland). Поэтому «писать клиент с нуля» не нужно: свободные ниши — Wayland-нативность и шаринг экрана из самого Linux-клиента, упаковка (Flatpak/AppImage/актуальный PKGBUILD), Ferdium-рецепт/нотификатор, работа с диплинками и компенсация ограничения «маркетплейс-приложения работают только в веб-версии».

---

## 1. Официальное

### 1.1. Linux-версия существует (главный факт, опровергающий исходную посылку)

- **Подтверждено (первоисточник):** «Мы рады сообщить, что выпустили приложение Битрикс24 для Linux-систем. Приложение Битрикс24 для Linux имеет все те же функции, что и приложение для Windows и macOS – задачи, чаты, звонки, диск, а также работу в двухоконном режиме». Статья helpdesk с пометкой «Последнее обновление: 24 июля 2026 г.». — [Приложение Битрикс24 для Linux, helpdesk.bitrix24.ru](https://helpdesk.bitrix24.ru/open/16171806/); EN-зеркало: [Bitrix24 Messenger for Linux, helpdesk.bitrix24.com](https://helpdesk.bitrix24.com/open/16172002/) (Last update: October 05, 2026).
- **Подтверждено:** релиз был объявлен в блоге Битрикс24 20 июля 2022 (обновл. 22.07.2022): «Десктопное приложение уже выпущено и доступно… в расширениях .deb и .rpm». — [Приложение Битрикс24 для Linux, bitrix24.ru/blogs](https://www.bitrix24.ru/blogs/prilozhenie-bitriks24-dlya-linux.php).
- **Контекст:** Linux-версия появилась не сразу — до релиза была beta (см. боль №5.1: тред 2021 г. про «bitrix24_desktop_6_0_45_40.tgz» для Linux).

### 1.2. Поддерживаемые дистрибутивы (официальный список)

- **Подтверждено:** DEB — Ubuntu 20/21/22/24.04, Kubuntu 20/22, Mint 20/21, Astra Linux 2.12; RPM — Fedora 35/36 (Gnome), Альт Рабочая станция 10, Альт 8СП Рабочая станция, Rosa 21.1, OpenSUSE Leap 15.4. — [helpdesk.bitrix24.ru/open/16171806](https://helpdesk.bitrix24.ru/open/16171806/).
- **Arch Linux в списке нет** — официальной поддержки Arch не заявлено. Для Arch сообщество пакует само (см. раздел 2.1). Список дистрибутивов заметно отстаёт (Fedora 35/36 — давно неактуальные версии) — интерпретация источника, не цитата.
- Скачивание: «Приложение Битрикс24 для Linux вы можете скачать в виде DEB- и RPM-пакетов на странице загрузок» → [bitrix24.ru/features/desktop.php](https://www.bitrix24.ru/features/desktop.php). На странице загрузок [bitrix24.ru/features/downloads/](https://www.bitrix24.ru/features/downloads/) в поисковой выдаче видны кнопки «Для Linux DEB» и «Для Linux RPM» (в прямом fetch кнопки-картинки не отрендерились — отмечаю как свидетельство из поискового кэша страницы, не из прямой выгрузки). Прямой URL официального пакета подтверждён через PKGBUILD в AUR: `https://dl.bitrix24.com/b24/bitrix24_desktop.deb` — [PKGBUILD AUR bitrix24](https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=bitrix24).

### 1.3. Что умеет десктоп-клиент (Win/macOS/Linux)

- **Подтверждено:** мессенджер с чатами, видеозвонки и конференции «прямо из чата», до 100 участников, безлимитная длительность; уведомления; CRM/задачи/диск «в одном окне»; синхронизация Bitrix24.Drive. — [bitrix24.com/apps/desktop.php](https://www.bitrix24.com/apps/desktop.php).
- **Подтверждено:** новый «all-in-one» десктоп: «Two separate windows with your Bitrix24 account in one and chats the other», тёмная тема в чате, CRM/Webmail/Аналитика/Drive внутри; двухоконный режим можно отключить в настройках. — [блог new-all-in-one-bitrix24-desktop-app](https://www.bitrix24.com/amp/blogs/applications/new-all-in-one-bitrix24-desktop-app.php).
- **Подтверждено:** системные требования: Windows 10 / Windows Server 2016+, macOS 12+; с версии 15+ — только 64-bit Windows. — [Download and install Bitrix24 Messenger](https://helpdesk.bitrix24.com/open/20819676/).
- **Подтверждено:** в Windows-установке MSI требуется драйвер Dokan для «эластичного диска» (Elastic drive) — [тот же helpdesk](https://helpdesk.bitrix24.com/open/20819676/). **Не подтверждено:** доступна ли синхронизация Диска (тем более эластичный диск) в Linux-версии; статья о синхронизации диска говорит про «Битрикс24 для Windows и Mac» — [helpdesk.bitrix24.ru/open/19941088](https://helpdesk.bitrix24.ru/open/19941088/).
- **Подтверждено:** служба поддержки принимает логи и с Linux-клиента («macOS and Linux: In the Bitrix24 app menu, click Support > Send logs») — [Send Bitrix24 Messenger app logs](https://helpdesk.bitrix24.com/open/20535138/), т.е. Linux-клиент — полноценная поддерживаемая платформа.

### 1.4. Ссылки-диплинки: `bx://` (а не `bx24://`)

- **Подтверждено (REST-документация):** у десктоп-приложения есть диплинк-протокол `bx://`. Задокументированный формат — для передачи контексту боту при открытии чата: `bx://v2/{portal}/botContext/dialogId/{dialogId}/context/{urlEncodedJson}`. «The `bx://` protocol belongs to the desktop application» — мобильные приложения его не обрабатывают; протокол `bitrix24://` лишь выводит приложение на передний план, но не парсит адрес. Определить извне, установлен ли у пользователя десктоп, нельзя. — [Passing Context to the Bot, apidocs.bitrix24.com](https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html), RU: [apidocs.bitrix24.ru](https://apidocs.bitrix24.ru/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html).
- **Не подтверждено:** общего (не bot-специфичного) каталога `bx://`-ссылок в открытой документации не нашёл; задокументирован только бот-контекст.

### 1.5. Критическое ограничение платформы: маркетплейс-приложения только в веб-версии

- **Подтверждено:** «For optimal integration app performance, it is essential to utilize the web version of Bitrix24. Please note that desktop and mobile versions are not compatible with integration apps». — [Troubleshooting common issues with Bitrix24 integration apps](https://helpdesk.bitrix24.com/open/17056078/). Это значит: виджеты приложений Маркета (включая телефонию-виджеты) не работают внутри десктоп-клиента на любой ОС — не только на Linux. Для проектирования плагина это ключевое ограничение.

---

## 2. Неофициальное / опенсорс

### 2.1. AUR: `bitrix24` — перепаковка официального DEB (главный вариант для Arch)

- **Подтверждено:** пакет [AUR bitrix24](https://aur.archlinux.org/packages/bitrix24) — версия 23.0.32.91-2, 8 голосов, первый сабмит 2022-09-01, последнее обновление 2026-07-24, мейнтейнеры Ganin/begati/Ge1i0N, сабмиттер angrycore.
- **Подтверждено (PKGBUILD):** исходник — официальный `https://dl.bitrix24.com/b24/bitrix24_desktop.deb` (URL без версии + зафиксированный sha256), распаковка `data.tar.xz` в `/opt/Bitrix24`, бинарники `Bitrix24-web` и `BDisk`, зависимости: `gtk3 libxss libappindicator-gtk3 nss alsa-lib libx11 glibc`. — [PKGBUILD](https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=bitrix24).
- **Инференс:** набор зависимостей (libx11, gtk3, libappindicator) указывает на Electron-приложение, собранное под X11; под Hyprland оно, скорее всего, запускается через XWayland по умолчанию (нативный Wayland через `--ozone-platform=wayland`/`ELECTRON_OZONE_PLATFORM_HINT` документацией Б24 не оговорён). Трей — через libappindicator → StatusNotifierItem (работает в waybar).
- **Инференс:** фиксированный sha256 при безверсионном URL означает, что при молчаливом обновлении DEB на dl.bitrix24.com AUR-пакет ломается до ручного обновления хеша — типовая хрупкость таких перепаковок.
- Также есть Ansible-роль, ставящая официальный Linux-DEB на Debian-подобные: [webarchitect609/ansible-role-bitrix24](https://github.com/webarchitect609/ansible-role-bitrix24) (последний пуш 2023-11).

### 2.2. GitHub-проекты (обёртки/альтернативные клиенты)

Поиск по GitHub (`in:name`, сортировка по звёздам) показывает, что экосистемы живых обёрток нет — доминируют SDK/обёртки REST API:

- [smallcms/infinitrix](https://github.com/smallcms/infinitrix) — «open source Bitrix24 messenger client» на Python/PyQt5/PyQtWebEngine; 6★; создан 2022-05, последний пуш 2023-11 — фактически заброшен.
- [MaxNeutrino/bitrix24-electron](https://github.com/MaxNeutrino/bitrix24-electron) — «Bitrix24 launcher» на Electron; 0★; 2018 год — мёртв.
- [Maroosek/Bitrix24_TimeTrack](https://github.com/Maroosek/Bitrix24_TimeTrack) — «Desktop app for bitrix24 to manage workers tasks more easily (~85% working incl. chat history)», Python; последний пуш 2026-10 — живой, но нишевый (тайм-трекинг).
- **Отбраковано:** репозиторий «Bitrix24-Desktop-App/Bitrix24-Mac» с описанием «Download Bitrix24 for Windows and Mac» — типичный SEO-мусор, не реальный клиент и не официальный (официальная организация — `bitrix24/`).
- **Вывод (инференс):** готовый «каркас» для форка под Linux отсутствует; свободная ниша реальна, но и потребность в ней подтверждена слабо (см. раздел 4).

### 2.3. Snap / Flatpak / AppImage

- **Подтверждено:** два неофициальных snap-пакета, оба заброшены: [snapcraft.io/bitrix24](https://snapcraft.io/bitrix24) («Bitrix24 (Beta)», паблишер ThePanaceya, «This is unofficial snap», последняя публикация 15.12.2021, помечен unmaintained) и [snapcraft.io/bitrix](https://snapcraft.io/bitrix) (beta-канал, не обновлялся с 10.06.2022).
- **Не найдено:** официального или стороннего Flatpak на Flathub (поиск ничего не возвращает, только страницы документации Flathub) — [flathub.org](https://flathub.org/en). **AppImage** от Б24/сообщества — не найдено.

### 2.4. Мульти-мессенджеры (Ferdium / Rambox / Wavebox)

- **Подтверждено:** в [ferdium-recipes](https://github.com/ferdium/ferdium-recipes) рецепта для Bitrix24 **нет** (путь `recipes/bitrix24` отсутствует в репо — проверено клоном/структурой каталога; в ветке «b…» соседние рецепты `binance`, `bip`, а bitrix отсутствует).
- **Подтверждено:** Ferdium поддерживает «Custom Website» — «Add any web-based service, simple URL-based configuration, full notification support» — [ferdium.app/features.html](https://ferdium.app/features.html), FAQ: [ferdium.org/faq](https://ferdium.org/faq). Т.е. обёртка Б24 в Ferdium возможна без рецепта, но без интеграции счётчика непрочитанных/тёмной темы (то, что даёт рецепт).
- **Подтверждено:** в списке сервисов Rambox Community Edition (`resources/services.json`) Bitrix24 отсутствует — проверено по raw-файлу. Rambox позволяет «add any other website as a custom app» — [Rambox Help Center](https://support.rambox.app/getting-started/how-to-create-and-configure-apps.html).
- **Не подтверждено:** наличие Bitrix24 в проприетарном Wavebox (закрытый каталог, напрямую не проверил).
- **Инференс:** как обёртка «годится» на уровне «открыть веб-версию в отдельном окне с уведомлениями браузерного типа»; звонки WebRTC внутри таких обёрток — тот же Chromium, т.е. те же ограничения, что и в браузере.

---

## 3. Веб-версия и PWA

- **Подтверждено:** видеозвонки/конференции работают в браузере; официально рекомендуемый браузер для не-macOS — **Google Chrome 80+**, «Other browsers may work, but full functionality is not guaranteed» — [Fix video conferencing issues](https://helpdesk.bitrix24.com/open/11417394/), [Technical requirements for video calls](https://helpdesk.bitrix24.com/open/21205364/). Демонстрация экрана — штатная кнопка Screen в окне звонка, выбор экрана/окна/вкладки в стандартном диалоге Chrome — [Bitrix24 Sync/calls helpdesk](https://helpdesk.bitrix24.com/open/21422038/).
- **Подтверждено:** для звонков нужны открытые TCP 443, TCP 7881 и UDP 1024–65000 — [Technical requirements](https://helpdesk.bitrix24.com/open/21205364/).
- **Wayland/шаринг экрана (важно для Omarchy/Hyprland):**
  - **Подтверждено (Hyprland Wiki):** «Screensharing is done through PipeWire on Wayland»; нужны `pipewire`, `wireplumber`, `xdg-desktop-portal-hyprland`; ключевое ограничение: «If your screensharing application is running under XWayland (like Discord or Skype), it can only see other XWayland windows and cannot share an entire screen or a Wayland window». — [Hyprland Wiki: Screen sharing](https://wiki.hypr.land/Useful-Utilities/Screen-Sharing/).
  - **Инференс:** веб-версия Б24 в Chrome/Chromium под Wayland использует тот же механизм WebRTC+PipeWire/portal, что и другие сайты; специфичных тредов «Bitrix24 + Wayland не шарится экран» в открытых источниках не найдено. Общие известные проблемы шаринга в Chromium под Wayland (чёрные квадраты у NVIDIA, ломавшиеся обновления) — [bbs.archlinux.org #280800](https://bbs.archlinux.org/viewtopic.php?id=280800), [forum.manjaro.org](https://forum.manjaro.org/t/screensharing-stopped-working-after-stable-update-2023-04-11-with-chromium-wayland-pipewire/138687), [r/archlinux](https://www.reddit.com/r/archlinux/comments/11nhfih/screen_sharing_in_googlechrome_on_waylandgnome/); см. также Arch Wiki, раздел PipeWire → WebRTC screen sharing: https://wiki.archlinux.org/title/PipeWire (получен по ссылке из тредов, напрямую не выгружался).
  - **Инференс:** если официальный Linux-клиент работает как XWayland-приложение (см. 2.1), то демонстрация экрана **из самого клиента** под Hyprland, вероятно, упрётся в то же ограничение XWayland (или потребует Electron-порталов). Прямых подтверждений ни того, ни другого не найдено — **не подтверждено**, проверять на реальной машине.
- **PWA:** **подтверждено отсутствие официального PWA-предложения**: в списке способов доступа Битрикс24 перечислены браузерная версия, мобильные приложения (iOS/Android/Huawei) и десктоп-приложение — [bitrix24.com/apps/mobile-app.php](https://www.bitrix24.com/apps/mobile-app.php); упоминаний installable PWA в официальных источниках не найдено. Возможность «установить сайт как приложение Chrome» — известный workaround, но это не PWA от вендора (инференс).

---

## 4. Боль и запросы юзеров (Linux-специфика и рядом)

1. **Beta Linux-клиент (2021): не работал ввод с клавиатуры в чате** — «скачал bitrix24_desktop_6_0_45_40.tgz… В самом чате ввод с клавиатуры не работает» — [dev.1c-bitrix.ru, форум, topic139476](https://dev.1c-bitrix.ru/community/forums/forum23/topic139476/) (21.04.2021). Историческое подтверждение, что Linux-клиент существовал как beta ещё до официального релиза.
2. **Установка RPM-клиента ломается на отечественных дистрибутивах:** «Не устанавливается Битрикс24 Десктоп на АЛЬТ8 СП… из-за старой версии rpmlib» (14.11.2024) — [dev.1c-bitrix.ru, история постов пользователя](https://dev.1c-bitrix.ru/support/forum/user/8013658/post/all/) (цитата из листинга постов; прямая ссылка на топик не вытащена — отмечаю).
3. **Ссылки портала не открываются в десктоп-приложении:** «Есть ли возможность открывать страницы портала… напрямую в программе Битрикс?» — вопрос без внятного решения — [Хабр Q&A](https://qna.habr.com/q/1330230). В сочетании с документацией по `bx://` (только bot-контекст, п.1.4) это указывает на реальный пробел: нет универсального механизма «открыть любую ссылку Б24 в десктопе».
4. **Тормоза десктоп-приложения** — «Грузит диспетчер постоянно на 20–30 процентов» — [forum.mista.ru](https://forum.mista.ru/topic/870286) (2021); систематический разбор тормозов портала — [Хабр/Интерволга](https://habr.com/ru/companies/intervolga/articles/922654/). Не Linux-специфика, но боль именно десктоп-клиента.
5. **Проблемы звонков из десктопа** (ошибка создания звонка код 8 при работе по RDP; «не работают аудио-видео звонки» в коробке) — [topic150266](https://dev.1c-bitrix.ru/community/forums/forum23/topic150266/), [topic128618](https://dev.1c-bitrix.ru/community/forums/forum23/topic128618/). Официальный траблшутинг: обновить/переустановить приложение, проверить SSL в коробке — [helpdesk RU](https://helpdesk.bitrix24.ru/open/19941164), [helpdesk EN 20331442](https://helpdesk.bitrix24.com/open/20331442/).
6. **Баг звуковых уведомлений открытых линий** (не приходит звук/счётчик, помогает только F5) — [Хабр Q&A](https://qna.habr.com/q/1356778). Не Linux-специфика.
7. **Трей-иконки у Electron-приложений на Linux/Wayland — общая боль жанра:** показательный кейс соседнего продукта MyChat (тоже Electron): после обновления пропала иконка в трее, разработчик объяснил переход на AppIndicator/SNI («Именно его используют Electron-приложения, Chrome, Telegram… Работает и на X11, и на Wayland») — [nsoft-s.com, тред про MyChat 2026.3.0](https://nsoft-s.com/forum/viewtopic.php?t=8555). **Внимание:** этот тред — про MyChat, НЕ про Битрикс24 (поиск ошибочно связал его с Б24; я проверил первоисточник). Конкретных тредов «у Битрикс24 на Linux нет иконки в трее» не найдено — **не подтверждено**, что такая боль массовая.
8. **Reddit по «Bitrix24 + Wayland/Arch/NixOS»:** содержательных тредов не найдено — либо боль закрыта официальным Linux-клиентом, либо Linux-аудитория Б24 мала/молчит. Это само по себе важный сигнал спроса (слабый, интерпретация).

---

## 5. Приложения Маркета с «десктопными» функциями (телефония, клик-ту-колл)

- Телефонные интеграции — большой готовый класс приложений Маркета: подборки [«30 популярных телефоний для Битрикс24»](https://www.bitrix24.ru/apps/collection/15732444/) и [«Телефонии для Битрикс24»](https://www.bitrix24.ru/apps/collection/21378570/).
- Конкретные примеры: [Интеграция UIS с Битрикс24](https://www.bitrix24.ru/apps/app/novosystem.uiscomagic/) (16102 установок, оценка 5/5), [Телефония Сипуни](https://www.bitrix24.ru/apps/app/sipuni.scrm2/) (5311 установок); EN-маркет: [Voxloud Click to call](https://www.bitrix24.com/apps/app/Voxloud.voxloud_click_to_call/) — «initiate calls directly from Bitrix24… integrates with the external Voxloud softphone», [ZIWO Cloud Contact Center](https://www.bitrix24.com/apps/app/vortexweb.ziwo_application_ashwani_kt/) — «Click-to-call directly from Bitrix24 contacts, leads and deals».
- Требования к телефонным приложениям Маркета (обязательные сценарии для публикации) описаны официально — [apidocs: requirements-telephony](https://apidocs.bitrix24.com/market/preparing-to-publish/requirements-telephony.html).
- **Ключевой нюанс (см. 1.5):** всё это — integration-приложения, работающие в веб-версии; в десктоп-клиенте (любой ОС) их виджеты не работают. Клик-ту-колл из карточки CRM — да, но внутри веб-версии.

---

## Вывод: гэпы и свободные ниши

1. **Не нужно писать «ещё один клиент Б24»** — официальный Linux-клиент есть с 2022 г., функционально паритетный с Win/macOS, поддерживается саппортом. Гипотеза «Linux-версии нет» неверна.
2. **Реальные гэпы (потенциальные ниши для плагина/инструмента):**
   - **Arch-упаковка и её качество:** AUR-пакет существует, но это хрупкая перепаковка безверсионного DEB с ручным sha256; нет официального PKGBUILD/AppImage/Flatpak. Ниша: аккуратный PKGBUILD/Flatpak/AppImage с автотрекингом релизов.
   - **Wayland/Hyprland-опыт:** нативный Wayland-режим клиента не задокументирован (зависимости намекают на XWayland); поведение шаринга экрана из Linux-клиента под Hyprland не подтверждено никем. Ниша: проверить/починить (флаги ozone, порталы, PipeWire), задокументировать.
   - **Диплинки:** задокументирован только `bx://…/botContext/...`; универсального «открыть ссылку портала в приложении» нет (подтверждено доками + Habr Q&A). Ниша: хендлер ссылок/расширение браузера «открыть в Б24-клиенте».
   - **Уведомления вне клиента:** нет рецепта Bitrix24 ни в Ferdium, ни в Rambox; нотификатор на REST (im.notify / события) с нативными уведомлениями Linux — незанятая мелкая ниша.
   - **Компенсация «маркетплейс только в веб»:** любой инструмент, который выводит функции маркетплейс-приложений (например, телефонию) за пределы веб-версии, — упирается в официальное ограничение платформы; честнее считать это границей экосистемы, а не нишей.
3. **Ограничение любого плагина:** REST-приложения/виджеты живут в веб-версии; десктоп-клиент их не исполняет — архитектуру плагина надо строить вокруг REST/webhooks + собственного UI, а не вокруг встраивания в клиент.
4. **Спрос не доказан:** живых открытых запросов «сделайте лучше для Linux» немного; основные найденные боли — старые (2021–2024) или не Linux-специфичные. Перед разработкой стоит провести 1–2 живых интервью/экспериментов (см. Next steps).

---

## Next steps

- Поставить AUR `bitrix24` на Omarchy и практически проверить: нативный ли Wayland (`ELECTRON_OZONE_PLATFORM_HINT=wayland`), трей в waybar, шаринг экрана в звонке из клиента, работу Диска — это закроет все «не подтверждено» из разделов 2.1 и 3.
- Прогнать веб-версию в Chrome под Hyprland с `pipewire + wireplumber + xdg-desktop-portal-hyprland` и сравнить качество звонков/шаринга с клиентом.
- При решении делать плагин — начать с REST (webhooks, `im.notify`, виджеты), а не с обёртки интерфейса.

---

## Список источников

**Официальные (Bitrix24/1С-Битрикс):**
- Приложение Битрикс24 для Linux (helpdesk RU) — https://helpdesk.bitrix24.ru/open/16171806/
- Bitrix24 Messenger for Linux (helpdesk EN) — https://helpdesk.bitrix24.com/open/16172002/
- Блог: анонс Linux-приложения (20.07.2022) — https://www.bitrix24.ru/blogs/prilozhenie-bitriks24-dlya-linux.php
- Страницы загрузок — https://www.bitrix24.ru/features/downloads/ , https://www.bitrix24.ru/features/desktop.php
- Описание десктоп-приложения (EN) — https://www.bitrix24.com/apps/desktop.php
- Блог: новый all-in-one десктоп — https://www.bitrix24.com/amp/blogs/applications/new-all-in-one-bitrix24-desktop-app.php
- Установка Messenger (Win/mac), системные требования — https://helpdesk.bitrix24.com/open/20819676/
- Логи в саппорт (macOS and Linux) — https://helpdesk.bitrix24.com/open/20535138/
- Настройки Messenger — https://helpdesk.bitrix24.com/open/25582355/
- Уведомления — https://helpdesk.bitrix24.com/open/25839311/ и https://helpdesk.bitrix24.ru/open/26929068/
- Диплинк bx:// (bot context) — https://apidocs.bitrix24.com/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html (RU: https://apidocs.bitrix24.ru/api-reference/chat-bots/chat-bots-v2/imbot.v2/bot-context.html)
- Интеграция-приложения: только веб-версия — https://helpdesk.bitrix24.com/open/17056078/
- Техтребования видеозвонков (Chrome 80+, порты) — https://helpdesk.bitrix24.com/open/21205364/
- Fix video conferencing issues — https://helpdesk.bitrix24.com/open/11417394/
- Troubleshoot Bitrix24 Messenger — https://helpdesk.bitrix24.com/open/20331442/
- Если не работает приложение Битрикс24 (RU) — https://helpdesk.bitrix24.ru/open/19941164
- Синхронизация Диска через приложение — https://helpdesk.bitrix24.ru/open/19941088/
- Требования к телефонным приложениям Маркета — https://apidocs.bitrix24.com/market/preparing-to-publish/requirements-telephony.html
- Каналы доступа (веб/мобайл/десктоп; для пункта о PWA) — https://www.bitrix24.com/apps/mobile-app.php

**Маркетплейс (телефония/клик-ту-колл):**
- Коллекции телефоний — https://www.bitrix24.ru/apps/collection/15732444/ , https://www.bitrix24.ru/apps/collection/21378570/
- UIS — https://www.bitrix24.ru/apps/app/novosystem.uiscomagic/
- Сипуни — https://www.bitrix24.ru/apps/app/sipuni.scrm2/
- Voxloud Click to call — https://www.bitrix24.com/apps/app/Voxloud.voxloud_click_to_call/
- ZIWO Cloud Contact Center — https://www.bitrix24.com/apps/app/vortexweb.ziwo_application_ashwani_kt/

**Упаковки и опенсорс:**
- AUR bitrix24 — https://aur.archlinux.org/packages/bitrix24
- PKGBUILD AUR bitrix24 — https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=bitrix24
- Repology: bitrix24 — https://repology.org/project/bitrix24/packages
- infinitrix (PyQt5-клиент) — https://github.com/smallcms/infinitrix
- bitrix24-electron (лаунчер) — https://github.com/MaxNeutrino/bitrix24-electron
- Bitrix24_TimeTrack — https://github.com/Maroosek/Bitrix24_TimeTrack
- ansible-role-bitrix24 — https://github.com/webarchitect609/ansible-role-bitrix24
- Snap (неофиц., заброшен) — https://snapcraft.io/bitrix24 ; https://snapcraft.io/bitrix
- Flathub (отсутствие результата) — https://flathub.org/en
- Ferdium recipes (нет рецепта bitrix24) — https://github.com/ferdium/ferdium-recipes ; custom services: https://ferdium.app/features.html , https://ferdium.org/faq
- Rambox CE services.json (нет Bitrix24) — https://raw.githubusercontent.com/ramboxapp/community-edition/master/resources/services.json ; custom apps: https://support.rambox.app/getting-started/how-to-create-and-configure-apps.html
- Гайд установки на Arch (сторонний) — https://tutorialforlinux.com/2025/05/22/how-to-install-bitrix24-on-arch-linux-step-by-step/

**Wayland/шаринг экрана (контекст):**
- Hyprland Wiki: Screen sharing — https://wiki.hypr.land/Useful-Utilities/Screen-Sharing/
- Arch Wiki: PipeWire (WebRTC screen sharing) — https://wiki.archlinux.org/title/PipeWire (по ссылке из тредов; напрямую не выгружался)
- bbs.archlinux.org (Chrome/Electron black boxes on Wayland) — https://bbs.archlinux.org/viewtopic.php?id=280800
- forum.manjaro.org (Chromium+Wayland+PipeWire) — https://forum.manjaro.org/t/screensharing-stopped-working-after-stable-update-2023-04-11-with-chromium-wayland-pipewire/138687
- r/archlinux (Chrome Wayland sharing) — https://www.reddit.com/r/archlinux/comments/11nhfih/screen_sharing_in_googlechrome_on_waylandgnome/

**Боли/форумы:**
- Beta Linux: клавиатура в чате (2021) — https://dev.1c-bitrix.ru/community/forums/forum23/topic139476/
- Альт 8СП: не устанавливается десктоп (rpmlib) — https://dev.1c-bitrix.ru/support/forum/user/8013658/post/all/
- Хабр Q&A: ссылки в десктопе — https://qna.habr.com/q/1330230
- Хабр Q&A: баг уведомлений открытых линий — https://qna.habr.com/q/1356778
- forum.mista.ru: тормоза приложения — https://forum.mista.ru/topic/870286
- Хабр (Интерволга): почему Б24 тормозит — https://habr.com/ru/companies/intervolga/articles/922654/
- Форумы dev.1c-bitrix: звонки по RDP — https://dev.1c-bitrix.ru/community/forums/forum23/topic150266/ ; звонки в коробке — https://dev.1c-bitrix.ru/community/forums/forum23/topic128618/
- Кейс Electron-трея (MyChat, не Б24 — фоновый контекст) — https://nsoft-s.com/forum/viewtopic.php?t=8555

**Отбраковано/деприоритезировано:**
- https://nsoft-s.com/forum/viewtopic.php?t=8555 — по проверке оказался про MyChat Client, а не Bitrix24 (поисковик ошибочно связал с Б24); использован только как общий контекст про Electron-трей.
- https://github.com/Bitrix24-Desktop-App/Bitrix24-Mac — SEO-репозиторий-витрина, не реальный клиент.
- https://wiki.astralinux.ru/... (PDF-экспорт wiki) — слабый вторичный источник, дублирует факт наличия Linux-клиента.
- https://topbusinesssoftware.com/... , https://www.hongkiat.com/... — SEO-обзоры, низкая достоверность.

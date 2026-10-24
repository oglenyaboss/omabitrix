package demo

import "time"

type demoUser struct {
	ID     int64
	Name   string
	Online bool
}

type demoStage struct {
	ID    int64
	Title string
	Sort  int
	Color string
}

type demoGroup struct {
	ID     int64
	Name   string
	Stages []demoStage
}

type demoFile struct {
	ID   int64
	Type string
	Name string
	Ext  string
	Data []byte
}

type demoMessage struct {
	ID       int64
	AuthorID int64
	Date     time.Time
	Text     string
	Deleted  bool
	Likes    []int64
	FileIDs  []int64
	ReplyID  int64
}

type demoDialog struct {
	ID       string
	ChatID   int64
	Title    string
	Online   bool
	Counter  int
	Group    bool
	Messages []*demoMessage
}

type demoTask struct {
	ID          int64
	Title       string
	Role        string
	GroupID     int64
	StageID     int64
	Deadline    time.Time
	HasDeadline bool
	NewComments int
	ChatID      int64
	CreatorID   int64
	Status      int
	Description string
}

type demoNotify struct {
	ID       int64
	Module   string
	Title    string
	Tag      string
	Text     string
	Date     time.Time
	Read     bool
	AuthorID int64
}

func (s *server) ago(minutes int) time.Time {
	return s.started.Add(-time.Duration(minutes) * time.Minute)
}

func (s *server) dayAt(offset, hour, minute int) time.Time {
	t := s.started.In(s.loc)
	return time.Date(t.Year(), t.Month(), t.Day()+offset, hour, minute, 0, 0, s.loc)
}

func (s *server) buildData() {
	const (
		me     = 1
		anna   = 2
		dmitry = 3
		maria  = 4
		igor   = 5
		olga   = 6
	)
	s.users = []demoUser{
		{ID: me, Name: "Алексей Ковалёв", Online: true},
		{ID: anna, Name: "Анна Смирнова", Online: true},
		{ID: dmitry, Name: "Дмитрий Козлов"},
		{ID: maria, Name: "Мария Иванова"},
		{ID: igor, Name: "Игорь Соколов", Online: true},
		{ID: olga, Name: "Ольга Новикова"},
	}

	s.groups[10] = &demoGroup{ID: 10, Name: "Мобильное приложение", Stages: []demoStage{
		{ID: 101, Title: "Бэклог", Sort: 100, Color: "a8adb4"},
		{ID: 102, Title: "В работе", Sort: 200, Color: "2fc6f6"},
		{ID: 103, Title: "Ревью", Sort: 300, Color: "ffa900"},
		{ID: 104, Title: "Готово", Sort: 400, Color: "7bd500"},
	}}
	s.groups[11] = &demoGroup{ID: 11, Name: "Сайт", Stages: []demoStage{
		{ID: 111, Title: "Новые", Sort: 100, Color: "a8adb4"},
		{ID: 112, Title: "В работе", Sort: 200, Color: "2fc6f6"},
		{ID: 113, Title: "Готово", Sort: 300, Color: "7bd500"},
	}}

	onboarding := s.addFile("image", "onboarding-v3.png", "png", png("onboarding"))
	store := s.addFile("image", "store-screens.png", "png", png("store"))
	crash := s.addFile("file", "crash-log.txt", "txt", []byte(crashLog))

	annaChat := s.newChat("2", 0, "Анна Смирнова", true, 2, false)
	s.addMessages(annaChat,
		msg(me, 95, "Как там макеты онбординга?"),
		msg(anna, 90, "Почти, докручиваю цвета"),
		msg(me, 85, "Анимацию переходов беру на себя"),
		msg(anna, 80, "Супер. Lottie или руками?"),
		msg(me, 75, "Руками, в Compose это пара строк"),
		msg(anna, 70, "Скинула третью версию 👇"),
		msg(anna, 65, "Цвета из брендбука, иконки уже под тёмную тему", onboarding.ID),
		msg(anna, 60, "Тогда жду билд к четвергу 🙌"),
	)

	product := s.newChat("chat101", 101, "Команда продукта", false, 5, true)
	s.addMessages(product,
		msg(maria, 160, "Релиз 2.4 в четверг, всё по плану?"),
		msg(dmitry, 150, "Регресс на iOS прошёл, два минорных бага завёл"),
		msg(igor, 140, "API бонусов задеплоил на стейдж"),
		msg(olga, 130, "Скриншоты для сторов готовы", store.ID),
		msg(me, 120, "Пуши на стейдже, проверяю"),
		msg(maria, 110, "Тогда фиксируем код-фриз в среду"),
		msg(dmitry, 100, "Мерджу релизную ветку"),
		msg(olga, 90, "Добавила тексты в сторах"),
		msg(igor, 80, "Краш на Android 12 в релиз не идёт"),
		msg(maria, 70, "[USER=1]Алексей Ковалёв[/USER] глянешь пуши до обеда?"),
	)

	dmitryChat := s.newChat("3", 0, "Дмитрий Козлов", false, 0, false)
	s.addMessages(dmitryChat,
		msg(dmitry, 180, "Привет, глянь ветку с рефакторингом"),
		msg(me, 170, "Смотрю, тесты зелёные"),
		msg(dmitry, 160, "Я ещё поправил конфиг CI"),
		msg(me, 150, "Там флаки на integration, но не блокер"),
		msg(dmitry, 140, "Ок, тогда мерджим?"),
		msg(me, 130, "Ок, мерджу"),
	)

	igorChat := s.newChat("5", 0, "Игорь Соколов", true, 1, false)
	s.addMessages(igorChat,
		msg(igor, 200, "Привет, есть минута?"),
		msg(me, 190, "Да, что там?"),
		msg(igor, 180, "Воспроизвёл падение на пикселе"),
		msg(igor, 170, "Нашёл краш на Android 12, лог во вложении", crash.ID),
	)

	marketing := s.newChat("chat102", 102, "Маркетинг × Продукт", false, 0, true)
	s.addMessages(marketing,
		msg(olga, 260, "Кампания к запуску стартует в понедельник"),
		msg(maria, 240, "Баннеры готовы, ссылки проверены"),
		msg(me, 220, "На лендинге поправлю заголовок"),
		msg(olga, 200, "Спасибо! Жду финальные тексты"),
		msg(maria, 180, "Всё в карточке задачи 207"),
	)

	mariaChat := s.newChat("4", 0, "Мария Иванова", false, 0, false)
	s.addMessages(mariaChat,
		msg(maria, 320, "Привет! Скинула тексты пушей"),
		msg(me, 300, "Спасибо, посмотрю"),
		msg(maria, 280, "Если что, правки в карточке"),
		msg(me, 260, "Отлично, беру"),
	)

	olgaChat := s.newChat("6", 0, "Ольга Новикова", false, 0, false)
	s.addMessages(olgaChat,
		msg(olga, 360, "Скриншоты для сторов обновлены"),
		msg(me, 340, "Супер, спасибо"),
		msg(olga, 320, "Если нужны другие размеры — скажи"),
	)

	s.chats = []*demoDialog{annaChat, product, dmitryChat, igorChat, marketing, mariaChat, olgaChat}

	type taskSpec struct {
		id, group, stage, chat   int64
		title, role, description string
		creator                  int64
		day, hour, minute        int
		hasDeadline              bool
		newComments              int
	}
	specs := []taskSpec{
		{201, 10, 102, 3201, "Пуш-уведомления о начислении бонусов", "do",
			"[B]Что сделать:[/B] добавить пуши о начислении бонусов в iOS и Android.\n\n" +
				"Согласовать тексты с маркетингом и проверить события /bonus/events на стейдже. " +
				"После ревью выкатить за фича-флагом.", maria, 0, 18, 0, true, 2},
		{202, 10, 103, 3202, "Экран онбординга: анимации и иллюстрации", "do",
			"Собрать третий вариант онбординга с анимациями переходов. " +
				"Иллюстрации и палитра — из брендбука, проверить контраст в тёмной теме. " +
				"Готовый билд показать на демо в четверг.", maria, 1, 12, 0, true, 0},
		{203, 10, 102, 3203, "Краш на Android 12 при входе через VK ID", "do",
			"Пользователи на Android 12 падают при входе через VK ID. " +
				"Нужно воспроизвести, собрать лог и починить гонку в SDK. " +
				"Проверить сценарий на Pixel 4a и эмуляторе.", igor, -1, 19, 0, true, 1},
		{204, 10, 101, 3204, "Ускорить холодный старт до 1.5 с", "do",
			"Холодный старт сейчас около 2.8 с на среднем устройстве. " +
				"Профилировать инициализацию и вынести тяжёлые вещи с главного потока. " +
				"Цель — уверенные 1.5 с без регресса по памяти.", dmitry, 5, 18, 0, true, 0},
		{205, 10, 101, 3205, "Тёмная тема в настройках", "do",
			"Добавить переключатель тёмной темы и системное следование. " +
				"Проверить все экраны настроек и контраст иконок. " +
				"Согласовать палитру с дизайном.", anna, 0, 0, 0, false, 0},
		{206, 10, 104, 3206, "Аналитика воронки регистрации", "do",
			"Собрать события воронки от установки до первого заказа. " +
				"Сверить названия с дашбордом и добавить недостающие шаги. " +
				"Отдать результаты маркетингу.", olga, 2, 15, 0, true, 0},
		{207, 11, 112, 3207, "Лендинг программы лояльности", "do",
			"Собрать лендинг программы лояльности на основе макетов. " +
				"Подключить форму и аналитику, проверить мобильную вёрстку. " +
				"Согласовать тексты с маркетингом.", olga, 2, 18, 0, true, 0},
		{208, 11, 111, 3208, "SEO-мета для каталога", "help",
			"Проверить title и description у карточек каталога. " +
				"Убрать дубли и добавить канонические ссылки. " +
				"После правок прогнать аудит.", olga, 7, 18, 0, true, 0},
		{209, 10, 103, 3209, "Релиз 2.4 в App Store и Google Play", "watch",
			"Подготовить релизные сборки и описания для сторов. " +
				"Проверить скриншоты и тексты, выставить поэтапную раскатку. " +
				"Следить за отзывами первые сутки.", maria, 3, 11, 0, true, 0},
		{210, 0, 0, 3210, "Обновить политику конфиденциальности", "do",
			"Обновить текст политики под новые требования и опубликовать на сайте. " +
				"Сверить формулировки с юристами. " +
				"После публикации уведомить поддержку.", maria, 0, 13, 0, true, 0},
	}
	for _, sp := range specs {
		t := &demoTask{
			ID: sp.id, Title: sp.title, Role: sp.role, GroupID: sp.group,
			StageID: sp.stage, ChatID: sp.chat, CreatorID: sp.creator,
			Status: 2, NewComments: sp.newComments, Description: sp.description,
		}
		if sp.hasDeadline {
			t.Deadline = s.dayAt(sp.day, sp.hour, sp.minute)
			t.HasDeadline = true
		}
		s.tasks[t.ID] = t
		s.taskIDs = append(s.taskIDs, t.ID)
	}

	s.taskChat(3201, "Пуш-уведомления о начислении бонусов", []msgSpec{
		msg(maria, 330, "Тексты пушей согласованы, лежат в карточке"),
		msg(dmitry, 320, "Эндпоинт /bonus/events готов на стейдже"),
		msg(maria, 310, "[USER=1]Алексей Ковалёв[/USER] успеем сегодня?"),
	})
	s.taskChat(3202, "Экран онбординга: анимации и иллюстрации", []msgSpec{
		msg(maria, 350, "Макет онбординга обновлён — посмотри в задаче"),
	})
	s.taskChat(3203, "Краш на Android 12 при входе через VK ID", []msgSpec{
		msg(igor, 340, "Воспроизводится на Pixel 4a, лог приложил"),
		msg(igor, 330, "Похоже на гонку в SDK VK ID"),
	})
	s.taskChat(3204, "Ускорить холодный старт до 1.5 с", []msgSpec{
		msg(dmitry, 360, "Снял профиль старта, узкое место в инициализации"),
	})
	s.taskChat(3205, "Тёмная тема в настройках", []msgSpec{
		msg(anna, 370, "Палитра тёмной темы готова, лежит в задаче"),
	})
	s.taskChat(3206, "Аналитика воронки регистрации", []msgSpec{
		msg(olga, 380, "События воронки собраны, жду сверку"),
	})
	s.taskChat(3207, "Лендинг программы лояльности", []msgSpec{
		msg(olga, 390, "Макеты лендинга в карточке задачи"),
	})
	s.taskChat(3208, "SEO-мета для каталога", []msgSpec{
		msg(olga, 400, "Список страниц для аудита приложила"),
	})
	s.taskChat(3209, "Релиз 2.4 в App Store и Google Play", []msgSpec{
		msg(maria, 410, "Сборки готовы, описания ждут проверки"),
	})
	s.taskChat(3210, "Обновить политику конфиденциальности", []msgSpec{
		msg(maria, 420, "Юристы прислали правки, посмотри сегодня"),
	})

	s.notifies = []*demoNotify{
		{ID: 808, Module: "im", Title: "Команда продукта",
			Text: "[USER=1]Алексей Ковалёв[/USER] глянешь пуши до обеда?",
			Date: s.ago(5), AuthorID: maria},
		{ID: 807, Module: "tasks",
			Text: "Мария Иванова назначила вас ответственным: [#201] Пуш-уведомления о начислении бонусов",
			Tag:  "TASKS|TASK|201|4", Date: s.ago(10), AuthorID: maria},
		{ID: 806, Module: "tasks",
			Text: "Игорь Соколов добавил комментарий в задаче [#203]: «Похоже на гонку в SDK VK ID»",
			Tag:  "TASKS|TASK|203|5", Date: s.ago(15), AuthorID: igor},
		{ID: 805, Module: "tasks",
			Text: "Напоминание: дедлайн задачи [#210] Обновить политику конфиденциальности сегодня в 13:00",
			Tag:  "TASKS|TASK|210|4", Date: s.ago(20), Read: true, AuthorID: maria},
		{ID: 804, Module: "tasks",
			Text: "Анна Смирнова добавила вас наблюдателем: [#209] Релиз 2.4 в App Store и Google Play",
			Tag:  "TASKS|TASK|209|2", Date: s.ago(30), Read: true, AuthorID: anna},
		{ID: 803, Module: "calendar", Title: "Демо спринта, 16:00",
			Text: "Напоминание о встрече: «Демо спринта, 16:00»",
			Date: s.ago(40), Read: true, AuthorID: me},
		{ID: 802, Module: "tasks",
			Text: "Мария Иванова добавила комментарий в задаче [#202]: «Анимации готовы к ревью»",
			Tag:  "TASKS|TASK|202|4", Date: s.ago(50), Read: true, AuthorID: maria},
		{ID: 801, Module: "tasks",
			Text: "Задача [#206] Аналитика воронки регистрации перенесена в Готово",
			Tag:  "TASKS|TASK|206|6", Date: s.ago(60), Read: true, AuthorID: olga},
	}
}

func (s *server) addFile(kind, name, ext string, data []byte) *demoFile {
	s.nextFileID++
	f := &demoFile{ID: s.nextFileID, Type: kind, Name: name, Ext: ext, Data: data}
	s.files[f.ID] = f
	return f
}

func (s *server) newChat(id string, chatID int64, title string, online bool, counter int, group bool) *demoDialog {
	d := &demoDialog{ID: id, ChatID: chatID, Title: title, Online: online, Counter: counter, Group: group}
	s.dialogs[id] = d
	return d
}

type msgSpec struct {
	author  int64
	minutes int
	text    string
	files   []int64
}

func msg(author, minutes int, text string, fileIDs ...int64) msgSpec {
	return msgSpec{author: int64(author), minutes: minutes, text: text, files: fileIDs}
}

func (s *server) addMessages(d *demoDialog, specs ...msgSpec) {
	for _, sp := range specs {
		s.appendMessage(d, sp.author, s.ago(sp.minutes), sp.text, sp.files...)
	}
}

func (s *server) taskChat(chatID int64, title string, specs []msgSpec) {
	d := s.newChat("chat"+itoa(chatID), chatID, "SRV. "+title, false, 0, true)
	s.addMessages(d, specs...)
}

func itoa(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{digits[n%10]}, b...)
		n /= 10
	}
	return string(b)
}

const crashLog = `FATAL EXCEPTION: main
Process: com.omabitrix.mobile, PID: 18432
java.lang.RuntimeException: Unable to start activity ComponentInfo{com.omabitrix.mobile/com.omabitrix.mobile.MainActivity}: java.lang.NullPointerException
	at android.app.ActivityThread.performLaunchActivity(ActivityThread.java:3782)
	at android.app.ActivityThread.handleLaunchActivity(ActivityThread.java:3945)
	at android.app.servertransaction.LaunchActivityItem.execute(LaunchActivityItem.java:112)
	at android.app.servertransaction.TransactionExecutor.executeCallbacks(TransactionExecutor.java:149)
	at android.os.Handler.dispatchMessage(Handler.java:106)
	at android.os.Looper.loopOnce(Looper.java:226)
	at android.os.Looper.loop(Looper.java:313)
	at android.app.ActivityThread.main(ActivityThread.java:8687)
	at java.lang.reflect.Method.invoke(Native Method)
	at com.android.internal.os.RuntimeInit$MethodAndArgsCaller.run(RuntimeInit.java:604)
	at com.android.internal.os.ZygoteInit.main(ZygoteInit.java:1071)
Caused by: java.lang.NullPointerException: Attempt to invoke virtual method 'boolean com.vk.sdk.VKAccessToken.isExpired()' on a null object reference
	at com.omabitrix.mobile.auth.VkLoginHandler.onResult(VkLoginHandler.kt:48)
	at com.omabitrix.mobile.auth.LoginCoordinator.restoreSession(LoginCoordinator.kt:91)
	at com.omabitrix.mobile.MainActivity.onCreate(MainActivity.kt:37)
	at android.app.Activity.performCreate(Activity.java:8591)
	at android.app.Activity.performCreate(Activity.java:8570)
	... 11 more
`

import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Commons as Commons
import qs.Ui

// Панель Bitrix24: три вкладки (чаты / уведомления / задачи) + оверлеи
// (чат, карточка задачи). Все данные либо от демона (счётчики, задачи),
// либо своим ходом по вебхуку; списки и истории кэшируются.
Panel {
  id: root
  moduleName: "lenya.omabitrix"
  manageIpc: false

  property var anchorItem: null
  property var hostWidget: null
  readonly property var barIdentity: hostWidget || root

  // общее для страниц
  readonly property color foreground: bar ? bar.foreground : Commons.Color.foreground
  readonly property color dim: Qt.darker(foreground, 1.55)
  readonly property color urgentColor: bar ? bar.urgent : Commons.Color.urgent
  readonly property color accent: Commons.Color.accent
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family
  readonly property real fontTitle: Style.font.title
  readonly property real fontBody: Style.font.body
  readonly property real fontCaption: Style.font.caption
  readonly property int spacing: Style.spacing.sm
  readonly property int listCap: Style.space(440)
  readonly property int historyCap: Style.space(340)
  readonly property int descCap: Style.space(220)
  readonly property string portal: setting("portal", "")

  // ---- состояние навигации
  property int tab: 0 // 0 чаты, 1 уведомления, 2 задачи
  property var selectedChat: null
  property var selectedTask: null

  // ---- данные демона
  property var snap: null
  readonly property var overdueTasks: snap && snap.overdue_tasks ? snap.overdue_tasks : []
  readonly property var todayTasks: snap && snap.today_tasks ? snap.today_tasks : []

  // ---- чаты (свой ход, кэш)
  property var chats: []
  property double chatsLoadedAt: 0
  property string chatsError: ""
  property bool loadingChats: false
  property string filterText: ""

  // ---- история чата
  property var messages: []
  property bool loadingHistory: false
  property string historyError: ""
  property bool sending: false
  property string sendError: ""
  property var messagesCache: ({}) // dialog_id -> {rows, at, noMore}

  // ---- уведомления
  property var notifications: []
  property double notifyLoadedAt: 0
  property bool loadingNotify: false
  property string notifyError: ""

  // ---- карточка задачи
  property bool taskUpdating: false
  property string taskUpdateError: ""

  // ссылки на поля ввода страниц (фокус)
  property var searchFieldRef: null
  property var replyFieldRef: null

  readonly property var filteredChats: {
    var f = String(filterText || "").toLowerCase()
    var out = []
    for (var i = 0; i < chats.length; i++) {
      var c = chats[i]
      if (f === "" || String(c.title).toLowerCase().indexOf(f) >= 0
          || String(c.preview).toLowerCase().indexOf(f) >= 0) out.push(c)
    }
    return out
  }

  readonly property var regularMessages: {
    var out = []
    for (var i = 0; i < messages.length; i++) if (!messages[i].system) out.push(messages[i])
    return out
  }

  readonly property var systemMessages: {
    var out = []
    for (var i = 0; i < messages.length; i++) if (messages[i].system) out.push(messages[i])
    return out
  }

  property string hookBase: ""

  function refreshFromHost() {
    snap = hostWidget && hostWidget.snap ? hostWidget.snap : null
    if (opened && Date.now() - chatsLoadedAt > 15000) loadChats(false)
    if (opened && tab === 1 && Date.now() - notifyLoadedAt > 20000) loadNotifications(false)
  }

  onOpenedChanged: {
    if (!opened) return
    snap = hostWidget && hostWidget.snap ? hostWidget.snap : null
    if (!hookBase) loadConfig()
    if (chats.length === 0) loadChats(true)
    else if (Date.now() - chatsLoadedAt > 20000) loadChats(false)
    if (tab === 1 && notifications.length === 0) loadNotifications(true)
  }

  onTabChanged: {
    if (!opened) return
    if (tab === 1 && Date.now() - notifyLoadedAt > 20000) loadNotifications(false)
  }

  function loadConfig() {
    if (!configProc.running) configProc.running = true
  }

  Process {
    id: configProc
    running: false
    command: ["cat", Quickshell.env("HOME") + "/.config/omabitrix/config.toml"]
    stdout: StdioCollector {
      onStreamFinished: {
        var portal = /portal\s*=\s*"([^"]+)"/.exec(text)
        var user = /webhook_user\s*=\s*(\d+)/.exec(text)
        var secret = /webhook_secret\s*=\s*"([^"]+)"/.exec(text)
        if (portal && user && secret) {
          root.hookBase = portal[1].replace(/\/+$/, "") + "/rest/" + user[1] + "/" + secret[1] + "/"
          if (root.opened && root.chats.length === 0) root.loadChats(true)
          if (root.opened && root.tab === 1 && root.notifications.length === 0) root.loadNotifications(true)
        }
      }
    }
  }

  Component.onCompleted: loadConfig()

  function api(method, params, onOk, onErr) {
    if (!hookBase) {
      onErr("вебхук не настроен")
      return
    }
    var xhr = new XMLHttpRequest()
    xhr.open("POST", hookBase + method + ".json")
    xhr.setRequestHeader("Content-Type", "application/json")
    xhr.onreadystatechange = function() {
      if (xhr.readyState !== XMLHttpRequest.DONE) return
      try {
        var r = JSON.parse(xhr.responseText)
        if (r.error !== undefined && r.error !== false) onErr(r.error + (r.error_description ? ": " + r.error_description : ""))
        else onOk(r.result)
      } catch (e) {
        onErr("портал недоступен")
      }
    }
    xhr.send(JSON.stringify(params))
  }

  function stripBB(text) {
    return String(text || "").replace(/\[[^\]]*\]/g, "").trim()
  }

  function fmtTime(iso) {
    var d = new Date(iso)
    if (isNaN(d.getTime())) return ""
    var now = new Date()
    var hh = ("0" + d.getHours()).slice(-2), mm = ("0" + d.getMinutes()).slice(-2)
    if (d.toDateString() === now.toDateString()) return hh + ":" + mm
    var dd = ("0" + d.getDate()).slice(-2), mo = ("0" + (d.getMonth() + 1)).slice(-2)
    return dd + "." + mo
  }

  function dayInfo(iso) {
    var d = new Date(iso)
    if (isNaN(d.getTime())) return { key: "", label: "" }
    var now = new Date()
    var yest = new Date(now.getTime() - 86400000)
    var label
    if (d.toDateString() === now.toDateString()) label = "сегодня"
    else if (d.toDateString() === yest.toDateString()) label = "вчера"
    else label = ("0" + d.getDate()).slice(-2) + "." + ("0" + (d.getMonth() + 1)).slice(-2)
    return { key: d.toDateString(), label: label }
  }

  function avatarOf(it) {
    if (it.user && it.user.avatar) return it.user.avatar
    if (it.avatar) {
      if (typeof it.avatar === "string") return it.avatar
      if (it.avatar.url) return it.avatar.url
    }
    if (it.chat && typeof it.chat.avatar === "string") return it.chat.avatar
    return ""
  }

  function colorOf(it) {
    if (it.avatar && typeof it.avatar === "object" && it.avatar.color) return it.avatar.color
    if (it.chat && it.chat.color) return it.chat.color
    return "#7a8a8a"
  }

  function loadChats(showSpinner) {
    if (loadingChats) return
    loadingChats = showSpinner === true
    chatsError = ""
    api("im.recent.list", { LIMIT: 40 },
      function(result) {
        loadingChats = false
        chatsLoadedAt = Date.now()
        var raw = result && result.items ? result.items : []
        var out = []
        for (var i = 0; i < raw.length; i++) {
          var it = raw[i]
          out.push({
            dialog_id: String(it.id),
            title: it.title || String(it.id),
            counter: it.counter || 0,
            preview: it.message ? stripBB(it.message.text) : "",
            time: it.message && it.message.date ? fmtTime(it.message.date) : "",
            stamp: it.message && it.message.date ? new Date(it.message.date).getTime() : 0,
            avatar: avatarOf(it),
            color: colorOf(it),
            online: it.user && it.user.status === "online"
          })
        }
        out.sort(function(a, b) {
          if ((b.counter > 0) !== (a.counter > 0)) return b.counter > 0 ? 1 : -1
          return b.stamp - a.stamp
        })
        chats = out
      },
      function(err) {
        loadingChats = false
        chatsError = err
      })
  }

  function putCache(dialogId, rows, noMore) {
    var copy = {}
    for (var k in messagesCache) copy[k] = messagesCache[k]
    copy[dialogId] = { rows: rows, at: Date.now(), noMore: noMore === true }
    messagesCache = copy
  }

  function cacheOf(dialogId) {
    return messagesCache[dialogId] || null
  }

  function rowsFromResult(result) {
    var users = {}
    var list = result && result.users ? result.users : []
    for (var u = 0; u < list.length; u++) users[list[u].id] = list[u].name
    var raw = result && result.messages ? result.messages : []
    var rows = []
    for (var i = 0; i < raw.length; i++) {
      var m = raw[i]
      if (m.params && m.params.IS_DELETED === "Y") continue
      var day = dayInfo(m.date)
      rows.push({
        id: m.id,
        name: m.author_id === 0 ? "" : (users[m.author_id] || ""),
        text: stripBB(m.text),
        time: fmtTime(m.date),
        dayKey: day.key,
        dayLabel: day.label,
        mine: m.author_id === 75,
        system: m.author_id === 0
      })
    }
    rows.reverse()
    return rows
  }

  function loadHistory(force) {
    if (!selectedChat) return
    var cached = cacheOf(selectedChat.dialog_id)
    if (cached) {
      messages = cached.rows
      if (!force && Date.now() - cached.at < 15000) {
        scrollChatDown()
        return
      }
    }
    loadingHistory = true
    historyError = ""
    api("im.dialog.messages.get", { DIALOG_ID: selectedChat.dialog_id, LIMIT: 30 },
      function(result) {
        loadingHistory = false
        var rows = rowsFromResult(result)
        messages = rows
        putCache(selectedChat.dialog_id, rows, rows.length === 0)
        scrollChatDown()
      },
      function(err) {
        loadingHistory = false
        historyError = err
      })
  }

  function loadOlder(dialogId, done) {
    var cached = cacheOf(dialogId)
    var firstId = cached && cached.rows.length > 0 ? cached.rows[0].id : 0
    if (!firstId) { done(false, null); return }
    api("im.dialog.messages.get", { DIALOG_ID: dialogId, LIMIT: 30, LAST_ID: firstId },
      function(result) {
        var older = rowsFromResult(result)
        var rows = older.concat(cached ? cached.rows : [])
        messages = rows
        putCache(dialogId, rows, older.length === 0)
        done(true, older)
      },
      function(err) {
        historyError = err
        done(false, null)
      })
  }

  function scrollChatDown() {
    Qt.callLater(function() {
      if (overlayLoader.item && overlayLoader.item.scrollDown) overlayLoader.item.scrollDown()
    })
  }

  function sendReply(chat, text, done) {
    if (sending) { done(false); return }
    sending = true
    sendError = ""
    api("im.message.add", { DIALOG_ID: chat.dialog_id, MESSAGE: text },
      function(messageId) {
        api("im.dialog.read", { DIALOG_ID: chat.dialog_id, MESSAGE_ID: messageId },
          function() {}, function() {})
        sending = false
        var day = dayInfo(new Date().toISOString())
        var row = { id: messageId, name: "", text: text,
          time: ("0" + new Date().getHours()).slice(-2) + ":" + ("0" + new Date().getMinutes()).slice(-2),
          dayKey: day.key, dayLabel: day.label, mine: true, system: false }
        messages = messages.concat([row])
        putCache(chat.dialog_id, messages)
        for (var c = 0; c < chats.length; c++) {
          if (chats[c].dialog_id === chat.dialog_id) {
            var nc = chats.slice()
            nc[c] = { dialog_id: chats[c].dialog_id, title: chats[c].title, counter: 0,
              preview: text, time: row.time, stamp: Date.now(),
              avatar: chats[c].avatar, color: chats[c].color, online: chats[c].online }
            chats = nc
            break
          }
        }
        if (hostWidget && hostWidget.markRead) hostWidget.markRead(chat.dialog_id)
        done(true)
      },
      function(err) {
        sending = false
        sendError = err
        done(false)
      })
  }

  // ---- уведомления
  function loadNotifications(showSpinner) {
    if (loadingNotify) return
    loadingNotify = showSpinner === true
    notifyError = ""
    api("im.notify.get", { LIMIT: 30 },
      function(result) {
        loadingNotify = false
        notifyLoadedAt = Date.now()
        var raw = result && result.notifications ? result.notifications : []
        var out = []
        for (var i = 0; i < raw.length; i++) {
          var n = raw[i]
          var title = stripBB(n.notify_title || "")
          out.push({
            id: n.id,
            module: n.notify_module || "",
            title: title,
            text: stripBB(n.text || ""),
            time: fmtTime(n.date),
            unread: n.notify_read === "N",
            stamp: new Date(n.date).getTime() || 0,
            url: ""
          })
        }
        out.sort(function(a, b) { return b.stamp - a.stamp })
        notifications = out
      },
      function(err) {
        loadingNotify = false
        notifyError = err
      })
  }

  function readAllNotifications() {
    api("im.notify.read.all", {},
      function() {
        var out = []
        for (var i = 0; i < notifications.length; i++) {
          out.push({ id: notifications[i].id, module: notifications[i].module,
            title: notifications[i].title, text: notifications[i].text,
            time: notifications[i].time, unread: false, stamp: notifications[i].stamp, url: "" })
        }
        notifications = out
      },
      function(err) { notifyError = err })
  }

  // ---- задачи
  function openTaskById(id) {
    selectedTask = { id: id, title: "Задача " + id, deadline: "", description: "",
      creatorName: "", status: "2", chatId: 0, loading: true }
    api("tasks.task.get", { taskId: id },
      function(result) {
        var t = result && result.task ? result.task : {}
        selectedTask = {
          id: id,
          title: t.title || ("Задача " + id),
          deadline: t.deadline ? fmtTime(t.deadline) : "",
          description: stripBB(t.description || ""),
          creatorName: t.creator && t.creator.name ? t.creator.name : "",
          status: t.status || "2",
          chatId: t.chatId || t.chat_id || 0,
          loading: false
        }
      },
      function(err) {
        selectedTask = { id: id, title: "Задача " + id, deadline: "", description: "",
          creatorName: "", status: "2", chatId: 0, loading: false, error: err }
      })
  }

  function setTaskStatus(taskId, status, done) {
    api("tasks.task.update", { taskId: taskId, fields: { STATUS: status } },
      function() { done(true, "") },
      function(err) { done(false, err) })
  }

  function openChatById(dialogId, title) {
    var found = null
    for (var i = 0; i < chats.length; i++) {
      if (chats[i].dialog_id === dialogId) { found = chats[i]; break }
    }
    selectChat(found || { dialog_id: dialogId, title: title || dialogId, counter: 0,
      preview: "", time: "", stamp: 0, avatar: "", color: "#7a8a8a", online: false })
  }

  function openDialog(dialogId) {
    Util.execArgv(["omarchy-launch-webapp", portal + "/online/?IM_DIALOG=" + dialogId])
  }

  function openDialogRaw(url) {
    Util.execArgv(["omarchy-launch-webapp", url])
  }

  function selectChat(chat) {
    selectedChat = chat
    if (!hookBase) loadConfig()
    loadHistory(false)
  }

  onSelectedChatChanged: if (overlayLoader.item && selectedChat) overlayLoader.item.chat = selectedChat
  onSelectedTaskChanged: if (overlayLoader.item && selectedTask) overlayLoader.item.task = selectedTask

  function closeChat() {
    selectedChat = null
    messages = []
    historyError = ""
    sendError = ""
  }

  function closeTask() {
    selectedTask = null
    taskUpdateError = ""
  }

  // ---- вкладки
  component TabButton: WidgetButton {
    id: tabBtn
    property int value: 0
    bar: root.bar
    useActiveColor: false
    anchors.verticalCenter: parent.verticalCenter

    onPressed: function(b) { if (b === Qt.LeftButton) { root.closeChat(); root.closeTask(); root.tab = value } }
  }

  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.barIdentity
    bar: root.bar
    open: root.opened
    focusTarget: pages
    contentWidth: panel.fittedContentWidth(Style.space(440))
    contentHeight: panel.fittedContentHeight(Math.min(pages.implicitHeight, Style.space(600)))

    Flickable {
      anchors.fill: parent
      clip: true
      contentWidth: pages.width
      contentHeight: pages.implicitHeight
      interactive: false // внутренние страницы скроллят себя сами

      Item {
        id: pages
        width: panel.contentWidth - panel.padding * 2 - Border.left(panel.borderSpec) - Border.right(panel.borderSpec)
        implicitHeight: tabBar.height + Style.spacing.md
          + (overlayLoader.visible ? overlayLoader.implicitHeight
            : (tabPages.item ? tabPages.item.implicitHeight : 0))

        // ---- вкладки
        Row {
          id: tabBar
          spacing: Style.spacing.md
          width: parent.width

          TabButton { value: 0; text: root.tab === 0 && !root.selectedChat && !root.selectedTask ? "󰭹 чаты" : "󰭹" }
          TabButton { value: 1; text: root.tab === 1 && !root.selectedChat && !root.selectedTask ? "󰅎 уведы" : "󰅎" }
          TabButton { value: 2; text: root.tab === 2 && !root.selectedChat && !root.selectedTask ? "󰄬 задачи" : "󰄬" }
        }

        // ---- вкладочная страница
        Loader {
          id: tabPages
          y: tabBar.height + Style.spacing.md
          width: parent.width
          visible: !overlayLoader.visible
          source: root.tab === 0 ? "ChatsPage.qml" : root.tab === 1 ? "NotifyPage.qml" : "TasksPage.qml"
          onLoaded: if (item) item.ctrl = root
        }

        // ---- оверлей: чат или задача
        Loader {
          id: overlayLoader
          y: tabBar.height + Style.spacing.md
          width: parent.width
          visible: root.selectedChat !== null || root.selectedTask !== null
          source: root.selectedChat !== null ? "ChatPage.qml" : (root.selectedTask !== null ? "TaskPage.qml" : "")
          onLoaded: {
            if (item) {
              item.ctrl = root
              if (root.selectedChat !== null) item.chat = root.selectedChat
              if (root.selectedTask !== null) item.task = root.selectedTask
            }
          }
          onSourceChanged: {
            if (item) {
              if (root.selectedChat !== null) item.chat = root.selectedChat
              if (root.selectedTask !== null) item.task = root.selectedTask
            }
          }
        }
      }
    }
  }
}

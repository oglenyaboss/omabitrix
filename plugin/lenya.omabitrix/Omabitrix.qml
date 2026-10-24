import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

// Одна иконка Bitrix24 в баре: бейдж непрочитанных, клик — TUI omabitrix
// (foot floating, SUPER+B — глобальный хоткей). Данные — снапшот демона omabitrixd.
// QML-панель отключена в пользу TUI (исходники страниц лежат в repo).
BarWidget {
  id: root

  moduleName: "lenya.omabitrix"

  // портал для ПКМ: из настроек виджета, иначе из снапшота демона
  readonly property string portal: setting("portal", "") || (snap && snap.portal || "")
  readonly property string snapshotPath: Quickshell.env("HOME") + "/.local/state/omabitrix/snapshot.json"

  property var snap: null
  property string snapError: ""

  readonly property int unreadTotal: snap && !snapError
    ? (snap.chats || 0) + (snap.notify || 0)
    : 0
  readonly property bool failed: computeFailed()

  function computeFailed() {
    if (snapError !== "") return true
    if (snap === null || snap === undefined) return false
    var ts = Date.parse(snap.updated_at || "")
    if (!isNaN(ts) && Date.now() - ts > 60000) return true
    var e = snap.error
    return e !== undefined && e !== null && String(e) !== ""
  }

  visible: true

  function update(raw) {
    try {
      snap = JSON.parse(raw)
      snapError = snap && snap.error ? snap.error : ""
    } catch (error) {
      snapError = "демон не отвечает"
    }
  }

  function launchTUI(args) {
    var argv = ["foot", "-a", "omabitrix", "omabitrix"]
    for (var i = 0; i < args.length; i++) argv.push(args[i])
    Util.execArgv(argv)
  }

  function refresh() {
    if (!catProc.running) catProc.running = true
  }

  Component.onCompleted: refresh()

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  // ---- снапшот демона: раз в 5 секунд (файл атомарно перезаписывается)
  Timer {
    interval: 5000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  Process {
    id: catProc
    running: false
    command: ["cat", root.snapshotPath]
    stdout: StdioCollector {
      onStreamFinished: root.update(text)
    }
  }

  IpcHandler {
    target: "lenya.omabitrix"

    function refresh(): void { root.refresh() }
    function open(): void { root.launchTUI([]) }
    function close(): void {}
    function toggle(): void { root.launchTUI([]) }
    function openTask(id: string): void { root.launchTUI(["--task", id]) }
    function openChat(dialogId: string): void { root.launchTUI(["--chat", dialogId]) }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.failed ? "󰅚" : (root.unreadTotal > 0 ? "󰭹 " + root.unreadTotal : "󰭹")
    useActiveColor: false
    active: root.failed
    horizontalMargin: 8.75
    verticalPadding: 8.75
    tooltipText: root.failed
      ? "omabitrix: нет связи с порталом"
      : "Bitrix24 · чаты: " + (root.snap ? root.snap.chats || 0 : 0)
        + " · увед: " + (root.snap ? root.snap.notify || 0 : 0)
        + " · просрочено: " + (root.snap ? root.snap.overdue || 0 : 0)
        + " · сегодня: " + (root.snap ? root.snap.today || 0 : 0)

    onPressed: function(b) {
      if (b === Qt.RightButton && root.portal) Util.execArgv(["omarchy-launch-webapp", root.portal])
      else root.launchTUI([])
    }
  }
}

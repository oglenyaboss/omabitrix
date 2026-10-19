import QtQuick
import qs.Commons
import qs.Ui

// Карточка задачи: описание, постановщик, дедлайн, смена статуса,
// переход в чат задачи. ctrl = Panel.
Column {
  id: page
  property var ctrl: null
  property var task: null // {id,title,deadline,description,creatorName,status,chatId}

  readonly property color foreground: ctrl ? ctrl.foreground : "white"
  readonly property color dim: ctrl ? ctrl.dim : "gray"
  readonly property color accent: ctrl ? ctrl.accent : "teal"
  readonly property color urgentColor: ctrl ? ctrl.urgentColor : "red"
  readonly property string fontFamily: ctrl ? ctrl.fontFamily : ""

  function alpha(c, a) { return Qt.rgba(c.r, c.g, c.b, a) }

  readonly property var statusActions: [
    { status: 3, glyph: "▶", label: "В работу" },
    { status: 4, glyph: "󰦗", label: "На проверке" },
    { status: 5, glyph: "󰄬", label: "Завершить" },
    { status: 6, glyph: "󰅑", label: "Отложить" }
  ]

  function statusName(s) {
    var map = { 1: "новая", 2: "ждёт выполнения", 3: "выполняется", 4: "на проверке",
      5: "завершена", 6: "отложена", 7: "отклонена" }
    return map[Number(s)] || String(s)
  }

  function setStatus(st) {
    if (!ctrl || !task || ctrl.taskUpdating) return
    ctrl.taskUpdating = true
    ctrl.taskUpdateError = ""
    ctrl.setTaskStatus(task.id, st, function(ok, err) {
      ctrl.taskUpdating = false
      if (ok) {
        page.task = { id: task.id, title: task.title, deadline: task.deadline,
          description: task.description, creatorName: task.creatorName,
          status: st, chatId: task.chatId }
        page.updatedFlash = true
        flashTimer.restart()
      } else {
        ctrl.taskUpdateError = err
      }
    })
  }

  property bool updatedFlash: false

  Timer {
    id: flashTimer
    interval: 2500
    onTriggered: page.updatedFlash = false
  }

  spacing: ctrl ? ctrl.spacing : 4

  // ---- шапка
    Item {
      width: parent.width
      height: Math.max(backBtn.implicitHeight, taskTitle.implicitHeight + taskMeta.implicitHeight)

      Row {
        spacing: Style.spacing.sm

        WidgetButton {
          id: backBtn
          bar: ctrl && ctrl.bar ? ctrl.bar : null
          text: "󰁍"
          tooltipText: "к задачам"
          onPressed: function(b) { if (b === Qt.LeftButton && ctrl) ctrl.closeTask() }
          anchors.verticalCenter: parent.verticalCenter
        }

        Column {
          spacing: 1
          anchors.verticalCenter: parent.verticalCenter

          Text {
            id: taskTitle
            text: task ? task.title : ""
            color: page.foreground
            font.family: page.fontFamily
            font.pixelSize: ctrl ? ctrl.fontTitle : 14
            font.bold: true
            wrapMode: Text.WordWrap
            width: page.width - backBtn.width - chatBtn.width - Style.spacing.md * 4
            maximumLineCount: 2
            elide: Text.ElideRight
          }

          Row {
            id: taskMeta
            spacing: Style.spacing.sm

            Text {
              text: task ? statusName(task.status) : ""
              color: page.accent
              font.family: page.fontFamily
              font.pixelSize: ctrl ? ctrl.fontCaption : 10
            }

            Text {
              visible: task && task.deadline && task.deadline !== ""
              text: task ? "· 󰔞 " + task.deadline : ""
              color: page.dim
              font.family: page.fontFamily
              font.pixelSize: ctrl ? ctrl.fontCaption : 10
            }

            Text {
              visible: task && task.creatorName && task.creatorName !== ""
              text: task ? "· " + task.creatorName : ""
              color: page.dim
              font.family: page.fontFamily
              font.pixelSize: ctrl ? ctrl.fontCaption : 10
            }
          }
        }
      }

      WidgetButton {
        id: chatBtn
        visible: task && task.chatId
        bar: ctrl && ctrl.bar ? ctrl.bar : null
        text: "󰭹"
        tooltipText: "чат задачи"
        onPressed: function(b) {
          if (b === Qt.LeftButton && task && task.chatId && ctrl)
            ctrl.openChatById("chat" + task.chatId, task.title)
        }
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
      }
    }

    Text {
      visible: page.updatedFlash
      text: "✓ статус обновлён"
      color: page.accent
      font.family: page.fontFamily
      font.pixelSize: ctrl ? ctrl.fontCaption : 10
    }

    Text {
      visible: ctrl && ctrl.taskUpdateError !== ""
      text: ctrl ? "⚠ " + ctrl.taskUpdateError : ""
      color: page.urgentColor
      font.family: page.fontFamily
      font.pixelSize: ctrl ? ctrl.fontCaption : 10
    }

    // ---- описание
    Flickable {
      width: parent.width
      height: Math.min(descText.implicitHeight, ctrl ? ctrl.descCap : 240)
      contentHeight: descText.implicitHeight
      clip: true
      interactive: contentHeight > height

      Text {
        id: descText
        width: parent.width
        text: task && task.description ? task.description : "без описания"
        color: page.dim
        opacity: 0.9
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontBody : 12
        wrapMode: Text.WordWrap
      }
    }

    // ---- смена статуса
    Column {
      id: actionsCol
      width: parent.width
      spacing: 1

      PanelSectionHeader {
        visible: !page.isClosed
        text: "СМЕНИТЬ СТАТУС"
      }

      Repeater {
        model: page.visibleActions

        delegate: Rectangle {
          id: actDelegate
          required property var modelData

          visible: true
          width: parent.width
          height: actText.implicitHeight + Style.spacing.controlPaddingY
          radius: Style.cornerRadius
          color: actMA.containsMouse ? page.alpha(page.accent, 0.12) : page.alpha(page.foreground, 0.05)

          Text {
            id: actText
            x: Style.spacing.controlPaddingX
            width: parent.width - Style.spacing.controlPaddingX * 2
            anchors.verticalCenter: parent.verticalCenter
            text: actDelegate.modelData.glyph + "  " + actDelegate.modelData.label
            color: page.foreground
            font.family: page.fontFamily
            font.pixelSize: ctrl ? ctrl.fontBody : 12
          }

          MouseArea {
            id: actMA
            anchors.fill: parent
            hoverEnabled: true
            onClicked: page.setStatus(actDelegate.modelData.status)
          }
        }
      }
    }

  readonly property bool isClosed: task && (Number(task.status) === 5 || Number(task.status) === 6 || Number(task.status) === 7)

  readonly property var visibleActions: {
    var list = []
    if (!task) return list
    var cur = Number(task.status)
    if (page.isClosed) {
      list.push({ status: 2, glyph: "󰁋", label: "Переоткрыть" })
    } else {
      for (var i = 0; i < statusActions.length; i++) {
        if (statusActions[i].status !== cur) list.push(statusActions[i])
      }
    }
    return list
  }
}

import QtQuick
import qs.Commons
import qs.Ui

// Страница чата: шапка, история пузырями, ответ внизу. ctrl = Panel.
Column {
  id: page
  property var ctrl: null
  property var chat: null

  readonly property color foreground: ctrl ? ctrl.foreground : "white"
  readonly property color dim: ctrl ? ctrl.dim : "gray"
  readonly property color accent: ctrl ? ctrl.accent : "teal"
  readonly property color urgentColor: ctrl ? ctrl.urgentColor : "red"
  readonly property string fontFamily: ctrl ? ctrl.fontFamily : ""

  spacing: ctrl ? ctrl.spacing : 4

  function alpha(c, a) { return Qt.rgba(c.r, c.g, c.b, a) }

  function scrollDown() {
    historyFlick.contentY = Math.max(0, historyFlick.contentHeight - historyFlick.height)
  }

  function sendReply() {
    if (!ctrl || !chat || ctrl.sending) return
    var text = replyField.text.trim()
    if (text === "") return
    ctrl.sendReply(chat, text, function(ok) {
      if (ok) { replyField.text = ""; scrollDown() }
    })
  }

  function loadMore() {
    if (!ctrl || !chat || page.loadingMore) return
    page.loadingMore = true
    ctrl.loadOlder(chat.dialog_id, function(ok, rows) {
      page.loadingMore = false
      if (ok && rows && rows.length === 0) page.noMore = true
    })
  }

  property bool loadingMore: false
  property bool noMore: false

  // ---- шапка чата
  Item {
    width: parent.width
    height: Math.max(avatarItem.size, titleCol.implicitHeight)

    Row {
      spacing: Style.spacing.sm

      WidgetButton {
        bar: ctrl && ctrl.bar ? ctrl.bar : null
        text: "󰁍"
        tooltipText: "к чатам"
        onPressed: function(b) { if (b === Qt.LeftButton && ctrl) ctrl.closeChat() }
        anchors.verticalCenter: parent.verticalCenter
      }

      Avatar {
        id: avatarItem
        size: 24
        source: chat && chat.avatar ? chat.avatar : ""
        title: chat ? chat.title : ""
        colorHex: chat && chat.color ? chat.color : "#7a8a8a"
      }

      Column {
        id: titleCol
        spacing: 0
        anchors.verticalCenter: parent.verticalCenter

        Text {
          text: chat ? chat.title : ""
          color: page.foreground
          font.family: page.fontFamily
          font.pixelSize: ctrl ? ctrl.fontTitle : 14
          font.bold: true
          elide: Text.ElideMiddle
          width: page.width - avatarItem.width - browserChatBtn.width - Style.spacing.md * 4
        }

        Text {
          visible: ctrl && ctrl.loadingHistory && ctrl.messages.length === 0
          text: "загрузка…"
          color: page.dim
          font.family: page.fontFamily
          font.pixelSize: ctrl ? ctrl.fontCaption : 10
        }
      }
    }

    WidgetButton {
      id: browserChatBtn
      bar: ctrl && ctrl.bar ? ctrl.bar : null
      text: "↗"
      tooltipText: "открыть в браузере"
      onPressed: function(b) {
        if (b === Qt.LeftButton && chat && ctrl) ctrl.openDialog(chat.dialog_id)
      }
      anchors.right: parent.right
      anchors.verticalCenter: parent.verticalCenter
    }
  }

  // ---- история
  Flickable {
    id: historyFlick
    width: parent.width
    height: Math.min(messagesCol.implicitHeight, ctrl ? ctrl.historyCap : 360)
    contentHeight: messagesCol.implicitHeight
    clip: true
    interactive: contentHeight > height

    Column {
      id: messagesCol
      width: parent.width
      spacing: Style.spacing.xs

      Text {
        visible: !page.noMore && ctrl && ctrl.messages.length > 0
        text: page.loadingMore ? "…" : "раньше 󰁘"
        color: page.dim
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontCaption : 10
        anchors.horizontalCenter: parent.horizontalCenter

        MouseArea {
          anchors.fill: parent
          onClicked: page.loadMore()
        }
      }

      Text {
        visible: page.noMore && ctrl && ctrl.messages.length > 0
        text: "начало переписки"
        color: page.dim
        opacity: 0.6
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontCaption : 10
        anchors.horizontalCenter: parent.horizontalCenter
      }

      Text {
        visible: ctrl && ctrl.historyError !== ""
        text: ctrl ? "⚠ " + ctrl.historyError : ""
        color: page.urgentColor
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontBody : 12
      }

      Repeater {
        model: ctrl ? ctrl.regularMessages : []

        delegate: Column {
          id: bubbleRow
          required property var modelData
          required property int index
          readonly property var prev: bubbleRow.index > 0 && ctrl ? ctrl.regularMessages[bubbleRow.index - 1] : null
          readonly property bool mine: modelData.mine
          readonly property bool showName: !modelData.mine && modelData.name !== ""
            && (bubbleRow.prev === null || bubbleRow.prev.name !== modelData.name)
          readonly property bool showDaySplit: bubbleRow.prev !== null && bubbleRow.prev.dayKey !== modelData.dayKey
          width: parent.width
          spacing: 0

          // плашка-разделитель дат
          Item {
            visible: bubbleRow.showDaySplit
            width: parent.width
            height: dayPill.height + Style.spacing.xs

            Rectangle {
              id: dayPill
              width: dayPillText.implicitWidth + Style.spacing.controlPaddingX
              height: dayPillText.implicitHeight + 2
              radius: height / 2
              color: page.alpha(page.foreground, 0.08)
              anchors.horizontalCenter: parent.horizontalCenter

              Text {
                id: dayPillText
                anchors.centerIn: parent
                text: bubbleRow.modelData.dayLabel
                color: page.dim
                font.family: page.fontFamily
                font.pixelSize: ctrl ? ctrl.fontCaption : 10
              }
            }
          }

          Text {
            visible: bubbleRow.showName
            text: bubbleRow.modelData.name
            color: page.dim
            font.family: page.fontFamily
            font.pixelSize: ctrl ? ctrl.fontCaption : 10
          }

          Item {
            width: parent.width
            height: bubbleCol.implicitHeight

            Column {
              id: bubbleCol
              width: parent.width * 0.85
              x: bubbleRow.mine ? parent.width - width : 0
              spacing: 0

              Rectangle {
                width: Math.min(bubbleText.implicitWidth + Style.spacing.controlPaddingX * 2, bubbleCol.width)
                height: bubbleText.implicitHeight + Style.spacing.controlPaddingY
                radius: Style.cornerRadius
                color: bubbleRow.mine ? page.alpha(page.accent, 0.18) : page.alpha(page.foreground, 0.08)
                x: bubbleRow.mine ? parent.width - width : 0

                Text {
                  id: bubbleText
                  x: Style.spacing.controlPaddingX
                  y: Style.spacing.controlPaddingY / 2
                  width: Math.min(implicitWidth, bubbleCol.width - Style.spacing.controlPaddingX * 2)
                  text: bubbleRow.modelData.text
                  color: page.foreground
                  font.family: page.fontFamily
                  font.pixelSize: ctrl ? ctrl.fontBody : 12
                  wrapMode: Text.WordWrap
                }
              }
            }
          }

          Text {
            text: bubbleRow.modelData.time
            color: page.dim
            opacity: 0.6
            font.family: page.fontFamily
            font.pixelSize: ctrl ? ctrl.fontCaption : 10
            horizontalAlignment: bubbleRow.mine ? Text.AlignRight : Text.AlignLeft
            width: parent.width
          }
        }
      }

      Repeater {
        model: ctrl ? ctrl.systemMessages : []

        delegate: Text {
          required property var modelData
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: modelData.text
          color: page.dim
          opacity: 0.7
          font.family: page.fontFamily
          font.pixelSize: ctrl ? ctrl.fontCaption : 10
          wrapMode: Text.WordWrap
        }
      }
    }
  }

  // ---- ответ
  Row {
    width: parent.width
    spacing: Style.spacing.sm

    TextField {
      id: replyField
      width: parent.width - sendBtn.width - parent.spacing
      placeholderText: ctrl && ctrl.sending ? "отправка…" : "ответить…"
      enabled: !(ctrl && ctrl.sending)
      foreground: page.foreground
      font.family: page.fontFamily
      onAccepted: page.sendReply()
      Component.onCompleted: if (ctrl) ctrl.replyFieldRef = replyField
    }

    WidgetButton {
      id: sendBtn
      bar: ctrl && ctrl.bar ? ctrl.bar : null
      text: ctrl && ctrl.sending ? "…" : "󰁝"
      onPressed: function(b) { if (b === Qt.LeftButton) page.sendReply() }
      anchors.verticalCenter: parent.verticalCenter
    }
  }

  Text {
    visible: ctrl && ctrl.sendError !== ""
    text: ctrl ? "⚠ не отправлено: " + ctrl.sendError : ""
    color: page.urgentColor
    font.family: page.fontFamily
    font.pixelSize: ctrl ? ctrl.fontCaption : 10
  }
}

import QtQuick
import qs.Commons
import qs.Ui

// Страница системных уведомлений (im.notify.get). ctrl = Panel.
Column {
  id: page
  property var ctrl: null

  readonly property color foreground: ctrl ? ctrl.foreground : "white"
  readonly property color dim: ctrl ? ctrl.dim : "gray"
  readonly property color accent: ctrl ? ctrl.accent : "teal"
  readonly property string fontFamily: ctrl ? ctrl.fontFamily : ""

  spacing: ctrl ? ctrl.spacing : 4

  function alpha(c, a) { return Qt.rgba(c.r, c.g, c.b, a) }

  function glyphFor(n) {
    var m = String(n && n.module || "")
    if (m === "tasks") return "󰄬"
    if (m === "calendar") return "󰃭"
    if (m === "im") return "󰅎"
    if (m === "disk") return "󰋊"
    return "󰅎"
  }

  Item {
    width: parent.width
    height: Math.max(readAllBtn.implicitHeight, hdrText.implicitHeight)

    Text {
      id: hdrText
      text: "Уведомления"
      color: page.foreground
      font.family: page.fontFamily
      font.pixelSize: ctrl ? ctrl.fontTitle : 14
      font.bold: true
      anchors.verticalCenter: parent.verticalCenter
    }

    WidgetButton {
      id: readAllBtn
      bar: ctrl && ctrl.bar ? ctrl.bar : null
      text: "󰗑 всё прочитано"
      tooltipText: "пометить все прочитанными"
      onPressed: function(b) { if (b === Qt.LeftButton && ctrl) ctrl.readAllNotifications() }
      anchors.right: parent.right
      anchors.verticalCenter: parent.verticalCenter
    }
  }

  Text {
    visible: ctrl && ctrl.loadingNotify && ctrl.notifications.length === 0
    text: "загрузка…"
    color: page.dim
    font.family: page.fontFamily
    font.pixelSize: ctrl ? ctrl.fontBody : 12
  }

  Text {
    visible: ctrl && ctrl.notifyError !== ""
    text: ctrl ? "⚠ " + ctrl.notifyError : ""
    color: ctrl ? ctrl.urgentColor : "red"
    font.family: page.fontFamily
    font.pixelSize: ctrl ? ctrl.fontBody : 12
  }

  Flickable {
    width: parent.width
    height: Math.min(notifyCol.implicitHeight, ctrl ? ctrl.listCap : 400)
    contentHeight: notifyCol.implicitHeight
    clip: true
    interactive: contentHeight > height

    Column {
      id: notifyCol
      width: parent.width
      spacing: 1

      Repeater {
        model: ctrl ? ctrl.notifications : []

        delegate: Rectangle {
          id: nDelegate
          required property var modelData

          width: parent.width
          height: nBody.implicitHeight + Style.spacing.controlPaddingY
          radius: Style.cornerRadius
          color: nDelegate.modelData.unread
            ? page.alpha(page.accent, 0.06)
            : (nMA.containsMouse ? page.alpha(page.foreground, 0.09) : "transparent")

          Row {
            spacing: Style.spacing.sm
            x: Style.spacing.controlPaddingX
            width: parent.width - Style.spacing.controlPaddingX * 2
            anchors.verticalCenter: parent.verticalCenter

            Text {
              text: page.glyphFor(nDelegate.modelData)
              color: page.dim
              font.family: page.fontFamily
              font.pixelSize: ctrl ? ctrl.fontTitle : 14
            }

            Column {
              id: nBody
              width: parent.width - Style.spacing.sm * 2 - nStamp.width
              spacing: 1

              Text {
                text: nDelegate.modelData.title !== "" ? nDelegate.modelData.title : nDelegate.modelData.text
                color: page.foreground
                opacity: nDelegate.modelData.unread ? 1.0 : 0.8
                font.family: page.fontFamily
                font.pixelSize: ctrl ? ctrl.fontBody : 12
                font.bold: nDelegate.modelData.unread
                elide: Text.ElideRight
                width: parent.width
                visible: text !== ""
              }

              Text {
                text: nDelegate.modelData.title !== "" ? nDelegate.modelData.text : ""
                color: page.dim
                font.family: page.fontFamily
                font.pixelSize: ctrl ? ctrl.fontCaption : 10
                elide: Text.ElideRight
                width: parent.width
                visible: text !== ""
              }
            }

            Text {
              id: nStamp
              text: nDelegate.modelData.time
              color: page.dim
              font.family: page.fontFamily
              font.pixelSize: ctrl ? ctrl.fontCaption : 10
              anchors.verticalCenter: parent.verticalCenter
            }
          }

          MouseArea {
            id: nMA
            anchors.fill: parent
            hoverEnabled: true
          }
        }
      }

      Text {
        visible: ctrl && ctrl.notifications.length === 0 && !ctrl.loadingNotify && ctrl.notifyError === ""
        text: "пусто"
        color: page.dim
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontBody : 12
      }
    }
  }
}

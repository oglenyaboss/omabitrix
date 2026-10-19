import QtQuick
import qs.Commons
import qs.Ui

// Страница списка чатов. Корень — Column: implicitHeight честный,
// иначе панель складывается в ноль. ctrl = Panel.
Column {
  id: page
  property var ctrl: null

  readonly property color foreground: ctrl ? ctrl.foreground : "white"
  readonly property color dim: ctrl ? ctrl.dim : "gray"
  readonly property color accent: ctrl ? ctrl.accent : "teal"
  readonly property string fontFamily: ctrl ? ctrl.fontFamily : ""

  spacing: ctrl ? ctrl.spacing : 4

  function alpha(c, a) { return Qt.rgba(c.r, c.g, c.b, a) }

  TextField {
    id: searchField
    width: parent.width
    placeholderText: "поиск чата…"
    foreground: page.foreground
    font.family: page.fontFamily
    onTextEdited: if (ctrl) ctrl.filterText = text
    Component.onCompleted: if (ctrl) ctrl.searchFieldRef = searchField
  }

  Text {
    visible: ctrl && ctrl.loadingChats && ctrl.chats.length === 0
    text: "загрузка…"
    color: page.dim
    font.family: page.fontFamily
    font.pixelSize: ctrl ? ctrl.fontBody : 12
  }

  Text {
    visible: ctrl && ctrl.chatsError !== ""
    text: ctrl ? "⚠ " + ctrl.chatsError : ""
    color: ctrl ? ctrl.urgentColor : "red"
    font.family: page.fontFamily
    font.pixelSize: ctrl ? ctrl.fontBody : 12
  }

  Flickable {
    width: parent.width
    height: Math.min(chatsCol.implicitHeight, ctrl ? ctrl.listCap : 400)
    contentHeight: chatsCol.implicitHeight
    clip: true
    interactive: contentHeight > height

    Column {
      id: chatsCol
      width: parent.width
      spacing: 1

      Repeater {
        model: ctrl ? ctrl.filteredChats : []

        delegate: Rectangle {
          id: chatDelegate
          required property var modelData
          readonly property bool unread: modelData.counter > 0

          width: parent.width
          height: Math.max(avatarItem.size, chatTexts.implicitHeight) + Style.spacing.controlPaddingY
          radius: Style.cornerRadius
          color: chatDelegateMA.containsMouse
            ? page.alpha(page.foreground, 0.09)
            : (chatDelegate.unread ? page.alpha(page.accent, 0.05) : "transparent")

          Row {
            spacing: Style.spacing.sm
            x: Style.spacing.controlPaddingX
            width: parent.width - Style.spacing.controlPaddingX * 2
            anchors.verticalCenter: parent.verticalCenter

            Avatar {
              id: avatarItem
              size: 26
              source: chatDelegate.modelData.avatar
              title: chatDelegate.modelData.title
              colorHex: chatDelegate.modelData.color
              online: chatDelegate.modelData.online
            }

            Column {
              id: chatTexts
              width: parent.width - avatarItem.width - Style.spacing.sm * 2 - chatRight.width
              spacing: 1
              anchors.verticalCenter: parent.verticalCenter

              Text {
                text: chatDelegate.modelData.title
                color: page.foreground
                opacity: chatDelegate.unread ? 1.0 : 0.85
                font.family: page.fontFamily
                font.pixelSize: ctrl ? ctrl.fontBody : 12
                font.bold: chatDelegate.unread
                elide: Text.ElideRight
                width: parent.width
              }

              Text {
                visible: chatDelegate.modelData.preview !== ""
                text: chatDelegate.modelData.preview
                color: page.dim
                font.family: page.fontFamily
                font.pixelSize: ctrl ? ctrl.fontCaption : 10
                elide: Text.ElideRight
                width: parent.width
              }
            }

            Column {
              id: chatRight
              spacing: 2
              anchors.verticalCenter: parent.verticalCenter

              Text {
                text: chatDelegate.modelData.time
                color: page.dim
                font.family: page.fontFamily
                font.pixelSize: ctrl ? ctrl.fontCaption : 10
                anchors.horizontalCenter: parent.horizontalCenter
              }

              Rectangle {
                visible: chatDelegate.unread
                width: counterLabel.implicitWidth + Style.spacing.xs
                height: counterLabel.implicitHeight + 2
                radius: height / 2
                color: page.alpha(page.accent, 0.22)
                anchors.horizontalCenter: parent.horizontalCenter

                Text {
                  id: counterLabel
                  anchors.centerIn: parent
                  text: chatDelegate.unread ? String(chatDelegate.modelData.counter) : ""
                  color: page.accent
                  font.family: page.fontFamily
                  font.pixelSize: ctrl ? ctrl.fontCaption : 10
                  font.bold: true
                }
              }
            }
          }

          MouseArea {
            id: chatDelegateMA
            anchors.fill: parent
            hoverEnabled: true
            onClicked: if (ctrl) ctrl.selectChat(chatDelegate.modelData)
          }
        }
      }

      Text {
        visible: ctrl && ctrl.chats.length === 0 && !ctrl.loadingChats && ctrl.chatsError === ""
        text: "чатов нет"
        color: page.dim
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontBody : 12
      }
    }
  }
}

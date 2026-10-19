import QtQuick
import qs.Commons
import qs.Ui

// Страница задач: hero + просрочка + сегодня. ctrl = Panel.
Column {
  id: page
  property var ctrl: null

  readonly property color foreground: ctrl ? ctrl.foreground : "white"
  readonly property color dim: ctrl ? ctrl.dim : "gray"
  readonly property color accent: ctrl ? ctrl.accent : "teal"
  readonly property color urgentColor: ctrl ? ctrl.urgentColor : "red"
  readonly property string fontFamily: ctrl ? ctrl.fontFamily : ""

  spacing: ctrl ? ctrl.spacing : 4

  function alpha(c, a) { return Qt.rgba(c.r, c.g, c.b, a) }

  // hero: что ближе всего
  Rectangle {
    visible: ctrl && ctrl.todayTasks.length > 0
    width: parent.width
    height: heroCol.implicitHeight + Style.spacing.controlPaddingY * 2
    radius: Style.cornerRadius
    color: page.alpha(page.accent, 0.06)

    Column {
      id: heroCol
      x: Style.spacing.controlPaddingX
      width: parent.width - Style.spacing.controlPaddingX * 2
      anchors.verticalCenter: parent.verticalCenter
      spacing: 1

      Text {
        text: "БЛИЖАЙШИЙ ДЕДЛАЙН"
        color: page.dim
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontCaption : 10
        font.bold: true
      }

      Text {
        text: ctrl && ctrl.todayTasks.length > 0
          ? "󰔞 " + ctrl.todayTasks[0].title + "  ·  " + (ctrl.todayTasks[0].deadline || "")
          : ""
        color: page.foreground
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontBody : 12
        elide: Text.ElideRight
        width: parent.width
      }
    }

    MouseArea {
      anchors.fill: parent
      onClicked: if (ctrl && ctrl.todayTasks.length > 0) ctrl.openTaskById(ctrl.todayTasks[0].id)
    }
  }

  Flickable {
    width: parent.width
    height: Math.min(tasksCol.implicitHeight, ctrl ? ctrl.listCap : 400)
    contentHeight: tasksCol.implicitHeight
    clip: true
    interactive: contentHeight > height

    Column {
      id: tasksCol
      width: parent.width
      spacing: 1

      PanelSectionHeader {
        visible: ctrl && ctrl.overdueTasks.length > 0
        text: "ПРОСРОЧЕНО · " + (ctrl ? ctrl.overdueTasks.length : 0)
      }

      Repeater {
        model: ctrl ? ctrl.overdueTasks : []

        delegate: Rectangle {
          id: ovDelegate
          required property var modelData

          width: parent.width
          height: ovText.implicitHeight + Style.spacing.controlPaddingY
          radius: Style.cornerRadius
          color: ovMA.containsMouse ? page.alpha(page.urgentColor, 0.14) : page.alpha(page.urgentColor, 0.06)

          Text {
            id: ovText
            x: Style.spacing.controlPaddingX
            width: parent.width - Style.spacing.controlPaddingX * 2
            anchors.verticalCenter: parent.verticalCenter
            text: "󰀪 " + ovDelegate.modelData.title
              + (ovDelegate.modelData.deadline ? "  ·  " + ovDelegate.modelData.deadline : "")
            color: page.urgentColor
            font.family: page.fontFamily
            font.pixelSize: ctrl ? ctrl.fontBody : 12
            elide: Text.ElideRight
          }

          MouseArea {
            id: ovMA
            anchors.fill: parent
            hoverEnabled: true
            onClicked: if (ctrl) ctrl.openTaskById(ovDelegate.modelData.id)
          }
        }
      }

      PanelSectionHeader {
        visible: ctrl && ctrl.todayTasks.length > 0
        text: "СЕГОДНЯ · " + (ctrl ? ctrl.todayTasks.length : 0)
      }

      Repeater {
        model: ctrl ? ctrl.todayTasks : []

        delegate: Rectangle {
          id: tdDelegate
          required property var modelData

          width: parent.width
          height: tdText.implicitHeight + Style.spacing.controlPaddingY
          radius: Style.cornerRadius
          color: tdMA.containsMouse ? page.alpha(page.foreground, 0.09) : "transparent"

          Text {
            id: tdText
            x: Style.spacing.controlPaddingX
            width: parent.width - Style.spacing.controlPaddingX * 2
            anchors.verticalCenter: parent.verticalCenter
            text: "󰔞 " + tdDelegate.modelData.title
              + (tdDelegate.modelData.deadline ? "  ·  " + tdDelegate.modelData.deadline : "")
            color: page.foreground
            font.family: page.fontFamily
            font.pixelSize: ctrl ? ctrl.fontBody : 12
            elide: Text.ElideRight
          }

          MouseArea {
            id: tdMA
            anchors.fill: parent
            hoverEnabled: true
            onClicked: if (ctrl) ctrl.openTaskById(tdDelegate.modelData.id)
          }
        }
      }

      Text {
        visible: ctrl && ctrl.overdueTasks.length === 0 && ctrl.todayTasks.length === 0
        text: "на сегодня ничего"
        color: page.dim
        font.family: page.fontFamily
        font.pixelSize: ctrl ? ctrl.fontBody : 12
      }
    }
  }
}

import QtQuick
import QtQuick.Effects
import qs.Commons

// ЧБ-аватар с кругом и фоллбэком на инициалы поверх цвета чата.
// Аватары Битрикса цветные — приглушаем до монохрома в стиле омарчи.
Item {
  id: root

  property string source: ""
  property string title: ""
  property string colorHex: "#7a8a8a"
  property int size: 26
  readonly property color tint: {
    var h = String(colorHex).replace("#", "")
    if (h.length === 6) {
      var r = parseInt(h.substring(0, 2), 16) / 255
      var g = parseInt(h.substring(2, 4), 16) / 255
      var b = parseInt(h.substring(4, 6), 16) / 255
      return Qt.rgba(r, g, b, 0.25)
    }
    return Qt.rgba(0.5, 0.5, 0.5, 0.25)
  }

  width: size
  height: size

  function initials() {
    var words = String(title || "").split(/\s+/).filter(function(w) { return w.length > 0 })
    var s = ""
    for (var i = 0; i < Math.min(2, words.length); i++) s += words[i][0].toUpperCase()
    return s || "?"
  }

  Rectangle {
    anchors.fill: parent
    radius: width / 2
    color: root.tint
    visible: img.status !== Image.Ready

    Text {
      anchors.centerIn: parent
      text: root.initials()
      color: root.tint
      font.family: Style.font.family
      font.pixelSize: root.size * 0.42
      font.bold: true
    }
  }

  Image {
    id: img
    anchors.fill: parent
    visible: false
    source: root.source
    fillMode: Image.PreserveAspectCrop
    asynchronous: true
  }

  // чб + круглая маска
  MultiEffect {
    anchors.fill: parent
    visible: img.status === Image.Ready
    source: img
    saturation: -1
    autoPaddingEnabled: false
    maskEnabled: true
    maskThresholdMin: 0.0
    maskThresholdMax: 0.5
    maskSource: circleMask
  }

  ShaderEffectSource {
    id: circleMask
    sourceItem: Rectangle {
      width: root.size
      height: root.size
      radius: root.size / 2
      color: "white"
    }
    live: false
    hideSource: true
  }

  // онлайн-точка для личных диалогов
  Rectangle {
    visible: root.online
    width: root.size * 0.28
    height: width
    radius: width / 2
    color: "#78b88a"
    anchors.right: parent.right
    anchors.bottom: parent.bottom
    border.width: 1
    border.color: root.tint
  }

  property bool online: false
}

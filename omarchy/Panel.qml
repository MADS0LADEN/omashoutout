import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import Quickshell
import Quickshell.Io
import qs.Ui
import qs.Commons
import "Model.js" as Model

Panel {
  id: root
  moduleName: "io.github.lkarlslund.shoutout"
  ipcTarget: "io.github.lkarlslund.shoutout"

  readonly property string localBinary: Quickshell.env("HOME") + "/.local/bin/shoutout"
  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property color dim: Qt.darker(foreground, 1.55)
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family

  property bool dirty: false
  property bool usePathBinary: false
  property bool showMp3Codec: false
  property bool initialLoadPending: false
  property string cliKind: ""
  property var baseConfig: ({})
  property var devices: []
  property string statusText: ""
  property string errorText: ""
  property string applyNotice: ""

  property bool formEnabled: true
  property string formDeviceId: ""
  property string formDeviceName: ""
  property string formHost: ""
  property int formPort: 8009
  property string formEndpoint: ""
  property int formReceiverPercent: 1
  property string formPreset: "balanced"
  property string formCodec: "cast-opus"
  property int formSegmentMs: 500
  property int formTargetDelayMs: 100
  property string formBitrate: "192"

  readonly property var destinationOptions: Model.destinationOptions(devices, baseConfig, formDeviceId)
  readonly property var encodingOptions: Model.encodingOptions(showMp3Codec)

  function markDirty() {
    dirty = true
    applyNotice = ""
  }

  function markCustomPreset() {
    if (formPreset !== "custom") formPreset = "custom"
    markDirty()
  }

  function shoutoutCommand(subcommand) {
    return usePathBinary ? ["shoutout", subcommand] : [localBinary, subcommand]
  }

  function commandFailedToStart(exitCode, exitStatus, stdoutReceived) {
    return exitStatus !== 0 && !stdoutReceived
  }

  function currentFormState() {
    return {
      enabled: formEnabled,
      device_id: formDeviceId,
      device_name: formDeviceName,
      host: formHost,
      port: formPort,
      receiver_volume: formReceiverPercent / 100,
      preset: formPreset,
      codec: formCodec,
      segment_ms: formSegmentMs,
      target_delay_ms: formTargetDelayMs,
      bitrate_kbps: parseInt(formBitrate, 10)
    }
  }

  function applyConfigToForm(config) {
    var f = Model.formFromConfig(config)
    formEnabled = f.enabled
    formDeviceId = f.device_id
    formDeviceName = f.device_name
    formHost = f.host
    formPort = f.port
    formEndpoint = f.endpoint
    formReceiverPercent = Math.max(0, Math.min(100, Math.round(f.receiver_volume * 100)))
    formPreset = f.preset
    formCodec = f.codec
    formSegmentMs = f.segment_ms
    formTargetDelayMs = f.target_delay_ms
    formBitrate = String(f.bitrate_kbps)
    showMp3Codec = formCodec === "mp3"
    baseConfig = config
  }

  function selectDestination(deviceId) {
    formDeviceId = String(deviceId || "")
    var dev = Model.findDevice(devices, formDeviceId)
    if (dev) {
      formDeviceName = String(dev.name || "")
      formHost = String(dev.host || "")
      formPort = Model.parsePort(dev.port) || 8009
      formEndpoint = Model.formatEndpoint(formHost, formPort)
    }
    markDirty()
  }

  function syncEndpointFromField() {
    var parsed = Model.parseEndpoint(formEndpoint)
    if (!parsed.ok) return
    formHost = parsed.host
    formPort = parsed.port
  }

  function applyPresetChoice(preset) {
    formPreset = preset
    if (preset === "custom") {
      markDirty()
      return
    }
    var patch = Model.presetPatch(preset)
    if (patch.codec) formCodec = patch.codec
    if (patch.bitrate_kbps) formBitrate = String(patch.bitrate_kbps)
    if (patch.target_delay_ms !== undefined) formTargetDelayMs = patch.target_delay_ms
    if (patch.segment_ms !== undefined) formSegmentMs = patch.segment_ms
    markDirty()
  }

  function ingestStatusPayload(raw) {
    var payload
    try {
      payload = JSON.parse(raw)
    } catch (e) {
      errorText = Model.display("Invalid response from shoutout status.")
      return
    }

    if (payload.error) errorText = Model.display(String(payload.error))
    else if (errorText.indexOf("CLI not found") < 0) errorText = ""

    if (Array.isArray(payload.devices)) {
      devices = payload.devices.slice(0, 64)
    }

    if (payload.status) statusText = Model.statusLine(payload.status)

    if (initialLoadPending && payload.config && !dirty) {
      applyConfigToForm(payload.config)
      initialLoadPending = false
    }
  }

  function ingestConfigPayload(raw) {
    var config
    try {
      config = JSON.parse(raw)
    } catch (e) {
      errorText = Model.display("Invalid response from shoutout config.")
      return
    }
    if (!dirty) applyConfigToForm(config)
    initialLoadPending = false
  }

  function ingestApplyPayload(raw, exitCode) {
    if (exitCode !== 0) {
      errorText = Model.display(cliStderr.text || "Apply failed.")
      return
    }
    var config
    try {
      config = JSON.parse(raw)
    } catch (e) {
      errorText = Model.display("Invalid response from shoutout apply.")
      return
    }
    applyConfigToForm(config)
    dirty = false
    errorText = ""
    applyNotice = "Settings applied. Volume scale updates without reconnecting."
  }

  function handleCliFinished(exitCode, exitStatus) {
    var stdoutReceived = cliStdout.gotData
    if (commandFailedToStart(exitCode, exitStatus, stdoutReceived)) {
      if (!usePathBinary) {
        usePathBinary = true
        Qt.callLater(retryCli)
        return
      }
      errorText = Model.display("ShoutOut CLI not found. Install the shoutout binary first.")
      statusText = Model.display(Model.offlineStatusMessage())
      initialLoadPending = false
      return
    }

    var raw = String(cliStdout.text || "")
    if (raw.length > 65536) {
      errorText = Model.display("Response too large.")
      initialLoadPending = false
      return
    }

    if (exitCode !== 0 && raw === "") {
      var err = String(cliStderr.text || "").trim()
      errorText = Model.display(err !== "" ? err : "Could not reach ShoutOut.")
      statusText = Model.display(Model.offlineStatusMessage())
      initialLoadPending = false
      return
    }

    if (cliKind === "status") ingestStatusPayload(raw)
    else if (cliKind === "config") ingestConfigPayload(raw)
    else if (cliKind === "apply") ingestApplyPayload(raw, exitCode)
    cliKind = ""
  }

  function retryCli() {
    var kind = cliKind
    if (kind === "") return
    startCli(kind, pendingApplyBody)
  }

  property var pendingApplyBody: null

  function startCli(kind, applyBody) {
    if (cliProc.running) return
    cliKind = kind
    pendingApplyBody = applyBody || null
    cliStdout.gotData = false
    cliProc.command = shoutoutCommand(kind)
    cliProc.running = true
  }

  function refreshStatus() {
    startCli("status")
  }

  function applySettings() {
    var parsed = Model.parseEndpoint(formEndpoint)
    if (!parsed.ok) {
      errorText = Model.display("Enter a valid receiver address (host:port).")
      return
    }
    formHost = parsed.host
    formPort = parsed.port
    var body = Model.configFromForm(baseConfig, currentFormState())
    startCli("apply", body)
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  onOpenedChanged: {
    if (opened) {
      applyNotice = ""
      if (!dirty) {
        initialLoadPending = true
        refreshStatus()
      }
      pollTimer.start()
      Qt.callLater(function() { keyCatcher.forceActiveFocus() })
    } else {
      pollTimer.stop()
    }
  }

  Timer {
    id: pollTimer
    interval: 2000
    repeat: true
    onTriggered: if (root.opened) root.refreshStatus()
  }

  Process {
    id: cliProc
    stdinEnabled: true
    stdout: StdioCollector {
      id: cliStdout
      property bool gotData: false
      waitForEnd: true
      onStreamFinished: {
        gotData = String(text || "") !== ""
      }
    }
    stderr: StdioCollector {
      id: cliStderr
      waitForEnd: true
    }
    onStarted: {
      if (root.cliKind === "apply" && root.pendingApplyBody) {
        write(JSON.stringify(root.pendingApplyBody) + "\n")
        root.pendingApplyBody = null
      }
    }
    onExited: function(exitCode, exitStatus) {
      root.handleCliFinished(exitCode, exitStatus)
    }
  }

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: ""
    tooltipText: "ShoutOut"
    onPressed: root.toggle()
  }

  KeyboardPanel {
    id: panel
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(420))
    contentHeight: panel.fittedContentHeight(column.implicitHeight, Style.space(640))

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      blocked: destinationDropdown.popupOpen || presetDropdown.popupOpen
        || codecDropdown.popupOpen || bitrateDropdown.popupOpen
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }

      Flickable {
        id: panelFlick
        anchors.fill: parent
        contentWidth: width
        contentHeight: column.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        flickableDirection: Flickable.VerticalFlick
        interactive: contentHeight > height
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        Column {
          id: column
          width: panelFlick.width
          spacing: Style.space(14)

          Text {
            textFormat: Text.PlainText
            text: "ShoutOut"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.title
            font.bold: true
            width: parent.width
          }

          Text {
            textFormat: Text.PlainText
            text: "Volume and mute are the ShoutOut output in the Audio panel."
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
            wrapMode: Text.WordWrap
            width: parent.width
          }

          Toggle {
            width: parent.width
            label: "Enable device"
            checked: root.formEnabled
            foreground: root.foreground
            fontFamily: root.fontFamily
            onClicked: {
              root.formEnabled = !root.formEnabled
              root.markDirty()
            }
          }

          Dropdown {
            id: destinationDropdown
            width: parent.width
            label: "Destination"
            foreground: root.foreground
            fontFamily: root.fontFamily
            options: root.destinationOptions
            value: root.formDeviceId
            onChanged: function(v) { root.selectDestination(v) }
          }

          Text {
            textFormat: Text.PlainText
            text: "Receiver address"
            color: Qt.darker(root.foreground, 1.4)
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            font.bold: true
            width: parent.width
          }

          TextField {
            width: parent.width
            placeholderText: "192.168.1.10:8009"
            text: root.formEndpoint
            foreground: root.foreground
            onTextEdited: {
              root.formEndpoint = text
              root.formDeviceId = ""
              root.markDirty()
            }
            onEditingFinished: root.syncEndpointFromField()
          }

          Text {
            textFormat: Text.PlainText
            text: "Receiver volume at full desktop volume"
            color: Qt.darker(root.foreground, 1.4)
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            font.bold: true
            width: parent.width
          }

          RowLayout {
            width: parent.width
            spacing: Style.space(10)

            PanelSlider {
              Layout.fillWidth: true
              bar: root.bar
              minimum: 0
              maximum: 100
              step: 1
              integer: true
              value: root.formReceiverPercent
              onMoved: function(v) { root.formReceiverPercent = Math.round(v) }
              onReleased: function(v) {
                root.formReceiverPercent = Math.round(v)
                root.markDirty()
              }
            }

            Text {
              textFormat: Text.PlainText
              text: String(root.formReceiverPercent) + "%"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              Layout.alignment: Qt.AlignVCenter
            }
          }

          Dropdown {
            id: presetDropdown
            width: parent.width
            label: "Playback preset"
            foreground: root.foreground
            fontFamily: root.fontFamily
            options: Model.presetOptions()
            value: root.formPreset
            onChanged: function(v) { root.applyPresetChoice(v) }
          }

          Dropdown {
            id: codecDropdown
            width: parent.width
            label: "Encoding"
            foreground: root.foreground
            fontFamily: root.fontFamily
            options: root.encodingOptions
            value: root.formCodec
            onChanged: function(v) {
              root.formCodec = v
              root.markCustomPreset()
            }
          }

          Text {
            textFormat: Text.PlainText
            text: "Live segment length"
            color: Qt.darker(root.foreground, 1.4)
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            font.bold: true
            width: parent.width
          }

          RowLayout {
            width: parent.width
            spacing: Style.space(10)
            opacity: root.formCodec === "aac-hls" ? 1.0 : 0.45

            PanelSlider {
              Layout.fillWidth: true
              bar: root.bar
              minimum: 250
              maximum: 2000
              step: 250
              integer: true
              enabled: root.formCodec === "aac-hls"
              value: root.formSegmentMs
              onMoved: function(v) { root.formSegmentMs = Math.round(v) }
              onReleased: function(v) {
                root.formSegmentMs = Math.round(v)
                root.markCustomPreset()
              }
            }

            Text {
              textFormat: Text.PlainText
              text: String(root.formSegmentMs) + " ms"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              Layout.alignment: Qt.AlignVCenter
            }
          }

          Text {
            textFormat: Text.PlainText
            text: "Cast Streaming target delay"
            color: Qt.darker(root.foreground, 1.4)
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            font.bold: true
            width: parent.width
          }

          RowLayout {
            width: parent.width
            spacing: Style.space(10)
            opacity: root.formCodec === "cast-opus" ? 1.0 : 0.45

            PanelSlider {
              Layout.fillWidth: true
              bar: root.bar
              minimum: 10
              maximum: 1000
              step: 10
              integer: true
              enabled: root.formCodec === "cast-opus"
              value: root.formTargetDelayMs
              onMoved: function(v) { root.formTargetDelayMs = Math.round(v) }
              onReleased: function(v) {
                root.formTargetDelayMs = Math.round(v)
                root.markCustomPreset()
              }
            }

            Text {
              textFormat: Text.PlainText
              text: String(root.formTargetDelayMs) + " ms"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              Layout.alignment: Qt.AlignVCenter
            }
          }

          Text {
            textFormat: Text.PlainText
            text: "Requested receiver buffer, not measured end-to-end latency."
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            wrapMode: Text.WordWrap
            width: parent.width
          }

          Dropdown {
            id: bitrateDropdown
            width: parent.width
            label: "Encoding bitrate"
            foreground: root.foreground
            fontFamily: root.fontFamily
            options: Model.bitrateOptions()
            value: root.formBitrate
            onChanged: function(v) {
              root.formBitrate = v
              root.markCustomPreset()
            }
          }

          Text {
            textFormat: Text.PlainText
            text: root.statusText
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
            wrapMode: Text.WordWrap
            width: parent.width
          }

          Text {
            textFormat: Text.PlainText
            visible: root.errorText !== ""
            text: root.errorText
            color: root.bar ? root.bar.urgent : Color.urgent
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            wrapMode: Text.WordWrap
            width: parent.width
          }

          Text {
            textFormat: Text.PlainText
            visible: root.applyNotice !== ""
            text: root.applyNotice
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
            wrapMode: Text.WordWrap
            width: parent.width
          }

          Button {
            width: parent.width
            text: "Apply"
            foreground: root.foreground
            fontFamily: root.fontFamily
            onClicked: root.applySettings()
          }
        }
      }
    }
  }
}

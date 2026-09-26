function display(value) {
  var text = String(value || "")
  var out = ""
  for (var i = 0; i < text.length && out.length < 160; i++) {
    var ch = text.charAt(i)
    var code = text.charCodeAt(i)
    if (code < 32 || code === 127) continue
    if (ch === "<" || ch === ">" || ch === "&") continue
    out += ch
  }
  return out
}

function parsePort(text) {
  var portStr = String(text || "").trim()
  if (portStr === "" || !/^\d+$/.test(portStr)) return NaN
  var port = parseInt(portStr, 10)
  if (!isFinite(port) || port < 1 || port > 65535) return NaN
  return port
}

function parseEndpoint(text) {
  var raw = String(text || "").trim()
  if (raw === "") return { ok: false }

  var host = ""
  var portPart = ""
  if (raw.charAt(0) === "[") {
    var end = raw.indexOf("]")
    if (end < 0) return { ok: false }
    host = raw.slice(1, end)
    if (raw.length <= end + 1 || raw.charAt(end + 1) !== ":") return { ok: false }
    portPart = raw.slice(end + 2)
  } else {
    var colon = raw.lastIndexOf(":")
    if (colon < 0) return { ok: false }
    host = raw.slice(0, colon)
    portPart = raw.slice(colon + 1)
  }

  host = String(host || "").trim()
  var port = parsePort(portPart)
  if (host === "" || !isFinite(port)) return { ok: false }
  return { ok: true, host: host, port: port }
}

function formatEndpoint(host, port) {
  var h = String(host || "").trim()
  var p = parsePort(port)
  if (h === "" || !isFinite(p)) return ""
  if (h.indexOf(":") >= 0) return "[" + h + "]:" + p
  return h + ":" + p
}

function presetPatch(preset) {
  switch (String(preset || "")) {
  case "low-latency":
    return {
      preset: "low-latency",
      codec: "cast-opus",
      bitrate_kbps: 128,
      target_delay_ms: 20
    }
  case "balanced":
    return {
      preset: "balanced",
      codec: "cast-opus",
      bitrate_kbps: 192,
      target_delay_ms: 100
    }
  case "high-quality":
    return {
      preset: "high-quality",
      codec: "aac-hls",
      bitrate_kbps: 320,
      segment_ms: 500
    }
  default:
    return { preset: "custom" }
  }
}

function configFromForm(baseConfig, form) {
  var base = baseConfig && typeof baseConfig === "object" ? baseConfig : {}
  var out = {}
  for (var key in base) {
    if (Object.prototype.hasOwnProperty.call(base, key)) out[key] = base[key]
  }

  out.version = base.version !== undefined && base.version !== null ? base.version : 1
  out.media_port = base.media_port !== undefined && base.media_port !== null ? base.media_port : 17833

  out.enabled = form.enabled === true
  out.device_id = String(form.device_id || "")
  out.device_name = String(form.device_name || "")
  out.host = String(form.host || "")
  out.port = parsePort(form.port)
  if (!isFinite(out.port)) out.port = 8009

  var rv = Number(form.receiver_volume)
  if (!isFinite(rv)) rv = 0
  out.receiver_volume = Math.max(0, Math.min(1, rv))

  out.preset = String(form.preset || "custom")
  out.codec = String(form.codec || "cast-opus")
  out.segment_ms = parseInt(form.segment_ms, 10)
  if (!isFinite(out.segment_ms)) out.segment_ms = 500
  out.target_delay_ms = parseInt(form.target_delay_ms, 10)
  if (!isFinite(out.target_delay_ms)) out.target_delay_ms = 100
  out.bitrate_kbps = parseInt(form.bitrate_kbps, 10)
  if (!isFinite(out.bitrate_kbps)) out.bitrate_kbps = 192

  return out
}

function deviceOptionLabel(device) {
  var name = display(String(device.name || "").trim())
  var model = display(String(device.model || "").trim())
  if (name === "" && model === "") return display(String(device.id || "Device"))
  if (model === "") return name
  if (name === "") return model
  return name + " — " + model
}

function destinationOptions(devices, config, selectedId) {
  var options = []
  var seen = {}
  var list = Array.isArray(devices) ? devices : []
  var limit = Math.min(list.length, 64)

  for (var i = 0; i < limit; i++) {
    var d = list[i]
    if (!d || typeof d !== "object") continue
    var id = String(d.id || "")
    if (id === "" || seen[id]) continue
    seen[id] = true
    options.push({ value: id, label: deviceOptionLabel(d) })
  }

  var cfg = config && typeof config === "object" ? config : {}
  var savedId = String(selectedId || cfg.device_id || "")
  if (savedId !== "" && !seen[savedId]) {
    var savedName = display(String(cfg.device_name || savedId))
    options.unshift({ value: savedId, label: savedName + " (unavailable)" })
  }

  return options
}

function findDevice(devices, id) {
  var want = String(id || "")
  if (want === "") return null
  var list = Array.isArray(devices) ? devices : []
  for (var i = 0; i < list.length && i < 64; i++) {
    var d = list[i]
    if (d && String(d.id || "") === want) return d
  }
  return null
}

function statusLine(status) {
  var s = status && typeof status === "object" ? status : {}
  var state = display(String(s.state || "unknown"))
  var player = display(String(s.player_state || ""))
  var head = player !== "" ? state + " · " + player : state

  var desktopVol = Number(s.sink_volume_percent)
  if (!isFinite(desktopVol)) desktopVol = 0
  var desktopMuted = s.sink_muted === true
  var desktopPart = "Desktop: " + Math.round(desktopVol) + "%, "
    + (desktopMuted ? "muted" : "unmuted")

  var recvVol = Number(s.receiver_volume)
  if (!isFinite(recvVol)) recvVol = 0
  var recvPart = "Receiver: " + (recvVol * 100).toFixed(1) + "%"

  var message = display(String(s.message || ""))
  var lines = head + "\n" + desktopPart + " · " + recvPart + "\n" + message

  var frames = Number(s.audio_frames)
  if (isFinite(frames) && frames > 0) {
    var delay = parseInt(s.receiver_delay_ms, 10)
    if (!isFinite(delay)) delay = 0
    var retrans = Number(s.retransmits)
    if (!isFinite(retrans)) retrans = 0
    lines += "\nReceiver-reported buffer: " + delay + " ms · Retransmitted packets: "
      + Math.round(retrans)
  }

  return lines
}

function offlineStatusMessage() {
  return "Omashoutout is not running. Run omashoutout install or systemctl --user start omashoutout."
}

function encodingOptions(showMp3) {
  var options = [
    { value: "cast-opus", label: "Cast Streaming / Opus (experimental)" },
    { value: "aac-hls", label: "AAC live segments" }
  ]
  if (showMp3) options.push({ value: "mp3", label: "MP3 (legacy configuration)" })
  return options
}

function presetOptions() {
  return [
    { value: "low-latency", label: "Low latency" },
    { value: "balanced", label: "Balanced" },
    { value: "high-quality", label: "High quality" },
    { value: "custom", label: "Custom" }
  ]
}

function bitrateOptions() {
  return [
    { value: "128", label: "128 kbps" },
    { value: "192", label: "192 kbps" },
    { value: "256", label: "256 kbps" },
    { value: "320", label: "320 kbps" }
  ]
}

function clampInt(value, min, max, fallback) {
  var n = parseInt(value, 10)
  if (!isFinite(n)) n = fallback
  return Math.max(min, Math.min(max, n))
}

function formFromConfig(config) {
  var c = config && typeof config === "object" ? config : {}
  return {
    enabled: c.enabled === true,
    device_id: String(c.device_id || ""),
    device_name: String(c.device_name || ""),
    host: String(c.host || ""),
    port: parsePort(c.port) || 8009,
    endpoint: formatEndpoint(c.host, c.port),
    receiver_volume: (function() {
      var rv = Number(c.receiver_volume)
      return isFinite(rv) ? rv : 0.01
    })(),
    preset: String(c.preset || "balanced"),
    codec: String(c.codec || "cast-opus"),
    segment_ms: clampInt(c.segment_ms, 250, 2000, 500),
    target_delay_ms: clampInt(c.target_delay_ms, 10, 1000, 100),
    bitrate_kbps: clampInt(c.bitrate_kbps, 128, 320, 192)
  }
}

package ru.cryon.app

import android.content.Intent
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Test
import org.junit.runner.RunWith
import org.json.JSONArray
import org.json.JSONObject
import java.net.ServerSocket
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger

/** Real Media3 transitions with the activity absent and screen switched off. */
@RunWith(AndroidJUnit4::class)
class BackgroundPlaybackTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext

    private fun command(action: String, configure: Intent.() -> Unit = {}) {
        instrumentation.runOnMainSync { CryonPlaybackService.command(context, action, configure) }
    }

    private fun waitFor(timeout: Long = 15_000, predicate: () -> Boolean) {
        val end = System.currentTimeMillis() + timeout
        while (System.currentTimeMillis() < end) { if (predicate()) return; Thread.sleep(100) }
        check(predicate()) { "Timed out. Player: ${CryonPlaybackService.latestState}" }
    }

    private fun state(): JSONObject = JSONObject(CryonPlaybackService.latestState.ifBlank { "{}" })

    private fun start(fixture: Fixture, tracks: JSONArray, radio: Boolean) {
        // Android 15 requires an activity or established foreground service to
        // request initial audio focus. Subsequent transitions run without it.
        val launch = Intent(context, PlaybackTestActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        val activity = instrumentation.startActivitySync(launch)
        Thread.sleep(500)
        command(CryonPlaybackService.ACTION_STOP)
        command(CryonPlaybackService.ACTION_QUEUE) {
            putExtra(CryonPlaybackService.EXTRA_QUEUE, JSONObject().put("queue", tracks).put("currentIndex", 0)
                .put("radio", radio).put("serverBaseUrl", fixture.base).toString())
        }
        command(CryonPlaybackService.ACTION_PLAY) {
            putExtra(CryonPlaybackService.EXTRA_TRACK_ID, tracks.getJSONObject(0).getString("id"))
            putExtra(CryonPlaybackService.EXTRA_STREAM_URL, fixture.base + "/stream/first")
            putExtra(CryonPlaybackService.EXTRA_AUTO_PLAY, true)
        }
        waitFor { state().optBoolean("playing") }
        instrumentation.runOnMainSync { activity.finish() }
    }

    @Test fun radioKeepsAdvancingWithNoActivityAndScreenOff() {
        Fixture().use { fixture ->
            try {
                start(fixture, JSONArray().put(fixture.track(0)).put(fixture.track(1)), true)
                instrumentation.uiAutomation.executeShellCommand("input keyevent 223").close()
                val played = mutableSetOf<String>()
                val end = System.currentTimeMillis() + 16_000
                while (System.currentTimeMillis() < end) { played.add(state().optString("trackId")); Thread.sleep(250) }
                check(played.filter { it.isNotBlank() }.size >= 5) { "Radio stopped: $played; ${state()}" }
                check(fixture.relatedRequests.get() >= 2) { "Radio did not replenish repeatedly" }
                check(fixture.historyRequests.get() >= 5) { "Background tracks were not recorded in history" }
                check(state().optBoolean("playing")) { "Screen-off playback stopped" }
            } finally {
                instrumentation.uiAutomation.executeShellCommand("input keyevent 224").close()
                command(CryonPlaybackService.ACTION_STOP)
            }
        }
    }

    @Test fun lanNextAndPauseWorkWithoutWebView() {
        Fixture().use { fixture ->
            try {
                val queue = JSONArray().put(fixture.track(0)).put(fixture.track(1)).put(fixture.track(2))
                start(fixture, queue, false)
                command(CryonPlaybackService.ACTION_SYNC) {
                    putExtra(CryonPlaybackService.EXTRA_QUEUE, fixture.snapshot(queue, 0).put("baseUrl", fixture.base)
                        .put("deviceId", "phone").put("isHost", false).put("playbackOnThisDevice", true).toString())
                }
                fixture.remoteChange(queue, 2, true)
                waitFor { state().optString("trackId") == "youtube:test-2" && state().optBoolean("playing") }
                fixture.remoteChange(queue, 2, false)
                waitFor { !state().optBoolean("autoPlay") && !state().optBoolean("playing") }
            } finally { command(CryonPlaybackService.ACTION_STOP) }
        }
    }

    @Test fun pauseIsPreservedWhenNextStreamResolvesLate() {
        Fixture().use { fixture ->
            try {
                fixture.resolveDelayMs = 800
                start(fixture, JSONArray().put(fixture.track(0)).put(fixture.track(1)), false)
                command(CryonPlaybackService.ACTION_NEXT)
                command(CryonPlaybackService.ACTION_PAUSE)
                Thread.sleep(3000)
                check(!state().optBoolean("playing") && !state().optBoolean("autoPlay")) { "Late preload resumed paused playback: ${state()}" }
            } finally { command(CryonPlaybackService.ACTION_STOP) }
        }
    }

    @Test fun backgroundHandoffStopsAudioBeforeAcknowledgingRelease() {
        Fixture().use { fixture ->
            try {
                val queue = JSONArray().put(fixture.track(0)).put(fixture.track(1))
                start(fixture, queue, true)
                command(CryonPlaybackService.ACTION_SYNC) {
                    putExtra(CryonPlaybackService.EXTRA_QUEUE, fixture.snapshot(queue, 0).put("baseUrl", fixture.base)
                        .put("deviceId", "phone").put("isHost", false).put("playbackOnThisDevice", true).toString())
                }
                fixture.requestHandoff(queue)
                waitFor { fixture.releaseRequests.get() > 0 }
                check(!state().optBoolean("playing") && !state().optBoolean("playbackOnThisDevice")) { "Old device still playing: ${state()}" }
            } finally { command(CryonPlaybackService.ACTION_STOP) }
        }
    }

    private class Fixture : AutoCloseable {
        private val socket = ServerSocket(0)
        private val executor = Executors.newCachedThreadPool()
        val base = "http://127.0.0.1:${socket.localPort}"
        val relatedRequests = AtomicInteger()
        val releaseRequests = AtomicInteger()
        val historyRequests = AtomicInteger()
        @Volatile var resolveDelayMs = 0L
        private val nextTrack = AtomicInteger(2)
        private var shared = JSONObject()
        private val wav = ByteBuffer.allocate(32_044).order(ByteOrder.LITTLE_ENDIAN).apply {
            put("RIFF".toByteArray()); putInt(32_036); put("WAVEfmt ".toByteArray()); putInt(16)
            putShort(1); putShort(1); putInt(8000); putInt(16000); putShort(2); putShort(16)
            put("data".toByteArray()); putInt(32_000); repeat(16_000) { putShort(0) }
        }.array()
        init {
            executor.execute {
                while (!socket.isClosed) {
                    val peer = runCatching { socket.accept() }.getOrNull() ?: break
                    executor.execute {
                        peer.use {
                            val input = peer.getInputStream().buffered()
                            fun line(): String { val bytes = mutableListOf<Byte>(); while (true) { val b = input.read(); if (b < 0 || b == 10) break; if (b != 13) bytes.add(b.toByte()) }; return bytes.toByteArray().toString(Charsets.UTF_8) }
                            val request = line().split(' ')
                            val headers = mutableMapOf<String, String>()
                            while (true) { val header = line(); if (header.isBlank()) break; headers[header.substringBefore(':').lowercase()] = header.substringAfter(':').trim() }
                            val body = ByteArray(headers["content-length"]?.toIntOrNull() ?: 0)
                            var offset = 0
                            while (offset < body.size) { val n = input.read(body, offset, body.size - offset); if (n < 0) break; offset += n }
                            val json = if (body.isEmpty()) JSONObject() else JSONObject(body.toString(Charsets.UTF_8))
                            val path = request.getOrElse(1) { "" }
                            val response: ByteArray = when {
                                path == "/v1/library/history" -> { historyRequests.incrementAndGet(); "{}".toByteArray() }
                                path.startsWith("/stream/") -> wav
                                path == "/v1/playback/resolve" -> {
                                    Thread.sleep(resolveDelayMs)
                                    JSONObject().put("track", json.getJSONObject("track")).put("streamUrl", "/stream/" + json.getJSONObject("track").getString("id")).toString().toByteArray()
                                }
                                path == "/v1/recommendations/related" -> {
                                    relatedRequests.incrementAndGet()
                                    JSONArray().apply { repeat(4) { val track = track(nextTrack.getAndIncrement()); put(JSONObject().put("id", track.getString("id").substringAfter(':')).put("service", "youtube").put("title", track.getString("title")).put("artists", JSONArray().put("Artist")).put("durationMs", 2000)) } }.toString().toByteArray()
                                }
                                path == "/v1/devices/state" -> synchronized(this) {
                                    if (request[0] == "PUT") shared = JSONObject(json.toString()).put("revision", shared.optLong("revision") + 1)
                                    if (request[0] == "POST") { releaseRequests.incrementAndGet(); shared.put("activePlaybackDeviceId", shared.optString("playbackDeviceId")).remove("releaseDeviceId"); shared.put("revision", shared.optLong("revision") + 1) }
                                    shared.toString().toByteArray()
                                }
                                else -> "{}".toByteArray()
                            }
                            peer.getOutputStream().apply {
                                write("HTTP/1.1 200 OK\r\nContent-Length: ${response.size}\r\nContent-Type: ${if (path.startsWith("/stream/")) "audio/wav" else "application/json"}\r\nConnection: close\r\n\r\n".toByteArray())
                                write(response); flush()
                            }
                        }
                    }
                }
            }
        }
        fun track(id: Int) = JSONObject().put("id", "youtube:test-$id").put("source", "youtube").put("title", "Track $id").put("artist", "Artist").put("duration", 2).put("accent", "violet")
        fun snapshot(queue: JSONArray, index: Int) = JSONObject().put("queue", queue).put("currentIndex", index).put("playbackDeviceId", "phone").put("isPlaying", true).put("progress", 0).put("volume", 50)
        fun remoteChange(queue: JSONArray, index: Int, playing: Boolean) = synchronized(this) {
            shared = snapshot(queue, index).put("deviceId", "desktop").put("revision", shared.optLong("revision") + 1).put("isPlaying", playing)
        }
        fun requestHandoff(queue: JSONArray) = synchronized(this) {
            shared = snapshot(queue, 0).put("deviceId", "desktop").put("revision", shared.optLong("revision") + 1)
                .put("playbackDeviceId", "host").put("activePlaybackDeviceId", "").put("releaseDeviceId", "phone")
        }
        override fun close() { socket.close(); executor.shutdownNow() }
    }
}

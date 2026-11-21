package ru.cryon.app

import android.app.Activity
import android.app.Instrumentation
import android.content.ContentValues
import android.content.Intent
import android.os.Build
import android.provider.MediaStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.json.JSONObject
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.net.ServerSocket
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger

@RunWith(AndroidJUnit4::class)
class AudioExportTest {
    @Test fun savesCompleteAudioThroughSystemDocumentResult() {
        assumeTrue(Build.VERSION.SDK_INT >= 29)
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        val filename = "Cryon-export-test-${System.nanoTime()}.wav"
        val uri = checkNotNull(context.contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, filename)
            put(MediaStore.MediaColumns.MIME_TYPE, "audio/wav")
        }))
        // Substitute the user's chosen document, while exercising the real
        // ACTION_CREATE_DOCUMENT callback and Android ContentResolver copy.
        val pickerRequests = AtomicInteger()
        val monitor = object : Instrumentation.ActivityMonitor() {
            override fun onStartActivity(intent: Intent): Instrumentation.ActivityResult? {
                if (intent.action != Intent.ACTION_CREATE_DOCUMENT) return null
                check(intent.type == "audio/wav" && intent.getStringExtra(Intent.EXTRA_TITLE) == filename)
                pickerRequests.incrementAndGet()
                return Instrumentation.ActivityResult(Activity.RESULT_OK, Intent().setData(uri))
            }
        }
        instrumentation.addMonitor(monitor)
        val socket = ServerSocket(0)
        val worker = Executors.newSingleThreadExecutor()
        val downloads = AtomicInteger()
        val audio = ByteArray(48_044) { (it % 127).toByte() }
        worker.execute {
            while (!socket.isClosed) {
                val peer = runCatching { socket.accept() }.getOrNull() ?: break
                runCatching { peer.use {
                    val reader = peer.getInputStream().bufferedReader()
                    val request = reader.readLine().orEmpty()
                    while (!reader.readLine().isNullOrBlank()) { }
                    val metadata = request.contains("metadata=1")
                    val payload = if (metadata) JSONObject().put("name", filename).put("mimeType", "audio/wav").toString().toByteArray()
                        else audio.also { downloads.incrementAndGet() }
                    peer.getOutputStream().apply {
                        write("HTTP/1.1 200 OK\r\nContent-Length: ${payload.size}\r\nContent-Type: ${if (metadata) "application/json" else "audio/wav"}\r\nConnection: close\r\n\r\n".toByteArray())
                        write(payload); flush()
                    }
                } }
            }
        }
        val ticket = "export-test-${System.nanoTime()}"
        try {
            AudioExportActivity.results[ticket] = "{\"state\":\"loading\"}"
            instrumentation.runOnMainSync {
                context.startActivity(Intent(context, AudioExportActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    .putExtra("ticket", ticket).putExtra("url", "http://127.0.0.1:${socket.localPort}/v1/library/export?service=soundcloud&id=test"))
            }
            val deadline = System.currentTimeMillis() + 15_000
            while (System.currentTimeMillis() < deadline && JSONObject(AudioExportActivity.results[ticket]!!).optString("state") == "loading") Thread.sleep(100)
            check(JSONObject(AudioExportActivity.results[ticket]!!).optString("state") == "saved") { AudioExportActivity.results[ticket].orEmpty() }
            check(pickerRequests.get() == 1 && downloads.get() == 1) { "Document result did not trigger a full download" }
            val saved = context.contentResolver.openInputStream(uri)!!.use { it.readBytes() }
            check(saved.contentEquals(audio)) { "Exported file was truncated or altered" }
        } finally {
            instrumentation.removeMonitor(monitor)
            context.contentResolver.delete(uri, null, null)
            AudioExportActivity.results.remove(ticket)
            socket.close(); worker.shutdownNow()
        }
    }
}

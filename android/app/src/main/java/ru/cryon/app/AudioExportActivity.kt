package ru.cryon.app

import android.content.ContentValues
import android.content.Context
import android.media.MediaMetadataRetriever
import android.media.MediaScannerConnection
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors

/** Automatic Music/Cryon exports. Only recorded app-created files can be removed. */
object AudioDownloads {
    val results = ConcurrentHashMap<String, String>()
    private val worker = Executors.newSingleThreadExecutor()
    private const val PREFS = "cryon_audio_exports"
    fun start(context: Context, exportUrl: String, ticket: String) {
        val app = context.applicationContext
        worker.execute {
            var created: Uri? = null
            var legacy: File? = null
            try {
                val parsed = Uri.parse(exportUrl)
                val key = parsed.getQueryParameter("key").orEmpty()
                require(parsed.scheme == "http" && parsed.host == "127.0.0.1" && parsed.path == "/v1/library/downloads/file" && key.matches(Regex("[a-f0-9]{64}"))) { "Некорректное скачивание" }
                val metadataConnection = URL("$exportUrl&metadata=1").openConnection() as HttpURLConnection
                metadataConnection.connectTimeout = 15000
                metadataConnection.readTimeout = 30000
                val metadata = try { if (metadataConnection.responseCode != 200) error("Скачанный файл недоступен"); JSONObject(metadataConnection.inputStream.bufferedReader().use { it.readText() }) } finally { metadataConnection.disconnect() }
                val name = metadata.getString("name")
                val mime = metadata.getString("mimeType")
                val size = metadata.getLong("size")
                val old = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getString(key, null)
                if (old != null && runCatching { app.contentResolver.openInputStream(Uri.parse(old))?.use { true } }.getOrNull() == true) {
                    results[ticket] = JSONObject().put("state", "saved").put("name", name).toString()
                    return@execute
                }
                created = if (Build.VERSION.SDK_INT >= 29) {
                    val values = ContentValues().apply { put(MediaStore.Audio.Media.DISPLAY_NAME, name); put(MediaStore.Audio.Media.MIME_TYPE, mime); put(MediaStore.Audio.Media.RELATIVE_PATH, "Music/Cryon"); put(MediaStore.Audio.Media.IS_PENDING, 1) }
                    app.contentResolver.insert(MediaStore.Audio.Media.EXTERNAL_CONTENT_URI, values) ?: error("Не удалось создать аудиофайл")
                } else {
                    val dir = File(app.getExternalFilesDir(Environment.DIRECTORY_MUSIC) ?: app.filesDir, "Cryon").apply { mkdirs() }
                    legacy = File(dir, key + "." + name.substringAfterLast('.'))
                    Uri.fromFile(legacy)
                }
                val connection = URL(exportUrl).openConnection() as HttpURLConnection
                connection.connectTimeout = 15000; connection.readTimeout = 180000
                try {
                    if (connection.responseCode != 200) error("Не удалось получить аудиофайл")
                    val destination = if (legacy != null) legacy!!.outputStream() else app.contentResolver.openOutputStream(created!!, "w") ?: error("Не удалось открыть файл")
                    var received = 0L
                    destination.use { out -> connection.inputStream.use { source ->
                        val bytes = ByteArray(64 * 1024); var last = 0L
                        while (true) { val n = source.read(bytes); if (n < 0) break; out.write(bytes, 0, n); received += n
                            val now = System.currentTimeMillis(); if (now - last > 200) { last = now; results[ticket] = JSONObject().put("state", "loading").put("received", received).put("total", size).put("message", "Сохраняем в Music/Cryon").toString() }
                        }
                    } }
                    if (received != size) error("Файл передан не полностью; повторите скачивание")
                } finally { connection.disconnect() }
                val retriever = MediaMetadataRetriever()
                val expected = metadata.optJSONObject("track")?.optLong("durationMs") ?: 0L
                try { retriever.setDataSource(app, created!!); val actual = retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull() ?: 0L; if (expected > 30000 && actual > 0 && actual < expected - 15000) error("Плеер прочитал только часть файла; экспорт отменён") } finally { retriever.release() }
                if (Build.VERSION.SDK_INT >= 29) app.contentResolver.update(created!!, ContentValues().apply { put(MediaStore.Audio.Media.IS_PENDING, 0) }, null, null)
                if (legacy != null) MediaScannerConnection.scanFile(app, arrayOf(legacy!!.absolutePath), arrayOf(mime), null)
                app.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putString(key, created.toString()).apply()
                results[ticket] = JSONObject().put("state", "saved").put("name", name).toString()
            } catch (failure: Exception) {
                if (legacy != null) legacy?.delete() else created?.let { runCatching { app.contentResolver.delete(it, null, null) } }
                results[ticket] = JSONObject().put("state", "error").put("error", failure.message.orEmpty()).toString()
            }
        }
    }
    fun delete(context: Context, key: String): Boolean {
        if (!key.matches(Regex("[a-f0-9]{64}"))) return false
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val stored = prefs.getString(key, null) ?: return true
        val uri = Uri.parse(stored)
        val deleted = runCatching {
            if (uri.scheme == "content" && uri.authority == "media") context.contentResolver.delete(uri, null, null) >= 0
            else if (uri.scheme == "file") { val file = File(uri.path.orEmpty()); val root = File(context.getExternalFilesDir(Environment.DIRECTORY_MUSIC) ?: context.filesDir, "Cryon").canonicalFile; file.canonicalFile.parentFile == root && file.name.startsWith("$key.") && (!file.exists() || file.delete()) }
            else false
        }.getOrDefault(false)
        if (deleted) prefs.edit().remove(key).apply()
        return deleted
    }
}

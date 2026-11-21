package ru.cryon.app

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors

/** Uses the system file picker so exported audio is accessible to other players. */
class AudioExportActivity : Activity() {
    private var ticket = ""
    private var exportUrl = ""

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        ticket = intent.getStringExtra("ticket").orEmpty()
        exportUrl = intent.getStringExtra("url").orEmpty()
        val parsed = Uri.parse(exportUrl)
        if (parsed.scheme != "http" || parsed.host != "127.0.0.1" || parsed.path != "/v1/library/export") {
            complete("error", "Некорректная ссылка скачивания")
            return
        }
        worker.execute {
            try {
                val connection = URL("$exportUrl&metadata=1").openConnection() as HttpURLConnection
                connection.connectTimeout = 15000
                connection.readTimeout = 180000
                val metadata = try {
                    if (connection.responseCode != 200) throw IllegalStateException(connection.errorStream?.bufferedReader()?.use { JSONObject(it.readText()).optString("error") } ?: "Источник недоступен")
                    JSONObject(connection.inputStream.bufferedReader().use { it.readText() })
                } finally { connection.disconnect() }
                runOnUiThread {
                    try {
                        startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE)
                            .setType(metadata.getString("mimeType")).putExtra(Intent.EXTRA_TITLE, metadata.getString("name")), SAVE_AUDIO)
                    } catch (error: Exception) { complete("error", error.message.orEmpty()) }
                }
            } catch (error: Exception) { runOnUiThread { complete("error", error.message.orEmpty()) } }
        }
    }

    @Deprecated("System document picker callback")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != SAVE_AUDIO) return
        val uri = data?.data
        if (resultCode != RESULT_OK || uri == null) { complete("cancelled", ""); return }
        worker.execute {
            try {
                val connection = URL(exportUrl).openConnection() as HttpURLConnection
                connection.connectTimeout = 15000
                connection.readTimeout = 180000
                try {
                    if (connection.responseCode != 200) throw IllegalStateException("Не удалось скачать трек")
                    contentResolver.openOutputStream(uri, "w")?.use { destination -> connection.inputStream.use { source -> source.copyTo(destination) } }
                        ?: throw IllegalStateException("Не удалось открыть файл для записи")
                } finally { connection.disconnect() }
                runOnUiThread { complete("saved", "") }
            } catch (error: Exception) {
                runCatching { contentResolver.delete(uri, null, null) }
                runOnUiThread { complete("error", error.message.orEmpty()) }
            }
        }
    }

    private fun complete(state: String, error: String) {
        results[ticket] = JSONObject().put("state", state).put("error", error).toString()
        finish()
    }

    companion object {
        private const val SAVE_AUDIO = 5107
        val results = ConcurrentHashMap<String, String>()
        private val worker = Executors.newSingleThreadExecutor()
    }
}

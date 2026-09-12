package ru.cryon.app

import android.annotation.SuppressLint
import android.content.Intent
import android.os.Bundle
import android.view.ViewGroup
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.documentfile.provider.DocumentFile
import android.webkit.JavascriptInterface
import org.json.JSONObject
import java.io.File
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebView
import android.webkit.WebViewClient
import android.webkit.WebSettings
import mobile.Mobile

/**
 * Тонкая нативная оболочка: поднимает Go-бэкенд (встроенный HTTP-сервер на
 * 127.0.0.1) и открывает его в полноэкранном WebView. Вся логика приложения —
 * React-фронтенд + Go-бэкенд, те же, что на десктопе.
 */
class MainActivity : AppCompatActivity() {

    private lateinit var webView: WebView
    private val folderRequestCode = 1001
    private var lastBackPressedAt = 0L
    private var baseURL: String = ""

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // 1) Go-бэкенд: SQLite в filesDir, HTTP-сервер на случайном порту петли.
        //    Повторный вход (singleTask + поворот экрана) — сервер уже поднят.
        baseURL = try {
            Mobile.start(filesDir.absolutePath)
        } catch (e: Exception) {
            // Сервер уже запущен (рекреация активити) — URL не вернуть из Go,
            // поэтому перезапускаем процесс целиком: Stop + Start.
            Mobile.stop()
            Mobile.start(filesDir.absolutePath)
        }

        // 2) WebView строим отдельным методом — тот же код нужен при пересоздании
        //    после смерти рендер-процесса (onRenderProcessGone).
        webView = buildWebView()
        setContentView(webView)
        webView.loadUrl(baseURL)
    }

    // Создаёт и настраивает WebView. Вынесено из onCreate, потому что при смерти
    // рендер-процесса (обычно OOM на тяжёлой сетке обложек в поиске) старый
    // WebView нельзя переиспользовать — нужен свежий с идентичной настройкой.
    @SuppressLint("SetJavaScriptEnabled")
    private fun buildWebView(): WebView {
        val wv = WebView(this)
        wv.addJavascriptInterface(AndroidBridge(), "CryonAndroid")
        wv.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            mediaPlaybackRequiresUserGesture = false
            cacheMode = WebSettings.LOAD_DEFAULT
            allowFileAccess = false
            allowContentAccess = false
        }
        wv.webViewClient = object : WebViewClient() {
            // Рендер-процесс WebView умер. Если НЕ обработать (или вернуть false),
            // Android убивает всё приложение — именно это выглядело как «вылет на
            // вкладке поиск» при загрузке множества обложек. Возвращаем true и
            // пересоздаём WebView, приложение остаётся живым.
            override fun onRenderProcessGone(view: WebView?, detail: RenderProcessGoneDetail?): Boolean {
                recreateWebView()
                return true
            }
        }
        wv.setBackgroundColor(0xFF08090D.toInt())
        return wv
    }

    // Отсоединяет и уничтожает мёртвый WebView, поднимает свежий и перезагружает
    // UI с того же локального URL. Вызывается из onRenderProcessGone (UI-поток),
    // runOnUiThread — страховка на случай иного контекста вызова.
    private fun recreateWebView() {
        runOnUiThread {
            if (this::webView.isInitialized) {
                val dead = webView
                try { (dead.parent as? ViewGroup)?.removeView(dead) } catch (_: Exception) {}
                try { dead.destroy() } catch (_: Exception) {}
            }
            webView = buildWebView()
            setContentView(webView)
            webView.loadUrl(baseURL)
        }
    }

    private inner class AndroidBridge {
        @JavascriptInterface
        fun pickMusicFolder() { runOnUiThread { startActivityForResult(Intent(Intent.ACTION_OPEN_DOCUMENT_TREE), folderRequestCode) } }
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != folderRequestCode) return
        val uri = if (resultCode == RESULT_OK) data?.data else null
        if (uri == null) {
            resolveFolderPicker("")
            return
        }
        // Копирование аудио может быть долгим (много файлов) — на UI-потоке это
        // ANR и «кнопка не работает». Уносим в фон, результат отдаём во WebView
        // на UI-потоке.
        Thread {
            val path = try { importMusicFolder(uri) } catch (_: Exception) { "" }
            runOnUiThread { resolveFolderPicker(path) }
        }.start()
    }

    // Возвращает выбранный (или пустой) путь во фронтенд. Проверяем наличие
    // резолвера: client.ts ставит window.__cryonFolderPickerResolve только на
    // время ожидания, вызов несуществующей функции кинул бы исключение.
    private fun resolveFolderPicker(path: String) {
        if (this::webView.isInitialized) {
            webView.evaluateJavascript(
                "window.__cryonFolderPickerResolve && window.__cryonFolderPickerResolve(" + JSONObject.quote(path) + ")",
                null,
            )
        }
    }

    private fun importMusicFolder(uri: android.net.Uri): String {
        val root = DocumentFile.fromTreeUri(this, uri) ?: return ""
        val target = File(filesDir, "imported_music").apply { deleteRecursively(); mkdirs() }
        copyAudioFiles(root, target)
        return if (target.listFiles()?.isNotEmpty() == true) target.absolutePath else ""
    }

    private fun copyAudioFiles(directory: DocumentFile, target: File) {
        for (entry in directory.listFiles()) {
            if (entry.isDirectory) copyAudioFiles(entry, File(target, entry.name ?: "folder").apply { mkdirs() })
            else if (entry.isFile && entry.name?.substringAfterLast('.', "")?.lowercase() in setOf("mp3", "m4a", "flac", "ogg", "wav", "aac", "opus", "webm")) {
                try { contentResolver.openInputStream(entry.uri)?.use { input -> File(target, entry.name ?: "track").outputStream().use(input::copyTo) } } catch (_: Exception) {}
            }
        }
    }
    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (!this::webView.isInitialized) return super.onBackPressed()
        // \u041e\u0431\u043e\u0440\u0430\u0447\u0438\u0432\u0430\u0435\u043c \u0432 try/catch \u043d\u0430 \u0441\u0442\u043e\u0440\u043e\u043d\u0435 JS: \u0435\u0441\u043b\u0438 __cryonAndroidBack \u0435\u0449\u0451 \u043d\u0435
        // \u0443\u0441\u0442\u0430\u043d\u043e\u0432\u043b\u0435\u043d \u0438\u043b\u0438 \u0431\u0440\u043e\u0441\u0438\u043b \u2014 \u043f\u043e\u043b\u0443\u0447\u0430\u0435\u043c "false", \u0430 \u043d\u0435 "null", \u0438 \u043f\u0435\u0440\u0435\u0445\u043e\u0434\u0438\u043c \u043a
        // \u043d\u0430\u0442\u0438\u0432\u043d\u043e\u043c\u0443 \u0444\u043e\u043b\u0431\u044d\u043a\u0443, \u0430 \u043d\u0435 \u043a \u043d\u0435\u043c\u0435\u0434\u043b\u0435\u043d\u043d\u043e\u043c\u0443 \u0432\u044b\u0445\u043e\u0434\u0443.
        val probe = "(function(){try{return window.__cryonAndroidBack&&window.__cryonAndroidBack()===true}catch(e){return false}})()"
        webView.evaluateJavascript(probe) { result ->
            when {
                // JS-\u0441\u043b\u043e\u0439 \u043e\u0431\u0440\u0430\u0431\u043e\u0442\u0430\u043b: \u0437\u0430\u043a\u0440\u044b\u043b \u043e\u0432\u0435\u0440\u043b\u0435\u0439 \u0438\u043b\u0438 \u0441\u0434\u0435\u043b\u0430\u043b \u0448\u0430\u0433 \u043d\u0430\u0437\u0430\u0434 \u043f\u043e \u0440\u043e\u0443\u0442\u0435\u0440\u0443.
                result == "true" -> lastBackPressedAt = 0L
                // JS \u043d\u0435 \u043e\u0431\u0440\u0430\u0431\u043e\u0442\u0430\u043b, \u043d\u043e \u0443 WebView \u0435\u0441\u0442\u044c \u0438\u0441\u0442\u043e\u0440\u0438\u044f \u043d\u0430\u0432\u0438\u0433\u0430\u0446\u0438\u0438 \u2014 \u0438\u0434\u0451\u043c \u043d\u0430\u0437\u0430\u0434
                // \u043f\u043e \u043d\u0435\u0439, \u0430 \u043d\u0435 \u0432\u044b\u0445\u043e\u0434\u0438\u043c. \u0411\u0435\u0437 \u044d\u0442\u043e\u0433\u043e \u00ab\u043d\u0430\u0437\u0430\u0434\u00bb \u0437\u0430\u043a\u0440\u044b\u0432\u0430\u043b \u043f\u0440\u0438\u043b\u043e\u0436\u0435\u043d\u0438\u0435
                // \u0432\u043c\u0435\u0441\u0442\u043e \u0432\u043e\u0437\u0432\u0440\u0430\u0442\u0430 \u043d\u0430 \u043f\u0440\u0435\u0434\u044b\u0434\u0443\u0449\u0438\u0439 \u044d\u043a\u0440\u0430\u043d.
                webView.canGoBack() -> {
                    lastBackPressedAt = 0L
                    webView.goBack()
                }
                // \u041c\u044b \u0432 \u043a\u043e\u0440\u043d\u0435, \u0434\u0435\u0432\u0430\u0442\u044c\u0441\u044f \u043d\u0435\u043a\u0443\u0434\u0430 \u2014 \u0434\u0432\u043e\u0439\u043d\u043e\u0435 \u043d\u0430\u0436\u0430\u0442\u0438\u0435 \u0434\u043b\u044f \u0432\u044b\u0445\u043e\u0434\u0430.
                else -> {
                    val now = System.currentTimeMillis()
                    if (now - lastBackPressedAt < 2000L) {
                        finish()
                    } else {
                        lastBackPressedAt = now
                        Toast.makeText(this, "\u041d\u0430\u0436\u043c\u0438\u0442\u0435 \u0435\u0449\u0451 \u0440\u0430\u0437, \u0447\u0442\u043e\u0431\u044b \u0432\u044b\u0439\u0442\u0438", Toast.LENGTH_SHORT).show()
                    }
                }
            }
        }
    }
    override fun onDestroy() {
        // Останавливаем Go-сервер и освобождаем SQLite.
        Mobile.stop()
        if (this::webView.isInitialized) {
            webView.destroy()
        }
        super.onDestroy()
    }
}

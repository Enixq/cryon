package ru.cryon.app

import android.annotation.SuppressLint
import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
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

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // 1) Go-бэкенд: SQLite в filesDir, HTTP-сервер на случайном порту петли.
        //    Повторный вход (singleTask + поворот экрана) — сервер уже поднят.
        val baseURL = try {
            Mobile.start(filesDir.absolutePath)
        } catch (e: Exception) {
            // Сервер уже запущен (рекреация активити) — URL не вернуть из Go,
            // поэтому перезапускаем процесс целиком: Stop + Start.
            Mobile.stop()
            Mobile.start(filesDir.absolutePath)
        }

        // 2) WebView: JS включён, автоплей медиа разрешён, DOM-хранилище для
        //    настроек фронтенда.
        webView = WebView(this)
        webView.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            mediaPlaybackRequiresUserGesture = false
            cacheMode = WebSettings.LOAD_DEFAULT
            allowFileAccess = false
            allowContentAccess = false
        }
        webView.webViewClient = WebViewClient()
        webView.setBackgroundColor(0xFF08090D.toInt())

        setContentView(webView)
        webView.loadUrl(baseURL)
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        // Кнопка «Назад»: сначала история WebView (SPA-роутинг), затем выход.
        if (this::webView.isInitialized && webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
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

package ru.cryon.app

import android.annotation.SuppressLint
import android.graphics.Color
import android.graphics.Bitmap
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.ViewGroup
import android.view.View
import android.webkit.CookieManager
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.LinearLayout
import android.widget.Button
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

/** The remote website never receives Cryon's JavaScript bridge. */
class VKVerificationActivity : AppCompatActivity() {
    private lateinit var loginView: WebView
    private lateinit var status: TextView
    private lateinit var errorText: TextView
    private lateinit var retry: Button
    private val handler = Handler(Looper.getMainLooper())
    private var submitted = false
    private var completed = false
    private var base = ""
    private var attempt = ""
    private var browsing = false
    private var pageReady = false
    private val poll = object : Runnable {
        override fun run() {
            if (isFinishing || isDestroyed || submitted) return
            if (!pageReady) { handler.postDelayed(this, 1000); return }
            // Old cookies can exist before the interactive login has finished.
            // Wait for the actual desktop audio page to recognise the user.
            loginView.evaluateJavascript("""
                (() => {
                    if (location.hostname !== 'vk.ru' || !location.pathname.startsWith('/audio')) return false;
                    const match = document.documentElement.innerHTML.match(/(?:"uid"\s*:\s*|(?:window\.)?vk\s*=\s*\{\s*"id"\s*:\s*)(\d+)/);
                    const id = Number((window.vk && window.vk.id) || (match && match[1]));
                    return Number.isSafeInteger(id) && id > 0;
                })()
            """.trimIndent()) { result ->
                if (isFinishing || isDestroyed || submitted) return@evaluateJavascript
                val cookies = collectCookies()
                val hasSession = cookies.split(';').any {
                    val pair = it.trim().split('=', limit = 2)
                    pair.size == 2 && pair[0] in listOf("remixsid", "remixnsid") && pair[1].isNotEmpty()
                }
                if (result == "true" && hasSession) submit(cookies)
                else handler.postDelayed(this, 1000)
            }
        }
    }

    private fun collectCookies(): String {
        val values = linkedMapOf<String, String>()
        for (url in listOf("https://vk.com/", "https://m.vk.com/", "https://login.vk.com/", "https://m.vk.ru/", "https://login.vk.ru/", "https://vk.ru/")) {
            CookieManager.getInstance().getCookie(url).orEmpty().split(';').forEach {
                val pair = it.trim().split('=', limit = 2)
                if (pair.size == 2 && pair[1].isNotEmpty()) values[pair[0]] = pair[1]
            }
        }
        return values.entries.joinToString("; ") { "${it.key}=${it.value}" }
    }

    private fun desktopUserAgent(original: String): String {
        val version = Regex("Chrome/([0-9.]+)").find(original)?.groupValues?.get(1) ?: "131.0.0.0"
        return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/$version Safari/537.36"
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        base = intent.getStringExtra("base").orEmpty()
        attempt = intent.getStringExtra("attempt").orEmpty()
        browsing = intent.getBooleanExtra("browse", false)
        val uri = Uri.parse(base)
        if (uri.scheme != "http" || uri.host != "127.0.0.1" || (!browsing && !attempt.matches(Regex("[a-f0-9]{32}")))) { finish(); return }
        status = TextView(this).apply {
            text = if (browsing) "‹    Сайт ВК" else "‹    Вход на сайте ВК"
            textSize = 18f; setTextColor(Color.WHITE); gravity = Gravity.CENTER_VERTICAL
            setPadding(24, 0, 24, 0); setBackgroundColor(Color.rgb(25, 25, 30))
            contentDescription = "Закрыть вход ВК"; setOnClickListener { finish() }
        }
        loginView = WebView(this).apply {
            settings.javaScriptEnabled = true; settings.domStorageEnabled = true
            settings.userAgentString = desktopUserAgent(settings.userAgentString)
            settings.useWideViewPort = true; settings.loadWithOverviewMode = true
            webChromeClient = WebChromeClient()
            webViewClient = object : WebViewClient() {
                override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
                    pageReady = false
                }
                override fun onPageFinished(view: WebView?, url: String?) {
                    pageReady = true
                }
                override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
                    val target = request?.url ?: return true
                    val host = target.host.orEmpty()
                    return target.scheme != "https" || !(host == "vk.ru" || host.endsWith(".vk.ru") || host == "vk.com" || host.endsWith(".vk.com"))
                }
            }
        }
        CookieManager.getInstance().setAcceptCookie(true)
        CookieManager.getInstance().setAcceptThirdPartyCookies(loginView, true)
        errorText = TextView(this).apply {
            visibility = View.GONE; setTextColor(Color.WHITE); setPadding(24, 12, 24, 12)
        }
        retry = Button(this).apply {
            text = "Проверить вход ещё раз"; visibility = View.GONE
            setOnClickListener { restartValidation() }
        }
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            addView(status, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, (56 * resources.displayMetrics.density).toInt()))
            addView(loginView, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))
            addView(errorText, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
            addView(retry, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        }
        setContentView(content)
        val begin = {
            if (!isFinishing && !isDestroyed) {
                loginView.loadUrl("https://vk.ru/audio")
                if (!browsing) handler.postDelayed(poll, 1000)
            }
        }
        if (intent.getBooleanExtra("fresh", false)) clearVKCookies(begin) else begin()
    }

    private fun clearVKCookies(onComplete: () -> Unit) {
        val manager = CookieManager.getInstance()
        val removals = mutableListOf<Pair<String, String>>()
        for (root in listOf("vk.com", "vk.ru")) {
            for (host in listOf(root, "login.$root", "id.$root", "m.$root")) {
                val url = "https://$host/"
                manager.getCookie(url).orEmpty().split(';').forEach {
                    val name = it.substringBefore('=').trim()
                    if (name.isNotEmpty()) {
                        for (domain in listOf("", "; Domain=$host", "; Domain=.$root")) {
                            removals.add(url to "$name=; Max-Age=0; Path=/; Secure$domain")
                        }
                    }
                }
            }
        }
        if (removals.isEmpty()) { onComplete(); return }
        var pending = removals.size
        removals.forEach { (url, value) ->
            manager.setCookie(url, value) {
                pending--
                if (pending == 0) { manager.flush(); onComplete() }
            }
        }
    }

    private fun submit(cookies: String) {
        submitted = true
        CookieManager.getInstance().flush()
        status.text = "Проверяем доступ к музыке ВК…"
        val payload = JSONObject().put("id", attempt).put("cookies", cookies).put("userAgent", loginView.settings.userAgentString).toString()
        Thread {
            var success = false
            var message = "Не удалось связаться с Cryon. Проверьте сеть и повторите проверку входа."
            var connection: HttpURLConnection? = null
            try {
                connection = URL("$base/v1/accounts/vk/session").openConnection() as HttpURLConnection
                connection.connectTimeout = 5000; connection.readTimeout = 45000
                connection.requestMethod = "POST"; connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                connection.outputStream.use { it.write(payload.toByteArray(Charsets.UTF_8)) }
                success = connection.responseCode == 200
                if (!success) {
                    val body = connection.errorStream?.bufferedReader()?.use { it.readText().take(16384) }.orEmpty()
                    message = runCatching { JSONObject(body).optString("error", message) }.getOrDefault(message)
                }
            } catch (_: Exception) { } finally { connection?.disconnect() }
            runOnUiThread {
                if (isFinishing || isDestroyed) return@runOnUiThread
                completed = success
                if (success) finish() else {
                    status.text = "‹    Вход на сайте ВК"
                    errorText.text = message; errorText.visibility = View.VISIBLE
                    retry.visibility = View.VISIBLE
                }
            }
        }.start()
    }

    private fun restartValidation() {
        retry.isEnabled = false
        status.text = "Повторяем проверку входа…"
        Thread {
            var nextAttempt = ""
            var connection: HttpURLConnection? = null
            try {
                connection = URL("$base/v1/accounts/vk/login").openConnection() as HttpURLConnection
                connection.connectTimeout = 5000; connection.readTimeout = 10000
                connection.requestMethod = "POST"
                if (connection.responseCode == 202) {
                    nextAttempt = connection.inputStream.bufferedReader().use { JSONObject(it.readText()).optString("id") }
                }
            } catch (_: Exception) { } finally { connection?.disconnect() }
            runOnUiThread {
                if (isFinishing || isDestroyed) return@runOnUiThread
                retry.isEnabled = true
                status.text = "‹    Вход на сайте ВК"
                if (nextAttempt.matches(Regex("[a-f0-9]{32}"))) {
                    attempt = nextAttempt; submitted = false
                    errorText.visibility = View.GONE; retry.visibility = View.GONE
                    handler.post(poll)
                } else Toast.makeText(this, "Не удалось начать проверку. Повторите позже.", Toast.LENGTH_LONG).show()
            }
        }.start()
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        if (!completed && attempt.isNotEmpty() && base.isNotEmpty()) Thread {
            try {
                val connection = URL("$base/v1/accounts/vk/login?id=$attempt").openConnection() as HttpURLConnection
                connection.connectTimeout = 3000; connection.readTimeout = 3000; connection.requestMethod = "DELETE"
                connection.responseCode; connection.disconnect()
            } catch (_: Exception) { }
        }.start()
        if (::loginView.isInitialized) { (loginView.parent as? ViewGroup)?.removeView(loginView); loginView.destroy() }
        super.onDestroy()
    }
}

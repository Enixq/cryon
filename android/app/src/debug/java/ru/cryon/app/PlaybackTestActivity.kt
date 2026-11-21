package ru.cryon.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Bundle
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.core.content.ContextCompat
import androidx.webkit.WebViewAssetLoader
import org.json.JSONObject

/** Runs the production React engine and bridge without the ARM-only Go binary. */
class PlaybackTestActivity : android.app.Activity() {
    var webView: WebView? = null
    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            val payload = intent?.getStringExtra(CryonPlaybackService.EXTRA_PAYLOAD) ?: return
            webView?.evaluateJavascript("window.dispatchEvent(new CustomEvent('cryon:native-playback', {detail: $payload}))", null)
        }
    }
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val base = intent.getStringExtra("testBase") ?: return
        val queue = intent.getStringExtra("testQueue") ?: "[]"
        val loader = WebViewAssetLoader.Builder().addPathHandler("/assets/", WebViewAssetLoader.AssetsPathHandler(this)).build()
        webView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
            webChromeClient = object : android.webkit.WebChromeClient() {
                override fun onConsoleMessage(message: android.webkit.ConsoleMessage): Boolean {
                    android.util.Log.i("CryonForegroundTest", "${message.message()}: ${message.lineNumber()}")
                    return true
                }
            }
            addJavascriptInterface(NativePlaybackBridge(this@PlaybackTestActivity), "CryonAndroid")
            webViewClient = object : WebViewClient() {
                override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                    if (request.url.path?.endsWith("/index.html") == true) {
                        val bootstrap = "window.__CRYON_BASE__=${JSONObject.quote(base)};window.__CRYON_TEST_QUEUE__=$queue;localStorage.clear();localStorage.setItem('cryon.device.id','phone');"
                        val html = assets.open("foreground-test/index.html").bufferedReader().use { it.readText() }
                            .replace("<script src=", "<script>$bootstrap</script><script src=")
                        return WebResourceResponse("text/html", "UTF-8", html.byteInputStream())
                    }
                    return loader.shouldInterceptRequest(request.url)
                }
            }
        }
        ContextCompat.registerReceiver(this, receiver, IntentFilter(CryonPlaybackService.ACTION_STATE), ContextCompat.RECEIVER_NOT_EXPORTED)
        setContentView(webView)
        webView?.loadUrl("https://appassets.androidplatform.net/assets/foreground-test/index.html")
    }
    override fun onResume() { super.onResume(); if (webView != null) CryonPlaybackService.uiForeground = true }
    override fun onPause() { if (webView != null) CryonPlaybackService.uiForeground = false; super.onPause() }
    override fun onDestroy() {
        if (webView != null) { unregisterReceiver(receiver); webView?.destroy(); webView = null }
        super.onDestroy()
    }
}

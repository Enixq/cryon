package ru.cryon.app

import android.annotation.SuppressLint
import android.graphics.Color
import android.os.Bundle
import android.view.Gravity
import android.view.ViewGroup
import android.view.WindowManager
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.OnBackPressedCallback
import androidx.appcompat.app.AppCompatActivity

/** A separate window keeps the Yandex page isolated from Cryon's JS bridge. */
class YandexVerificationActivity : AppCompatActivity() {
    private lateinit var loginView: WebView

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)

        val density = resources.displayMetrics.density
        val header = TextView(this).apply {
            text = "‹    Подтверждение Яндекс ID"
            textSize = 18f
            setTextColor(Color.WHITE)
            gravity = Gravity.CENTER_VERTICAL
            setPadding((16 * density).toInt(), 0, (16 * density).toInt(), 0)
            setBackgroundColor(Color.rgb(25, 25, 30))
            contentDescription = "Закрыть подтверждение Яндекс ID"
            setOnClickListener { finish() }
        }
        loginView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            isFocusable = true
            isFocusableInTouchMode = true
            webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean =
                    request?.url?.scheme != "https"
            }
            webChromeClient = WebChromeClient()
        }
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            addView(header, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, (56 * density).toInt()))
            addView(loginView, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))
        }
        setContentView(content)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (loginView.canGoBack()) loginView.goBack() else finish()
            }
        })
        if (savedInstanceState == null) loginView.loadUrl("https://ya.ru/device")
        else loginView.restoreState(savedInstanceState)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        loginView.saveState(outState)
        super.onSaveInstanceState(outState)
    }

    override fun onDestroy() {
        (loginView.parent as? ViewGroup)?.removeView(loginView)
        loginView.destroy()
        super.onDestroy()
    }
}

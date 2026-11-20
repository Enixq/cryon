package ru.cryon.app

import android.annotation.SuppressLint
import android.content.BroadcastReceiver
import android.content.Context
import android.content.ContentUris
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.Bundle
import android.app.Notification
import android.app.PendingIntent
import android.app.NotificationChannel
import android.app.NotificationManager
import android.provider.MediaStore
import android.provider.Settings
import android.view.ViewGroup
import android.widget.Toast
import android.content.pm.PackageManager
import android.window.OnBackInvokedDispatcher
import androidx.appcompat.app.AppCompatActivity
import androidx.browser.customtabs.CustomTabsIntent
import androidx.documentfile.provider.DocumentFile
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.media.app.NotificationCompat.MediaStyle
import android.support.v4.media.MediaMetadataCompat
import android.support.v4.media.session.MediaSessionCompat
import android.support.v4.media.session.PlaybackStateCompat
import androidx.media.session.MediaButtonReceiver
import android.webkit.JavascriptInterface
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebView
import android.webkit.WebViewClient
import android.webkit.WebSettings
import android.webkit.WebChromeClient
import android.webkit.ValueCallback
import android.net.Uri
import mobile.Mobile

/**
 * Тонкая нативная оболочка: поднимает Go-бэкенд (встроенный HTTP-сервер на
 * 127.0.0.1) и открывает его в полноэкранном WebView. Вся логика приложения —
 * React-фронтенд + Go-бэкенд, те же, что на десктопе.
 */
class MainActivity : AppCompatActivity() {

    private lateinit var webView: WebView
    private val folderRequestCode = 1001
    private val mediaPermissionRequestCode = 1002
    private val notificationPermissionRequestCode = 1003
    private val fileRequestCode = 1004
    private val reportRequestCode = 1005
    private var fileChooserCallback: ValueCallback<Array<Uri>>? = null
    private var pendingPlaybackReport: String? = null
    private var pendingUpdatePath: String? = null
    private val nowPlayingNotificationId = 77
    private val nowPlayingChannelId = "cryon_now_playing"
    private var lastBackPressedAt = 0L
    private var pendingNowPlaying: Array<Any?>? = null
    private lateinit var mediaSession: MediaSessionCompat
    private var baseURL: String = ""
    private var currentArtworkUrl: String = ""
    private var currentArtwork: Bitmap? = null
    private var artworkRequestId = 0L
    private val playbackStateReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action != CryonPlaybackService.ACTION_STATE) return
            dispatchNativePlaybackState(intent)
        }
    }
    private val remoteCommandReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action != CryonPlaybackService.ACTION_REMOTE_COMMAND) return
            val command = intent.getStringExtra(CryonPlaybackService.EXTRA_REMOTE_COMMAND).orEmpty()
            if (command == CryonPlaybackService.ACTION_NEXT) dispatchPlayerCommand("next")
            if (command == CryonPlaybackService.ACTION_PREVIOUS) dispatchPlayerCommand("previous")
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        pendingPlaybackReport = savedInstanceState?.getString("pendingPlaybackReport")
        pendingUpdatePath = savedInstanceState?.getString("pendingUpdatePath")
        ContextCompat.registerReceiver(
            this,
            playbackStateReceiver,
            IntentFilter(CryonPlaybackService.ACTION_STATE),
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        ContextCompat.registerReceiver(
            this,
            remoteCommandReceiver,
            IntentFilter(CryonPlaybackService.ACTION_REMOTE_COMMAND),
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        mediaSession = MediaSessionCompat(this, "Cryon").apply {
            setCallback(object : MediaSessionCompat.Callback() {
                override fun onPlay() = dispatchPlayerCommand("play")
                override fun onPause() = dispatchPlayerCommand("pause")
                override fun onSkipToNext() = dispatchPlayerCommand("next")
                override fun onSkipToPrevious() = dispatchPlayerCommand("previous")
                override fun onSeekTo(pos: Long) = dispatchPlayerCommand("seek:$pos")
            })
            // Media3 owns the active media session. Keep this legacy bridge
            // inactive while the WebView UI is being migrated.
            isActive = false
        }

        // 1) Go-бэкенд: SQLite в filesDir, HTTP-сервер на случайном порту петли.
        //    Повторный вход (singleTask + поворот экрана) — сервер уже поднят.
        baseURL = Mobile.start(filesDir.absolutePath)

        // 2) WebView строим отдельным методом — тот же код нужен при пересоздании
        //    после смерти рендер-процесса (onRenderProcessGone).
        webView = buildWebView()
        setContentView(webView)
        webView.loadUrl(baseURL)

        // 3) «Назад». На Android 13+ (API 33+) при включённом предиктивном жесте
        //    (enableOnBackInvokedCallback=true в манифесте) устаревший
        //    onBackPressed() системой НЕ вызывается — из-за этого «назад»
        //    закрывал приложение вместо шага по роутеру. Регистрируем
        //    OnBackInvokedCallback; на API < 33 остаётся onBackPressed().
        //    Оба ведут в общий handleBackGesture().
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            onBackInvokedDispatcher.registerOnBackInvokedCallback(
                OnBackInvokedDispatcher.PRIORITY_DEFAULT,
            ) { handleBackGesture() }
        }
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
            allowContentAccess = true
        }
        wv.webChromeClient = object : WebChromeClient() {
            override fun onShowFileChooser(view: WebView?, callback: ValueCallback<Array<Uri>>, params: FileChooserParams?): Boolean {
                fileChooserCallback?.onReceiveValue(null)
                fileChooserCallback = callback
                return try {
                    val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
                        addCategory(Intent.CATEGORY_OPENABLE)
                        type = "*/*"
                        putExtra(Intent.EXTRA_MIME_TYPES, arrayOf("application/zip", "application/json", "application/octet-stream"))
                    }
                    startActivityForResult(intent, fileRequestCode)
                    true
                } catch (_: Exception) {
                    fileChooserCallback = null
                    callback.onReceiveValue(null)
                    false
                }
            }
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
        fun openTokenBrowser(url: String) {
            runOnUiThread {
                try {
                    CustomTabsIntent.Builder().build().launchUrl(this@MainActivity, Uri.parse(url))
                } catch (_: Exception) {
                    startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                }
            }
        }

        @JavascriptInterface
        fun readClipboard(): String {
            val clipboard = getSystemService(CLIPBOARD_SERVICE) as? android.content.ClipboardManager
            return clipboard?.primaryClip?.getItemAt(0)?.coerceToText(this@MainActivity)?.toString().orEmpty()
        }

        @JavascriptInterface
        fun pickMusicFolder() { runOnUiThread { startActivityForResult(Intent(Intent.ACTION_OPEN_DOCUMENT_TREE), folderRequestCode) } }

        @JavascriptInterface
        fun installUpdate() {
            Thread {
                try {
                    val path = Mobile.prepareUpdate(filesDir.absolutePath)
                    runOnUiThread {
                        pendingUpdatePath = path
                        dispatchUpdateResult("")
                        openUpdateInstaller()
                    }
                } catch (error: Exception) {
                    runOnUiThread { dispatchUpdateResult(error.message ?: "Не удалось скачать обновление") }
                }
            }.start()
        }

        @JavascriptInterface
        fun savePlaybackReport(report: String) {
            runOnUiThread {
                try {
                    pendingPlaybackReport = report
                    val intent = Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                        addCategory(Intent.CATEGORY_OPENABLE)
                        type = "application/json"
                        putExtra(Intent.EXTRA_TITLE, "cryon-playback-report.json")
                    }
                    startActivityForResult(intent, reportRequestCode)
                } catch (_: Exception) {
                    pendingPlaybackReport = null
                    Toast.makeText(this@MainActivity, "Не удалось открыть сохранение отчёта", Toast.LENGTH_LONG).show()
                }
            }
        }

        @JavascriptInterface
        fun updateNowPlaying(title: String, artist: String, playing: Boolean, artworkUrl: String, duration: Double, position: Double, quality: String, liked: Boolean, radio: Boolean) {
            runOnUiThread { showNowPlaying(title, artist, playing, artworkUrl, duration, position, quality, liked, radio) }
        }

        @JavascriptInterface
        fun clearNowPlaying() {
            runOnUiThread {
                (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).cancel(nowPlayingNotificationId)
                mediaSession.setMetadata(null)
                mediaSession.isActive = false
            }
        }

        @JavascriptInterface
        fun nativePlay(streamUrl: String, trackId: String, title: String, artist: String, artworkUrl: String, durationMs: Long, autoPlay: Boolean, startPositionMs: Long, mimeType: String) {
            CryonPlaybackService.command(this@MainActivity, CryonPlaybackService.ACTION_PLAY) {
                putExtra(CryonPlaybackService.EXTRA_STREAM_URL, streamUrl)
                putExtra(CryonPlaybackService.EXTRA_TRACK_ID, trackId)
                putExtra(CryonPlaybackService.EXTRA_TITLE, title)
                putExtra(CryonPlaybackService.EXTRA_ARTIST, artist)
                putExtra(CryonPlaybackService.EXTRA_ARTWORK_URL, artworkUrl)
                putExtra(CryonPlaybackService.EXTRA_DURATION_MS, durationMs)
                putExtra(CryonPlaybackService.EXTRA_AUTO_PLAY, autoPlay)
                putExtra(CryonPlaybackService.EXTRA_POSITION_MS, startPositionMs.coerceAtLeast(0L))
                putExtra(CryonPlaybackService.EXTRA_MIME_TYPE, mimeType)
            }
        }

        @JavascriptInterface
        fun nativePause() { CryonPlaybackService.command(this@MainActivity, CryonPlaybackService.ACTION_PAUSE) }

        @JavascriptInterface
        fun nativeResume() { CryonPlaybackService.command(this@MainActivity, CryonPlaybackService.ACTION_RESUME) }

        @JavascriptInterface
        fun nativeStop() { CryonPlaybackService.command(this@MainActivity, CryonPlaybackService.ACTION_STOP) }

        @JavascriptInterface
        fun nativeSeek(positionMs: Long) {
            CryonPlaybackService.command(this@MainActivity, CryonPlaybackService.ACTION_SEEK) {
                putExtra(CryonPlaybackService.EXTRA_POSITION_MS, positionMs)
            }
        }

        @JavascriptInterface
        fun nativeSetVolume(volume: Int) {
            CryonPlaybackService.command(this@MainActivity, CryonPlaybackService.ACTION_VOLUME) {
                putExtra(CryonPlaybackService.EXTRA_VOLUME, volume)
            }
        }

        @JavascriptInterface
        fun scanDeviceMusic(): String {
            if (!hasMediaPermission()) {
                runOnUiThread {
                    requestPermissions(arrayOf(mediaPermission()), mediaPermissionRequestCode)
                    Toast.makeText(this@MainActivity, "\u0420\u0430\u0437\u0440\u0435\u0448\u0438\u0442\u0435 \u0434\u043e\u0441\u0442\u0443\u043f \u043a \u043c\u0443\u0437\u044b\u043a\u0435 \u0438 \u043d\u0430\u0436\u043c\u0438\u0442\u0435 \u0435\u0449\u0451 \u0440\u0430\u0437", Toast.LENGTH_LONG).show()
                }
                return ""
            }
            return scanMediaStore()
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == reportRequestCode) {
            val report = pendingPlaybackReport
            pendingPlaybackReport = null
            val uri = if (resultCode == RESULT_OK) data?.data else null
            if (uri != null && report != null) {
                val saved = try {
                    val output = contentResolver.openOutputStream(uri) ?: error("Cannot open report")
                    output.use { it.write(report.toByteArray(Charsets.UTF_8)) }
                    true
                } catch (_: Exception) {
                    false
                }
                Toast.makeText(this, if (saved) "Отчёт сохранён" else "Не удалось сохранить отчёт", Toast.LENGTH_LONG).show()
            }
            return
        }
        if (requestCode == fileRequestCode) {
            val uri = if (resultCode == RESULT_OK) data?.data?.takeIf { it.scheme == "content" } else null
            fileChooserCallback?.onReceiveValue(uri?.let { arrayOf(it) })
            fileChooserCallback = null
            return
        }
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

    private fun dispatchPlayerCommand(command: String) {
        runOnUiThread {
            if (this::webView.isInitialized) {
                webView.evaluateJavascript("window.__cryonNativeMediaCommand && window.__cryonNativeMediaCommand(" + JSONObject.quote(command) + ")", null)
            }
        }
    }

    private fun dispatchNativePlaybackState(intent: Intent) {
        if (!this::webView.isInitialized) return
        val payload = JSONObject()
            .put("trackId", intent.getStringExtra(CryonPlaybackService.EXTRA_TRACK_ID).orEmpty())
            .put("state", intent.getStringExtra(CryonPlaybackService.EXTRA_STATE).orEmpty())
            .put("playing", intent.getBooleanExtra(CryonPlaybackService.EXTRA_PLAYING, false))
            .put("positionMs", intent.getLongExtra(CryonPlaybackService.EXTRA_POSITION_MS, 0L))
            .put("durationMs", intent.getLongExtra(CryonPlaybackService.EXTRA_DURATION_MS, 0L))
            .put("error", intent.getStringExtra(CryonPlaybackService.EXTRA_ERROR).orEmpty())
        runOnUiThread {
            if (this::webView.isInitialized) {
                webView.evaluateJavascript("window.dispatchEvent(new CustomEvent('cryon:native-playback', { detail: $payload }))", null)
            }
        }
    }

    private fun commandPendingIntent(command: String): PendingIntent {
        val intent = Intent(this, MainActivity::class.java).setAction("ru.cryon.app.$command")
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or if (Build.VERSION.SDK_INT >= 23) PendingIntent.FLAG_IMMUTABLE else 0
        return PendingIntent.getActivity(this, command.hashCode(), intent, flags)
    }

    private fun loadArtwork(url: String): Bitmap? = try {
        if (url.isBlank()) null else (URL(url).openConnection() as HttpURLConnection).run {
            connectTimeout = 5000
            readTimeout = 5000
            doInput = true
            connect()
            inputStream.use { BitmapFactory.decodeStream(it) }
        }
    } catch (_: Exception) { null }

    private fun showNowPlaying(title: String, artist: String, playing: Boolean, artworkUrl: String = "", duration: Double = 0.0, position: Double = 0.0, quality: String = "", liked: Boolean = false, radio: Boolean = false, artwork: Bitmap? = null, artworkLoaded: Boolean = false) {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission("android.permission.POST_NOTIFICATIONS") != PackageManager.PERMISSION_GRANTED) {
            pendingNowPlaying = arrayOf(title, artist, playing, artworkUrl, duration, position, quality, liked, radio)
            requestPermissions(arrayOf("android.permission.POST_NOTIFICATIONS"), notificationPermissionRequestCode)
            return
        }

        val requestedArtworkUrl = artworkUrl.trim()
        if (artworkLoaded) {
            if (requestedArtworkUrl != currentArtworkUrl) return
            currentArtwork = artwork
        } else if (requestedArtworkUrl != currentArtworkUrl) {
            currentArtworkUrl = requestedArtworkUrl
            currentArtwork = null
            artworkRequestId += 1
            val requestId = artworkRequestId
            if (requestedArtworkUrl.isNotBlank()) {
                Thread {
                    val loaded = loadArtwork(requestedArtworkUrl)
                    runOnUiThread {
                        if (requestId == artworkRequestId && requestedArtworkUrl == currentArtworkUrl) {
                            showNowPlaying(title, artist, playing, requestedArtworkUrl, duration, position, quality, liked, radio, loaded, true)
                        }
                    }
                }.start()
                return
            }
        } else if (requestedArtworkUrl.isNotBlank() && currentArtwork == null) {
            return
        }

        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(NotificationChannel(nowPlayingChannelId, "Cryon playback", NotificationManager.IMPORTANCE_LOW))
        }
        val state = if (playing) PlaybackStateCompat.STATE_PLAYING else PlaybackStateCompat.STATE_PAUSED
        val progressMax = (duration * 1000).toInt().coerceAtLeast(0)
        val progressValue = (position * 1000).toInt().coerceIn(0, progressMax)
        mediaSession.isActive = true
        mediaSession.setMetadata(MediaMetadataCompat.Builder()
            .putString(MediaMetadataCompat.METADATA_KEY_TITLE, title.ifBlank { "Cryon" })
            .putString(MediaMetadataCompat.METADATA_KEY_ARTIST, artist)
            .putString(MediaMetadataCompat.METADATA_KEY_ALBUM, quality)
            .putLong(MediaMetadataCompat.METADATA_KEY_DURATION, progressMax.toLong())
            .apply { currentArtwork?.let { putBitmap(MediaMetadataCompat.METADATA_KEY_ALBUM_ART, it) } }
            .build())
        mediaSession.setPlaybackState(PlaybackStateCompat.Builder()
            .setActions(PlaybackStateCompat.ACTION_PLAY or PlaybackStateCompat.ACTION_PAUSE or PlaybackStateCompat.ACTION_SKIP_TO_NEXT or PlaybackStateCompat.ACTION_SKIP_TO_PREVIOUS or PlaybackStateCompat.ACTION_SEEK_TO)
            .setState(state, (position * 1000).toLong().coerceAtLeast(0L), 1f)
            .build())
        val notification = NotificationCompat.Builder(this, nowPlayingChannelId)
            .setSmallIcon(android.R.drawable.ic_media_play)
            .setContentTitle(title.ifBlank { "Cryon" })
            .setContentText(if (quality.isBlank()) artist else "$artist • $quality")
            .setCategory(NotificationCompat.CATEGORY_TRANSPORT)
            .setOngoing(playing)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setContentIntent(PendingIntent.getActivity(
                this,
                7001,
                Intent(this, MainActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
                },
                PendingIntent.FLAG_UPDATE_CURRENT or if (Build.VERSION.SDK_INT >= 23) PendingIntent.FLAG_IMMUTABLE else 0,
            ))
            .apply { currentArtwork?.let { setLargeIcon(it) } }
            .setProgress(progressMax, progressValue, false)
            .addAction(NotificationCompat.Action(android.R.drawable.ic_media_previous, "Previous", MediaButtonReceiver.buildMediaButtonPendingIntent(this, PlaybackStateCompat.ACTION_SKIP_TO_PREVIOUS)))
            .addAction(NotificationCompat.Action(if (playing) android.R.drawable.ic_media_pause else android.R.drawable.ic_media_play, if (playing) "Pause" else "Play", MediaButtonReceiver.buildMediaButtonPendingIntent(this, if (playing) PlaybackStateCompat.ACTION_PAUSE else PlaybackStateCompat.ACTION_PLAY)))
            .addAction(NotificationCompat.Action(android.R.drawable.ic_media_next, "Next", MediaButtonReceiver.buildMediaButtonPendingIntent(this, PlaybackStateCompat.ACTION_SKIP_TO_NEXT)))
            .addAction(NotificationCompat.Action(android.R.drawable.btn_star_big_on, if (liked) "Unlike" else "Favorite", commandPendingIntent("favorite")))
            .addAction(NotificationCompat.Action(android.R.drawable.ic_menu_compass, if (radio) "Radio off" else "Radio by track", commandPendingIntent("radio")))
            .setStyle(MediaStyle().setMediaSession(mediaSession.sessionToken).setShowActionsInCompactView(0, 1, 2))
            .build()
        manager.notify(nowPlayingNotificationId, notification)
    }

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        intent?.action?.removePrefix("ru.cryon.app.")?.let { command ->
            if (command == "favorite" || command == "radio") dispatchPlayerCommand(command)
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == notificationPermissionRequestCode && grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
            val pending = pendingNowPlaying ?: return
            pendingNowPlaying = null
            showNowPlaying(pending[0] as String, pending[1] as String, pending[2] as Boolean, pending[3] as String, pending[4] as Double, pending[5] as Double, pending[6] as String, pending[7] as Boolean, pending[8] as Boolean)
        }
    }

    private fun mediaPermission(): String = if (Build.VERSION.SDK_INT >= 33) "android.permission.READ_MEDIA_AUDIO" else "android.permission.READ_EXTERNAL_STORAGE"

    private fun hasMediaPermission(): Boolean = Build.VERSION.SDK_INT < 23 || checkSelfPermission(mediaPermission()) == PackageManager.PERMISSION_GRANTED

    private fun scanMediaStore(): String {
        val target = File(filesDir, "scanned_music").apply { mkdirs() }
        val projection = arrayOf(MediaStore.Audio.Media._ID, MediaStore.Audio.Media.DISPLAY_NAME, MediaStore.Audio.Media.MIME_TYPE, MediaStore.Audio.Media.DURATION)
        val selection = "${MediaStore.Audio.Media.IS_MUSIC} != 0"
        var copied = 0
        contentResolver.query(MediaStore.Audio.Media.EXTERNAL_CONTENT_URI, projection, selection, null, MediaStore.Audio.Media.TITLE + " COLLATE NOCASE ASC")?.use { cursor ->
            val idColumn = cursor.getColumnIndexOrThrow(MediaStore.Audio.Media._ID)
            val nameColumn = cursor.getColumnIndexOrThrow(MediaStore.Audio.Media.DISPLAY_NAME)
            val durationColumn = cursor.getColumnIndexOrThrow(MediaStore.Audio.Media.DURATION)
            while (cursor.moveToNext()) {
                val id = cursor.getLong(idColumn)
                val name = cursor.getString(nameColumn)?.takeIf { it.isNotBlank() } ?: "track-$id"
                val duration = cursor.getLong(durationColumn).coerceAtLeast(0L)
                val safeName = "${id}--${duration}--${name.replace(Regex("[^A-Za-z0-9._-]"), "_")}"
                target.listFiles()?.filter { it.name.startsWith("$id-") && it.name != safeName }?.forEach { it.delete() }
                val targetFile = File(target, safeName)
                if (targetFile.exists()) { copied++; continue }
                val uri = ContentUris.withAppendedId(MediaStore.Audio.Media.EXTERNAL_CONTENT_URI, id)
                try { contentResolver.openInputStream(uri)?.use { input -> targetFile.outputStream().use(input::copyTo) }; if (targetFile.length() > 0) copied++ } catch (_: Exception) { targetFile.delete() }
            }
        }
        if (copied == 0) { Toast.makeText(this, "No music files found", Toast.LENGTH_SHORT).show(); return "" }
        Toast.makeText(this, "Music files scanned: $copied", Toast.LENGTH_SHORT).show()
        return target.absolutePath
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
    // Единая обработка «Назад» (регистрируется в onCreate). Спрашиваем фронтенд
    // (__cryonAndroidBack — закрыть оверлей / шаг назад по роутеру), иначе идём
    // по истории WebView, иначе двойное нажатие для выхода. Вызывается из
    // OnBackInvokedCallback (API 33+) и из onBackPressed() (API < 33).
    private fun handleBackGesture() {
        if (!this::webView.isInitialized) { finish(); return }
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
    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        // На API 33+ «назад» приходит через OnBackInvokedCallback (onCreate), и
        // этот устаревший метод системой не вызывается. Оставлен для API < 33.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) return
        handleBackGesture()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        pendingPlaybackReport?.let { outState.putString("pendingPlaybackReport", it) }
        pendingUpdatePath?.let { outState.putString("pendingUpdatePath", it) }
        super.onSaveInstanceState(outState)
    }

    override fun onResume() {
        super.onResume()
        if (pendingUpdatePath != null && (Build.VERSION.SDK_INT < Build.VERSION_CODES.O || packageManager.canRequestPackageInstalls())) {
            openUpdateInstaller()
        }
    }

    private fun dispatchUpdateResult(error: String) {
        if (this::webView.isInitialized) {
            webView.evaluateJavascript("window.dispatchEvent(new CustomEvent('cryon:update-result', { detail: " + JSONObject.quote(error) + " }))", null)
        }
    }

    private fun openUpdateInstaller() {
        val path = pendingUpdatePath ?: return
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
                Toast.makeText(this, "Разрешите установку обновлений Cryon и вернитесь в приложение", Toast.LENGTH_LONG).show()
                startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:$packageName")))
                return
            }
            val uri = FileProvider.getUriForFile(this, "$packageName.updates", File(path))
            pendingUpdatePath = null
            startActivity(Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, "application/vnd.android.package-archive")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            })
        } catch (error: Exception) {
            pendingUpdatePath = null
            dispatchUpdateResult(error.message ?: "Не удалось открыть установку обновления")
            Toast.makeText(this, "Не удалось открыть установку обновления", Toast.LENGTH_LONG).show()
        }
    }

    override fun onDestroy() {
        fileChooserCallback?.onReceiveValue(null)
        fileChooserCallback = null
        unregisterReceiver(playbackStateReceiver)
        unregisterReceiver(remoteCommandReceiver)
        if (this::mediaSession.isInitialized) mediaSession.release()
        // Do not stop the loopback Go engine here: Media3 may still stream
        // through it after the Activity/WebView was recreated or backgrounded.
        // Android tears down the process together with the service when needed.
        if (this::webView.isInitialized) {
            webView.destroy()
        }
        super.onDestroy()
    }
}

package ru.cryon.app

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper
import android.os.Bundle
import androidx.core.content.ContextCompat
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.database.StandaloneDatabaseProvider
import androidx.media3.datasource.DefaultHttpDataSource
import androidx.media3.datasource.cache.CacheDataSource
import androidx.media3.datasource.cache.LeastRecentlyUsedCacheEvictor
import androidx.media3.datasource.cache.SimpleCache
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import androidx.media3.session.CommandButton
import androidx.media3.session.SessionCommand
import androidx.media3.session.SessionResult
import com.google.common.util.concurrent.Futures
import com.google.common.util.concurrent.ListenableFuture
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors
import org.json.JSONArray
import org.json.JSONObject

/**
 * Sole owner of Android audio playback. Media3 keeps playing when the WebView
 * renderer or activity is recreated; broadcasts are informational UI updates.
 *
 * The service also keeps the queue moving ON ITS OWN. WebView JavaScript is
 * frozen while the screen is off or the app is backgrounded, so a UI-driven
 * "track ended → play next" step silently stopped playback after one track.
 * The service therefore receives the upcoming track descriptors and resolves
 * the next stream itself over the loopback API, well before the current track
 * ends; the UI only mirrors what the player has already done.
 */
class CryonPlaybackService : MediaSessionService() {
    private var mediaSession: MediaSession? = null
    private var player: ExoPlayer? = null
    private var cache: SimpleCache? = null
    private var currentTrackId = ""
    private val queue = mutableListOf<JSONObject>()
    private var queueIndex = 0
    private var generation = 0L
    private var repeat = "off"
    private var shuffle = false
    private var radioEpoch = 0L
    private var radioLoading = false
    private var retryRadioAt = 0L
    private var radioAnchorOffset = 0
    private val recentIds = LinkedHashSet<String>()
    private var radioEnabled = false
    private var serverBaseUrl = ""
    private var keepPlaying = false
    private var preloadingNext = false
    private val worker = Executors.newSingleThreadExecutor()
    private val radioWorker = Executors.newSingleThreadExecutor()
    private val syncWorker = Executors.newSingleThreadExecutor()
    private var syncConfig: JSONObject? = null
    private var syncRevision = 0L
    private var syncInflight = false
    private var syncingLocally = false
    private var syncSeekNonce = 0L
    private var recovering = false
    private var historyRecordedId = ""
    private var waitingAfterError = false
    private var commandId = 0L
    private var tickerScheduled = false
    private val progressHandler = Handler(Looper.getMainLooper())
    private val progressTicker = object : Runnable {
        override fun run() {
            tickerScheduled = false
            val playbackPlayer = player ?: return
            publishState(stateFor(playbackPlayer))
            maybePreloadNext(playbackPlayer)
            maybeRefillRadio()
            syncDeviceState()
            scheduleTicker()
        }
    }
    private fun scheduleTicker() {
        if (!tickerScheduled && (keepPlaying || syncConfig != null)) {
            tickerScheduled = true
            progressHandler.postDelayed(progressTicker, PROGRESS_INTERVAL_MS)
        }
    }
    private val previousCommand = SessionCommand(ACTION_PREVIOUS, Bundle.EMPTY)
    private val nextCommand = SessionCommand(ACTION_NEXT, Bundle.EMPTY)
    private val likeCommand = SessionCommand(ACTION_LIKE, Bundle.EMPTY)
    private val dislikeCommand = SessionCommand(ACTION_DISLIKE, Bundle.EMPTY)
    private val rejectedTracks = mutableSetOf<String>()
    private val feedbackWorker = Executors.newSingleThreadExecutor()

    private var notificationLiked = false
    private fun notificationButtons(liked: Boolean): List<CommandButton> = listOf(
        CommandButton.Builder(CommandButton.ICON_PREVIOUS)
            .setSessionCommand(previousCommand)
            .setDisplayName("Предыдущий")
            .setSlots(CommandButton.SLOT_BACK)
            .build(),
        CommandButton.Builder(CommandButton.ICON_NEXT)
            .setSessionCommand(nextCommand)
            .setDisplayName("Следующий")
            .setSlots(CommandButton.SLOT_FORWARD)
            .build(),
        CommandButton.Builder(if (liked) CommandButton.ICON_HEART_FILLED else CommandButton.ICON_HEART_UNFILLED)
            .setSessionCommand(likeCommand).setDisplayName("Добавить в библиотеку")
            .setSlots(CommandButton.SLOT_BACK_SECONDARY, CommandButton.SLOT_OVERFLOW).build(),
        CommandButton.Builder(CommandButton.ICON_THUMB_DOWN_UNFILLED)
            .setSessionCommand(dislikeCommand).setDisplayName("Не рекомендовать и пропустить")
            .setSlots(CommandButton.SLOT_FORWARD_SECONDARY, CommandButton.SLOT_OVERFLOW).build(),
    )

    override fun onCreate() {
        super.onCreate()
        cache = SimpleCache(
            File(cacheDir, "audio-cache"),
            LeastRecentlyUsedCacheEvictor(CACHE_SIZE_BYTES),
            StandaloneDatabaseProvider(this),
        )
        val dataSourceFactory = CacheDataSource.Factory()
            .setCache(requireNotNull(cache))
            // The local server downloads and verifies the complete remote file
            // before responding. Media3's default read timeout is too short for
            // that first request, especially over a VPN.
            .setUpstreamDataSourceFactory(DefaultHttpDataSource.Factory().setReadTimeoutMs(120_000))
            .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR)
        player = ExoPlayer.Builder(this)
            .setMediaSourceFactory(DefaultMediaSourceFactory(dataSourceFactory))
            .setWakeMode(C.WAKE_MODE_NETWORK)
            .setAudioAttributes(androidx.media3.common.AudioAttributes.DEFAULT, true)
            .setHandleAudioBecomingNoisy(true)
            .build()
            .also { playbackPlayer ->
                playbackPlayer.repeatMode = Player.REPEAT_MODE_OFF
                playbackPlayer.addListener(object : Player.Listener {
                    override fun onPlaybackStateChanged(playbackState: Int) {
                        if (playbackState == Player.STATE_ENDED && !radioEnabled && repeat == "off" && queueIndex >= queue.lastIndex) keepPlaying = false
                        publishState(
                            when (playbackState) {
                                Player.STATE_BUFFERING -> STATE_BUFFERING
                                Player.STATE_READY -> STATE_READY
                                Player.STATE_ENDED -> STATE_ENDED
                                else -> STATE_IDLE
                            },
                        )
                        if (playbackState == Player.STATE_ENDED) resumeAfterLatePreload()
                    }

                    // Media3 advanced to the item the service queued itself. The UI
                    // mirrors this transition instead of driving it.
                    override fun onMediaItemTransition(mediaItem: MediaItem?, reason: Int) {
                        waitingAfterError = false
                        mediaItem?.mediaId?.takeIf { it.isNotEmpty() }?.let { trackId ->
                            currentTrackId = trackId
                            queue.indexOfFirst { it.optString("id") == trackId }.takeIf { it >= 0 }?.let { queueIndex = it }
                            recentIds.add(trackId)
                            while (recentIds.size > 500) recentIds.remove(recentIds.first())
                        }
                        // Keep the timeline short while retaining queue history.
                        if (playbackPlayer.currentMediaItemIndex > 0) playbackPlayer.removeMediaItems(0, playbackPlayer.currentMediaItemIndex)
                        publishState(stateFor(playbackPlayer))
                        if (playbackPlayer.isPlaying) recordCurrentHistory()
                        maybePreloadNext(playbackPlayer)
                        maybeRefillRadio()
                    }

                    override fun onIsPlayingChanged(isPlaying: Boolean) {
                        if (isPlaying) recordCurrentHistory()
                        publishState(stateFor(playbackPlayer))
                        scheduleTicker()
                    }

                    override fun onPlayWhenReadyChanged(playWhenReady: Boolean, reason: Int) {
                        keepPlaying = playWhenReady
                        publishState(stateFor(playbackPlayer))
                    }

                    override fun onPlayerError(error: PlaybackException) {
                        recoverCurrent(error)
                    }
                })
            }
        val sessionActivity = PendingIntent.getActivity(
            this,
            7100,
            Intent(this, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            },
            PendingIntent.FLAG_UPDATE_CURRENT or if (android.os.Build.VERSION.SDK_INT >= 23) PendingIntent.FLAG_IMMUTABLE else 0,
        )
        mediaSession = MediaSession.Builder(this, requireNotNull(player))
            .setSessionActivity(sessionActivity)
            .setMediaButtonPreferences(notificationButtons(false))
            .setCallback(object : MediaSession.Callback {
                override fun onConnect(session: MediaSession, controllerInfo: MediaSession.ControllerInfo): MediaSession.ConnectionResult {
                    val commands = MediaSession.ConnectionResult.DEFAULT_SESSION_AND_LIBRARY_COMMANDS
                        .buildUpon()
                        .add(previousCommand)
                        .add(nextCommand)
                        .add(likeCommand)
                        .add(dislikeCommand)
                        .build()
                    return MediaSession.ConnectionResult.accept(commands, MediaSession.ConnectionResult.DEFAULT_PLAYER_COMMANDS)
                }

                override fun onCustomCommand(
                    session: MediaSession,
                    controllerInfo: MediaSession.ControllerInfo,
                    customCommand: SessionCommand,
                    args: Bundle,
                ): ListenableFuture<SessionResult> {
                    if (customCommand.customAction == ACTION_LIKE) rateCurrent(1)
                    if (customCommand.customAction == ACTION_DISLIKE) rateCurrent(-1)
                    if (customCommand.customAction == ACTION_PREVIOUS || customCommand.customAction == ACTION_NEXT) {
                        if (customCommand.customAction == ACTION_NEXT) skipNext()
                        else if (session.player.currentPosition > 3000) session.player.seekTo(0)
                        else playQueueIndex((queueIndex - 1).coerceAtLeast(0))
                    }
                    return Futures.immediateFuture(SessionResult(SessionResult.RESULT_SUCCESS))
                }
            })
            .build()
        addSession(requireNotNull(mediaSession))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        val playbackPlayer = player ?: return START_NOT_STICKY
        commandId = maxOf(commandId, intent?.getLongExtra(EXTRA_COMMAND_ID, 0L) ?: 0L)
        when (intent?.action) {
            ACTION_PLAY -> {
                generation++
                preloadingNext = false
                recovering = false
                historyRecordedId = ""
                waitingAfterError = false
                keepPlaying = intent.getBooleanExtra(EXTRA_AUTO_PLAY, false)
                currentTrackId = intent.getStringExtra(EXTRA_TRACK_ID).orEmpty()
                val item = MediaItem.Builder()
                    .setUri(intent.getStringExtra(EXTRA_STREAM_URL).orEmpty())
                    .setMimeType(intent.getStringExtra(EXTRA_MIME_TYPE)?.takeIf { it.isNotBlank() })
                    .setMediaId(currentTrackId)
                    .setMediaMetadata(
                        MediaMetadata.Builder()
                            .setTitle(intent.getStringExtra(EXTRA_TITLE))
                            .setArtist(intent.getStringExtra(EXTRA_ARTIST))
                            .setDurationMs(intent.getLongExtra(EXTRA_DURATION_MS, 0).takeIf { it > 0 })
                            .setArtworkUri(intent.getStringExtra(EXTRA_ARTWORK_URL)?.takeIf { it.isNotBlank() }?.let(android.net.Uri::parse))
                            .build(),
                    )
                    .build()
                playbackPlayer.setMediaItem(
                    item,
                    intent.getLongExtra(EXTRA_POSITION_MS, 0L).coerceAtLeast(0L),
                )
                playbackPlayer.prepare()
                if (intent.getBooleanExtra(EXTRA_AUTO_PLAY, false)) playbackPlayer.play()
                else playbackPlayer.pause()
            }
            ACTION_QUEUE -> updateQueue(JSONObject(intent.getStringExtra(EXTRA_QUEUE).orEmpty()))
            ACTION_SYNC -> {
                val config = JSONObject(intent.getStringExtra(EXTRA_QUEUE).orEmpty())
                if (syncConfig?.optString("baseUrl") != config.optString("baseUrl")) syncRevision = 0
                syncRevision = maxOf(syncRevision, config.optLong("revision"))
                syncConfig = config
                syncingLocally = config.optBoolean("playbackOnThisDevice")
                syncSeekNonce = config.optLong("seekNonce")
            }
            ACTION_PAUSE -> { keepPlaying = false; playbackPlayer.pause(); publishState(stateFor(playbackPlayer)) }
            ACTION_RESUME -> { keepPlaying = true; playbackPlayer.play(); resumeAfterLatePreload() }
            ACTION_LIKE -> rateCurrent(1)
            ACTION_DISLIKE -> rateCurrent(-1)
            ACTION_NEXT -> skipNext()
            ACTION_PREVIOUS -> { if (playbackPlayer.currentPosition > 3000) playbackPlayer.seekTo(0) else playQueueIndex((queueIndex - 1).coerceAtLeast(0)) }
            ACTION_STOP -> {
                radioEpoch++
                generation++
                preloadingNext = false
                radioLoading = false
                recovering = false
                keepPlaying = false
                syncConfig = null
                syncingLocally = false
                syncRevision = 0
                waitingAfterError = false
                queue.clear()
                playbackPlayer.stop()
                playbackPlayer.clearMediaItems()
                currentTrackId = ""
                publishState(STATE_IDLE)
            }
            ACTION_SEEK -> playbackPlayer.seekTo(intent.getLongExtra(EXTRA_POSITION_MS, 0L).coerceAtLeast(0L))
            ACTION_VOLUME -> playbackPlayer.volume = intent.getIntExtra(EXTRA_VOLUME, 100).coerceIn(0, 100) / 100f
        }
        scheduleTicker()
        return START_NOT_STICKY
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession? = mediaSession

    override fun onTaskRemoved(rootIntent: Intent?) {
        onStartCommand(Intent(this, CryonPlaybackService::class.java).setAction(ACTION_STOP), 0, 0)
        progressHandler.removeCallbacks(progressTicker)
        tickerScheduled = false
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onDestroy() {
        mediaSession?.let { removeSession(it) }
        mediaSession?.release()
        mediaSession = null
        player?.release()
        player = null
        cache?.release()
        cache = null
        progressHandler.removeCallbacksAndMessages(null)
        generation++
        worker.shutdownNow()
        radioWorker.shutdownNow()
        syncWorker.shutdownNow()
        feedbackWorker.shutdownNow()
        latestState = ""
        super.onDestroy()
    }

    private fun stateFor(playbackPlayer: Player): String = when (playbackPlayer.playbackState) {
        Player.STATE_BUFFERING -> STATE_BUFFERING
        Player.STATE_READY -> STATE_READY
        Player.STATE_ENDED -> STATE_ENDED
        else -> STATE_IDLE
    }

    private fun publishState(state: String, error: String? = null) {
        val playbackPlayer = player ?: return
        val durationMs = playbackPlayer.duration.takeIf { it != C.TIME_UNSET }?.coerceAtLeast(0L) ?: 0L
        val payload = JSONObject().put("trackId", currentTrackId).put("state", state)
            .put("playing", playbackPlayer.isPlaying).put("autoPlay", keepPlaying).put("commandId", commandId)
            .put("positionMs", playbackPlayer.currentPosition.coerceAtLeast(0L))
            .put("durationMs", durationMs).put("error", error.orEmpty())
            .put("radio", radioEnabled).put("playbackOnThisDevice", syncConfig == null || syncingLocally)
        if (queue.getOrNull(queueIndex)?.optString("id") == currentTrackId) {
            payload.put("queue", JSONArray(queue)).put("currentIndex", queueIndex)
        }
        latestState = payload.toString()
        val liked = queue.getOrNull(queueIndex)?.optBoolean("liked") == true
        if (liked != notificationLiked) {
            notificationLiked = liked
            mediaSession?.setMediaButtonPreferences(notificationButtons(liked))
        }
        sendBroadcast(
            Intent(ACTION_STATE)
                .setPackage(packageName)
                .putExtra(EXTRA_PAYLOAD, latestState)
                .putExtra(EXTRA_TRACK_ID, currentTrackId)
                .putExtra(EXTRA_STATE, state)
                .putExtra(EXTRA_PLAYING, playbackPlayer.isPlaying)
                .putExtra(EXTRA_POSITION_MS, playbackPlayer.currentPosition.coerceAtLeast(0L))
                .putExtra(EXTRA_DURATION_MS, durationMs)
                .putExtra(EXTRA_ERROR, error.orEmpty()),
        )
    }

    private fun updateQueue(snapshot: JSONObject) {
        val incoming = snapshot.optJSONArray("queue") ?: return
        val nextQueue = (0 until incoming.length()).map { incoming.getJSONObject(it) }
        if (nextQueue.map { it.optString("id") } != queue.map { it.optString("id") }) {
            radioEpoch++
            generation++
            preloadingNext = false
            radioLoading = false
            queue.clear()
            queue.addAll(nextQueue)
        }
        queueIndex = snapshot.optInt("currentIndex").coerceIn(0, (queue.size - 1).coerceAtLeast(0))
        radioEnabled = snapshot.optBoolean("radio")
        repeat = snapshot.optString("repeat", "off")
        shuffle = snapshot.optBoolean("shuffle")
        serverBaseUrl = snapshot.optString("serverBaseUrl", serverBaseUrl).trimEnd('/')
        val p = player ?: return
        p.repeatMode = if (repeat == "one") Player.REPEAT_MODE_ONE else Player.REPEAT_MODE_OFF
        // Changes to the tail must not restart the current media item.
        val expected = queue.getOrNull(queueIndex + 1)?.optString("id")
        if (!shuffle && p.mediaItemCount > p.currentMediaItemIndex + 1 && p.getMediaItemAt(p.currentMediaItemIndex + 1).mediaId != expected) {
            p.removeMediaItems(p.currentMediaItemIndex + 1, p.mediaItemCount)
        }
        maybePreloadNext(p)
        maybeRefillRadio()
    }

    /** The service continues receiving LAN commands when WebView is suspended. */
    private fun syncDeviceState() {
        val config = syncConfig ?: return
        if (uiForeground || syncInflight || !syncingLocally) return
        val p = player ?: return
        val revision = syncRevision
        val localSnapshot = JSONObject(config.toString()).put("queue", JSONArray(queue)).put("currentIndex", queueIndex)
            .put("isPlaying", keepPlaying).put("progress", p.currentPosition / 1000.0)
            .put("radio", radioEnabled).put("repeat", repeat).put("shuffle", shuffle).put("seekNonce", syncSeekNonce)
        syncInflight = true
        syncWorker.execute {
            val result = runCatching {
                val base = config.getString("baseUrl")
                val remote = syncRequest(base, "GET", null)
                if (remote.optLong("revision") > revision) remote
                else if (localSnapshot.optJSONArray("queue")?.length() != 0) {
                    localSnapshot.put("baseRevision", revision)
                    val ack = syncRequest(base, "PUT", localSnapshot)
                    localSnapshot.put("revision", ack.optLong("revision"))
                } else remote
            }
            progressHandler.post {
                syncInflight = false
                if (uiForeground || syncConfig?.optString("baseUrl") != config.optString("baseUrl")) return@post
                val incoming = result.getOrNull() ?: return@post
                if (incoming.optLong("revision") <= syncRevision) return@post
                syncRevision = incoming.optLong("revision")
                if (incoming.optString("deviceId") == config.optString("deviceId")) return@post
                val owner = incoming.optString("playbackDeviceId", "host")
                val release = incoming.optString("releaseDeviceId")
                val active = incoming.optString("activePlaybackDeviceId", owner)
                val own = release.isBlank() && active == owner && (owner == config.optString("deviceId") || (owner == "host" && config.optBoolean("isHost")))
                val currentPlayer = player ?: return@post
                if (!own) {
                    syncingLocally = false
                    keepPlaying = false
                    generation++
                    currentPlayer.stop()
                    publishState(stateFor(currentPlayer))
                    if (release == config.optString("deviceId") || (release == "host" && config.optBoolean("isHost"))) {
                        syncWorker.execute { runCatching { syncRequest(config.getString("baseUrl"), "POST", JSONObject().put("releaseDeviceId", release).put("baseRevision", incoming.optLong("revision"))) } }
                    }
                    return@post
                }
                val incomingQueue = incoming.optJSONArray("queue") ?: return@post
                val index = incoming.optInt("currentIndex")
                val incomingId = incomingQueue.optJSONObject(index)?.optString("id") ?: return@post
                val requestedPlaying = incoming.optBoolean("isPlaying")
                updateQueue(JSONObject(incoming.toString()).put("serverBaseUrl", serverBaseUrl))
                syncConfig = JSONObject(config.toString()).put("playbackDeviceId", owner).put("volume", incoming.optInt("volume", 68)).put("muted", incoming.optBoolean("muted"))
                if (incomingId != currentTrackId) playQueueIndex(index, requestedPlaying, (incoming.optDouble("progress") * 1000).toLong())
                else {
                    if (incoming.optLong("seekNonce") != syncSeekNonce) currentPlayer.seekTo((incoming.optDouble("progress") * 1000).toLong())
                    keepPlaying = requestedPlaying
                    if (requestedPlaying) currentPlayer.play() else currentPlayer.pause()
                }
                syncSeekNonce = incoming.optLong("seekNonce")
                currentPlayer.volume = if (incoming.optBoolean("muted")) 0f else incoming.optInt("volume", 68) / 100f
                publishState(stateFor(currentPlayer))
            }
        }
    }

    private fun syncRequest(base: String, method: String, body: JSONObject?): JSONObject {
        val connection = URL(base + "/v1/devices/state").openConnection() as HttpURLConnection
        try {
            connection.connectTimeout = 3000
            connection.readTimeout = 3000
            connection.requestMethod = method
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            }
            if (connection.responseCode !in 200..299) throw java.io.IOException("Cryon sync HTTP ${connection.responseCode}")
            return JSONObject(connection.inputStream.bufferedReader().use { it.readText().ifBlank { "{}" } })
        } finally { connection.disconnect() }
    }

    private fun backendTrack(track: JSONObject): JSONObject {
        val id = track.optString("id")
        return JSONObject().put("id", id.substringAfter(':')).put("service", track.optString("source", id.substringBefore(':')))
            .put("title", track.optString("title")).put("artists", track.optJSONArray("artists") ?: JSONArray().put(track.optString("primaryArtist").ifBlank { track.optString("artist") }))
            .put("album", track.optString("album")).put("durationMs", track.optLong("duration") * 1000)
            .put("artworkUrl", track.optString("coverUrl")).put("playableKind", track.optString("playableKind", "stream"))
            .put("externalUrl", track.optString("externalUrl"))
    }

    private fun feedbackKey(track: JSONObject): String =
        (track.optString("primaryArtist").ifBlank { track.optString("artist") }.trim() + "|" + track.optString("title").trim()).lowercase(java.util.Locale.ROOT)

    private fun rateCurrent(score: Int) {
        val track = queue.getOrNull(queueIndex)?.takeIf { it.optString("id") == currentTrackId } ?: return
        val trackCopy = JSONObject(track.toString())
        if (score == -1) {
            rejectedTracks.add(feedbackKey(track))
            generation++
            preloadingNext = false
            val p = player ?: return
            if (p.mediaItemCount > p.currentMediaItemIndex + 1) p.removeMediaItems(p.currentMediaItemIndex + 1, p.mediaItemCount)
            for (index in queue.lastIndex downTo queueIndex + 1) {
                if (rejectedTracks.contains(feedbackKey(queue[index]))) queue.removeAt(index)
            }
            skipNext()
        }
        feedbackWorker.execute {
            val result = runCatching { request("/v1/library/feedback", JSONObject().put("track", backendTrack(trackCopy)).put("score", score)) }
            progressHandler.post {
                if (player == null) return@post
                if (result.isSuccess) {
                    queue.filter { it.optString("id") == trackCopy.optString("id") }.forEach { it.put("liked", score == 1) }
                    publishState(stateFor(requireNotNull(player)))
                } else {
                    android.widget.Toast.makeText(this, "Не удалось сохранить оценку. Повторите ещё раз.", android.widget.Toast.LENGTH_SHORT).show()
                }
            }
        }
    }

    private fun recordCurrentHistory() {
        if (historyRecordedId == currentTrackId) return
        val spec = queue.getOrNull(queueIndex)?.takeIf { it.optString("id") == currentTrackId } ?: return
        historyRecordedId = currentTrackId
        worker.execute { runCatching { request("/v1/library/history", JSONObject().put("track", backendTrack(spec))) } }
    }

    private fun frontendTrack(track: JSONObject): JSONObject {
        val artists = track.optJSONArray("artists") ?: JSONArray()
        return JSONObject().put("id", track.optString("service") + ":" + track.optString("id"))
            .put("source", track.optString("service")).put("title", track.optString("title"))
            .put("artists", artists).put("artist", (0 until artists.length()).joinToString(", ") { artists.optString(it) })
            .put("primaryArtist", artists.optString(0)).put("album", track.optString("album"))
            .put("duration", track.optLong("durationMs") / 1000).put("coverUrl", track.optString("artworkUrl"))
            .put("playableKind", track.optString("playableKind", "stream")).put("quality", track.optString("quality"))
            .put("accent", "violet")
    }

    private fun request(path: String, body: JSONObject): JSONObject {
        val connection = URL(serverBaseUrl + path).openConnection() as HttpURLConnection
        try {
            connection.connectTimeout = 15_000
            connection.readTimeout = 120_000
            connection.requestMethod = "POST"
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            if (connection.responseCode !in 200..299) throw java.io.IOException("Cryon HTTP ${connection.responseCode}")
            val text = connection.inputStream.bufferedReader().use { it.readText() }
            return if (text.trimStart().startsWith("[")) JSONObject().put("tracks", JSONArray(text)) else JSONObject(text)
        } finally { connection.disconnect() }
    }

    private fun resolveItem(track: JSONObject, fallbackOnly: Boolean = false): MediaItem {
        val input = backendTrack(track)
        val resolved = try {
            if (fallbackOnly) throw java.io.IOException("Retry with another provider")
            request("/v1/playback/resolve", JSONObject().put("track", input).put("native", true))
        }
        catch (error: Exception) {
            request("/v1/playback/fallback", JSONObject().put("track", input).put("failedService", input.optString("service")).put("excludedCandidates", JSONArray()))
        }
        val stream = resolved.getString("streamUrl")
        val uri = if (stream.startsWith("/")) serverBaseUrl + stream else stream
        // Warm the loopback proxy now: YouTube may download the complete file
        // before returning its first byte, which must happen before the transition.
        if (stream.startsWith("/stream/") || uri.startsWith(serverBaseUrl + "/stream/")) {
            val warm = URL(uri).openConnection() as HttpURLConnection
            try {
                warm.connectTimeout = 15_000
                warm.readTimeout = 120_000
                warm.setRequestProperty("Range", "bytes=0-0")
                warm.inputStream.use { it.read() }
            } finally { warm.disconnect() }
        }
        return MediaItem.Builder().setMediaId(track.getString("id")).setUri(uri)
            .setMimeType(resolved.optString("mimeType").takeIf { it.isNotBlank() })
            .setMediaMetadata(MediaMetadata.Builder().setTitle(track.optString("title")).setArtist(track.optString("artist"))
                .setDurationMs((track.optLong("duration") * 1000).takeIf { it > 0 })
                .setArtworkUri(track.optString("coverUrl").takeIf { it.isNotBlank() }?.let(android.net.Uri::parse)).build()).build()
    }

    private fun recoverCurrent(error: PlaybackException) {
        if (recovering) return
        val spec = queue.getOrNull(queueIndex) ?: run { publishState(STATE_ERROR, error.message); return }
        val position = player?.currentPosition ?: 0L
        val token = generation
        recovering = true
        worker.execute {
            val result = runCatching { resolveItem(spec, true) }
            progressHandler.post {
                if (token != generation) return@post
                recovering = false
                val p = player ?: return@post
                result.onSuccess { item -> val shouldPlay = keepPlaying; p.setMediaItem(item, position); p.prepare(); if (shouldPlay) p.play() }
                    .onFailure {
                        if (radioEnabled && queueIndex + 1 < queue.size) playQueueIndex(queueIndex + 1)
                        else if (radioEnabled) {
                            waitingAfterError = true
                            keepPlaying = true
                            maybeRefillRadio()
                            publishState(STATE_BUFFERING, error.message)
                        } else { keepPlaying = false; publishState(STATE_ERROR, error.message ?: "Ошибка Media3") }
                    }
            }
        }
    }

    private fun maybePreloadNext(p: Player) {
        if (preloadingNext || serverBaseUrl.isBlank() || currentTrackId.isBlank() || queue.getOrNull(queueIndex)?.optString("id") != currentTrackId) return
        if (repeat == "one" || p.mediaItemCount > p.currentMediaItemIndex + 1) return
        val next = if (shuffle) queue.drop(queueIndex + 1).randomOrNull() else queue.getOrNull(queueIndex + 1)
        if (next == null) return
        preloadingNext = true
        val token = generation
        worker.execute {
            val result = runCatching { resolveItem(next) }
            progressHandler.post {
                if (token != generation) return@post
                preloadingNext = false
                val currentPlayer = player ?: return@post
                result.onSuccess { item ->
                    currentPlayer.addMediaItem(item)
                    resumeAfterLatePreload()
                }.onFailure {
                    // Skip an unavailable candidate and keep filling the radio.
                    queue.remove(next)
                    publishState(stateFor(currentPlayer))
                    maybePreloadNext(currentPlayer)
                    maybeRefillRadio()
                }
            }
        }
    }

    private fun resumeAfterLatePreload() {
        val p = player ?: return
        if ((p.playbackState != Player.STATE_ENDED && !waitingAfterError) || !keepPlaying) return
        if (p.hasNextMediaItem()) { p.seekToNextMediaItem(); p.prepare(); p.play() }
        else if (repeat == "all" && !radioEnabled && queue.isNotEmpty()) playQueueIndex(0)
        else { maybePreloadNext(p); maybeRefillRadio() }
    }

    private fun skipNext() {
        val p = player ?: return
        keepPlaying = true
        if (shuffle && queue.size > 1) {
            val candidates = queue.indices.filter { it != queueIndex && (!radioEnabled || it > queueIndex) }
            if (candidates.isNotEmpty()) playQueueIndex(candidates.random()) else maybeRefillRadio()
        } else if (p.hasNextMediaItem()) { p.seekToNextMediaItem(); p.play() }
        else if (queueIndex + 1 < queue.size) playQueueIndex(queueIndex + 1)
        else if (repeat == "all" && !radioEnabled) playQueueIndex(0)
        else { p.seekTo(p.duration.takeIf { it != C.TIME_UNSET } ?: 0); maybePreloadNext(p); maybeRefillRadio() }
    }

    private fun playQueueIndex(index: Int, autoPlay: Boolean = true, positionMs: Long = 0) {
        val spec = queue.getOrNull(index) ?: return
        generation++
        preloadingNext = false
        keepPlaying = autoPlay
        val token = generation
        worker.execute {
            val result = runCatching { resolveItem(spec) }
            progressHandler.post {
                if (token != generation) return@post
                val p = player ?: return@post
                result.onSuccess { item -> val shouldPlay = keepPlaying; queueIndex = index; p.setMediaItem(item, positionMs); p.prepare(); if (shouldPlay) p.play() else p.pause() }
                    .onFailure { publishState(STATE_ERROR, it.message) }
            }
        }
    }

    private fun maybeRefillRadio() {
        if (!radioEnabled || radioLoading || serverBaseUrl.isBlank() || queue.isEmpty() || queue.size - queueIndex - 1 > 6 || System.currentTimeMillis() < retryRadioAt) return
        val anchor = queue[(queue.lastIndex - radioAnchorOffset % minOf(queue.size, 6)).coerceAtLeast(0)]
        val token = radioEpoch
        radioLoading = true
        radioWorker.execute {
            val result = runCatching { request("/v1/recommendations/related", JSONObject().put("track", backendTrack(anchor)).put("limit", 24)) }
            progressHandler.post {
                if (token != radioEpoch) return@post
                radioLoading = false
                if (!radioEnabled) return@post
                val candidates = result.getOrNull()?.optJSONArray("tracks") ?: JSONArray()
                val known = queue.map { it.optString("id") }.toMutableSet()
                val fresh = (0 until candidates.length()).map { frontendTrack(candidates.getJSONObject(it)) }
                    .filter { !rejectedTracks.contains(feedbackKey(it)) && !recentIds.contains(it.optString("id")) && known.add(it.optString("id")) }.take(12)
                if (fresh.isEmpty()) { radioAnchorOffset++; retryRadioAt = System.currentTimeMillis() + 5000; return@post }
                radioAnchorOffset = 0
                if (queueIndex > 100) { val remove = queueIndex - 100; repeat(remove) { queue.removeAt(0) }; queueIndex = 100 }
                queue.addAll(fresh)
                player?.let { publishState(stateFor(it)); maybePreloadNext(it) }
            }
        }
    }

    companion object {
        const val ACTION_PLAY = "ru.cryon.app.action.PLAY"
        const val ACTION_PAUSE = "ru.cryon.app.action.PAUSE"
        const val ACTION_RESUME = "ru.cryon.app.action.RESUME"
        const val ACTION_STOP = "ru.cryon.app.action.STOP"
        const val ACTION_SEEK = "ru.cryon.app.action.SEEK"
        const val ACTION_VOLUME = "ru.cryon.app.action.VOLUME"
        const val ACTION_QUEUE = "ru.cryon.app.action.QUEUE"
        const val ACTION_SYNC = "ru.cryon.app.action.SYNC"
        const val EXTRA_QUEUE = "queue"
        const val EXTRA_PAYLOAD = "payload"
        const val EXTRA_COMMAND_ID = "commandId"
        const val ACTION_STATE = "ru.cryon.app.action.PLAYBACK_STATE"
        const val ACTION_REMOTE_COMMAND = "ru.cryon.app.action.REMOTE_COMMAND"
        const val ACTION_PREVIOUS = "ru.cryon.app.action.PREVIOUS"
        const val ACTION_NEXT = "ru.cryon.app.action.NEXT"
        const val ACTION_LIKE = "ru.cryon.app.action.LIKE"
        const val ACTION_DISLIKE = "ru.cryon.app.action.DISLIKE"
        const val EXTRA_STREAM_URL = "streamUrl"
        const val EXTRA_MIME_TYPE = "mimeType"
        const val EXTRA_TRACK_ID = "trackId"
        const val EXTRA_TITLE = "title"
        const val EXTRA_ARTIST = "artist"
        const val EXTRA_ARTWORK_URL = "artworkUrl"
        const val EXTRA_AUTO_PLAY = "autoPlay"
        const val EXTRA_POSITION_MS = "positionMs"
        const val EXTRA_VOLUME = "volume"
        const val EXTRA_STATE = "state"
        const val EXTRA_PLAYING = "playing"
        const val EXTRA_DURATION_MS = "durationMs"
        const val EXTRA_ERROR = "error"
        const val EXTRA_REMOTE_COMMAND = "remoteCommand"
        const val STATE_IDLE = "idle"
        const val STATE_BUFFERING = "buffering"
        const val STATE_READY = "ready"
        const val STATE_ENDED = "ended"
        const val STATE_ERROR = "error"
        private const val CACHE_SIZE_BYTES = 256L * 1024L * 1024L
        private const val PROGRESS_INTERVAL_MS = 500L
        @Volatile var latestState: String = ""
        @Volatile var uiForeground: Boolean = false

        fun command(context: Context, action: String, configure: Intent.() -> Unit = {}) {
            val intent = Intent(context, CryonPlaybackService::class.java).setAction(action).apply(configure)
            // A restored paused queue must not start a foreground service without
            // an active media notification. Playback promotion happens only once
            // an explicit play request reaches Media3.
            if (action == ACTION_PLAY && intent.getBooleanExtra(EXTRA_AUTO_PLAY, false)) {
                ContextCompat.startForegroundService(context, intent)
            } else {
                context.startService(intent)
            }
        }
    }
}

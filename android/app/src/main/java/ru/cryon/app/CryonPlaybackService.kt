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

/**
 * Sole owner of Android audio playback. Media3 keeps playing when the WebView
 * renderer or activity is recreated; broadcasts are informational UI updates.
 */
class CryonPlaybackService : MediaSessionService() {
    private var mediaSession: MediaSession? = null
    private var player: ExoPlayer? = null
    private var cache: SimpleCache? = null
    private var currentTrackId = ""
    private val progressHandler = Handler(Looper.getMainLooper())
    private val progressTicker = object : Runnable {
        override fun run() {
            val playbackPlayer = player ?: return
            if (!playbackPlayer.isPlaying) return
            publishState(stateFor(playbackPlayer))
            progressHandler.postDelayed(this, PROGRESS_INTERVAL_MS)
        }
    }
    private val previousCommand = SessionCommand(ACTION_PREVIOUS, Bundle.EMPTY)
    private val nextCommand = SessionCommand(ACTION_NEXT, Bundle.EMPTY)

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
            .build()
            .also { playbackPlayer ->
                playbackPlayer.repeatMode = Player.REPEAT_MODE_OFF
                playbackPlayer.addListener(object : Player.Listener {
                    override fun onPlaybackStateChanged(playbackState: Int) {
                        publishState(
                            when (playbackState) {
                                Player.STATE_BUFFERING -> STATE_BUFFERING
                                Player.STATE_READY -> STATE_READY
                                Player.STATE_ENDED -> STATE_ENDED
                                else -> STATE_IDLE
                            },
                        )
                    }

                    override fun onIsPlayingChanged(isPlaying: Boolean) {
                        publishState(stateFor(playbackPlayer))
                        progressHandler.removeCallbacks(progressTicker)
                        if (isPlaying) progressHandler.post(progressTicker)
                    }

                    override fun onPlayerError(error: PlaybackException) {
                        publishState(STATE_ERROR, error.message ?: "Ошибка Media3")
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
            .setMediaButtonPreferences(
                listOf(
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
                ),
            )
            .setCallback(object : MediaSession.Callback {
                override fun onConnect(session: MediaSession, controllerInfo: MediaSession.ControllerInfo): MediaSession.ConnectionResult {
                    val commands = MediaSession.ConnectionResult.DEFAULT_SESSION_AND_LIBRARY_COMMANDS
                        .buildUpon()
                        .add(previousCommand)
                        .add(nextCommand)
                        .build()
                    return MediaSession.ConnectionResult.accept(commands, session.player.availableCommands)
                }

                override fun onCustomCommand(
                    session: MediaSession,
                    controllerInfo: MediaSession.ControllerInfo,
                    customCommand: SessionCommand,
                    args: Bundle,
                ): ListenableFuture<SessionResult> {
                    if (customCommand.customAction == ACTION_PREVIOUS || customCommand.customAction == ACTION_NEXT) {
                        sendBroadcast(
                            Intent(ACTION_REMOTE_COMMAND)
                                .setPackage(packageName)
                                .putExtra(EXTRA_REMOTE_COMMAND, customCommand.customAction),
                        )
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
        when (intent?.action) {
            ACTION_PLAY -> {
                currentTrackId = intent.getStringExtra(EXTRA_TRACK_ID).orEmpty()
                val item = MediaItem.Builder()
                    .setUri(intent.getStringExtra(EXTRA_STREAM_URL).orEmpty())
                    .setMimeType(intent.getStringExtra(EXTRA_MIME_TYPE)?.takeIf { it.isNotBlank() })
                    .setMediaId(currentTrackId)
                    .setMediaMetadata(
                        MediaMetadata.Builder()
                            .setTitle(intent.getStringExtra(EXTRA_TITLE))
                            .setArtist(intent.getStringExtra(EXTRA_ARTIST))
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
            ACTION_PAUSE -> playbackPlayer.pause()
            ACTION_RESUME -> playbackPlayer.play()
            ACTION_STOP -> {
                playbackPlayer.stop()
                currentTrackId = ""
                publishState(STATE_IDLE)
            }
            ACTION_SEEK -> playbackPlayer.seekTo(intent.getLongExtra(EXTRA_POSITION_MS, 0L).coerceAtLeast(0L))
            ACTION_VOLUME -> playbackPlayer.volume = intent.getIntExtra(EXTRA_VOLUME, 100).coerceIn(0, 100) / 100f
        }
        return START_NOT_STICKY
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession? = mediaSession

    override fun onTaskRemoved(rootIntent: Intent?) {
        if (player?.isPlaying != true) stopSelf()
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
        sendBroadcast(
            Intent(ACTION_STATE)
                .setPackage(packageName)
                .putExtra(EXTRA_TRACK_ID, currentTrackId)
                .putExtra(EXTRA_STATE, state)
                .putExtra(EXTRA_PLAYING, playbackPlayer.isPlaying)
                .putExtra(EXTRA_POSITION_MS, playbackPlayer.currentPosition.coerceAtLeast(0L))
                .putExtra(EXTRA_DURATION_MS, durationMs)
                .putExtra(EXTRA_ERROR, error.orEmpty()),
        )
    }

    companion object {
        const val ACTION_PLAY = "ru.cryon.app.action.PLAY"
        const val ACTION_PAUSE = "ru.cryon.app.action.PAUSE"
        const val ACTION_RESUME = "ru.cryon.app.action.RESUME"
        const val ACTION_STOP = "ru.cryon.app.action.STOP"
        const val ACTION_SEEK = "ru.cryon.app.action.SEEK"
        const val ACTION_VOLUME = "ru.cryon.app.action.VOLUME"
        const val ACTION_STATE = "ru.cryon.app.action.PLAYBACK_STATE"
        const val ACTION_REMOTE_COMMAND = "ru.cryon.app.action.REMOTE_COMMAND"
        const val ACTION_PREVIOUS = "ru.cryon.app.action.PREVIOUS"
        const val ACTION_NEXT = "ru.cryon.app.action.NEXT"
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

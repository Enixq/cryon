package ru.cryon.app

import android.content.Context
import android.webkit.JavascriptInterface

/** Shared by the real activity and foreground WebView instrumentation. */
open class NativePlaybackBridge(private val context: Context) {
    @JavascriptInterface
    fun nativePlay(streamUrl: String, trackId: String, title: String, artist: String, artworkUrl: String, durationMs: Long, autoPlay: Boolean, startPositionMs: Long, mimeType: String, commandId: Long) {
        CryonPlaybackService.command(context, CryonPlaybackService.ACTION_PLAY) {
            putExtra(CryonPlaybackService.EXTRA_COMMAND_ID, commandId)
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
    fun nativePause(commandId: Long) { CryonPlaybackService.command(context, CryonPlaybackService.ACTION_PAUSE) { putExtra(CryonPlaybackService.EXTRA_COMMAND_ID, commandId) } }

    @JavascriptInterface
    fun nativeSetQueue(json: String) {
        CryonPlaybackService.command(context, CryonPlaybackService.ACTION_QUEUE) { putExtra(CryonPlaybackService.EXTRA_QUEUE, json) }
    }

    @JavascriptInterface
    fun nativeGetState(): String = CryonPlaybackService.latestState

    @JavascriptInterface
    fun nativeConfigureSync(json: String) {
        CryonPlaybackService.command(context, CryonPlaybackService.ACTION_SYNC) { putExtra(CryonPlaybackService.EXTRA_QUEUE, json) }
    }

    @JavascriptInterface
    fun nativeResume(commandId: Long) { CryonPlaybackService.command(context, CryonPlaybackService.ACTION_RESUME) { putExtra(CryonPlaybackService.EXTRA_COMMAND_ID, commandId) } }

    @JavascriptInterface
    fun nativeStop(commandId: Long) { CryonPlaybackService.command(context, CryonPlaybackService.ACTION_STOP) { putExtra(CryonPlaybackService.EXTRA_COMMAND_ID, commandId) } }

    @JavascriptInterface
    fun nativeSeek(positionMs: Long) {
        CryonPlaybackService.command(context, CryonPlaybackService.ACTION_SEEK) {
            putExtra(CryonPlaybackService.EXTRA_POSITION_MS, positionMs)
        }
    }

    @JavascriptInterface
    fun nativeSetVolume(volume: Int) {
        CryonPlaybackService.command(context, CryonPlaybackService.ACTION_VOLUME) {
            putExtra(CryonPlaybackService.EXTRA_VOLUME, volume)
        }
    }

}

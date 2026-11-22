package ru.cryon.app

import android.content.ContentValues
import android.content.Context
import android.os.Build
import android.provider.MediaStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.json.JSONObject
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.net.ServerSocket
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger
import java.nio.ByteBuffer
import java.nio.ByteOrder

@RunWith(AndroidJUnit4::class)
class AudioExportTest {
 @Test fun savesCompleteAudioAutomaticallyAndDeletesOnlyItsOwnFile() {
  assumeTrue(Build.VERSION.SDK_INT >= 29)
  val context = InstrumentationRegistry.getInstrumentation().targetContext
  val key = "a".repeat(64)
  AudioDownloads.delete(context,key)
  val filename = "Cryon-export-test-${System.nanoTime()}.wav"
  val unrelated = checkNotNull(context.contentResolver.insert(MediaStore.Audio.Media.EXTERNAL_CONTENT_URI, ContentValues().apply { put(MediaStore.MediaColumns.DISPLAY_NAME,"Cryon-unrelated-test.wav");put(MediaStore.MediaColumns.MIME_TYPE,"audio/wav");put(MediaStore.MediaColumns.RELATIVE_PATH,"Music/Cryon") }))
  val audio = ByteArray(44+8000*2*45)
  ByteBuffer.wrap(audio).order(ByteOrder.LITTLE_ENDIAN).apply { put("RIFF".toByteArray());putInt(audio.size-8);put("WAVEfmt ".toByteArray());putInt(16);putShort(1);putShort(1);putInt(8000);putInt(16000);putShort(2);putShort(16);put("data".toByteArray());putInt(audio.size-44) }
  val socket = ServerSocket(0);val worker=Executors.newSingleThreadExecutor();val downloads=AtomicInteger()
  worker.execute { while(!socket.isClosed) { val peer=runCatching{socket.accept()}.getOrNull() ?: break; runCatching { peer.use { val reader=peer.getInputStream().bufferedReader();val request=reader.readLine().orEmpty();while(!reader.readLine().isNullOrBlank()){};val metadata=request.contains("metadata=1");val payload=if(metadata) JSONObject().put("name",filename).put("mimeType","audio/wav").put("size",audio.size).put("track",JSONObject().put("durationMs",45000)).toString().toByteArray() else audio.also {downloads.incrementAndGet()};peer.getOutputStream().apply{write("HTTP/1.1 200 OK\r\nContent-Length: ${payload.size}\r\nContent-Type: ${if(metadata) "application/json" else "audio/wav"}\r\nConnection: close\r\n\r\n".toByteArray());write(payload);flush()} } } } }
  val ticket="test-${System.nanoTime()}"
  try {
   AudioDownloads.start(context,"http://127.0.0.1:${socket.localPort}/v1/library/downloads/file?key=$key",ticket)
   val deadline=System.currentTimeMillis()+15000
   while(System.currentTimeMillis()<deadline && JSONObject(AudioDownloads.results[ticket] ?: "{}").optString("state") !in listOf("saved","error")) Thread.sleep(100)
   check(JSONObject(AudioDownloads.results[ticket]!!).optString("state")=="saved") {AudioDownloads.results[ticket].orEmpty()}
   check(downloads.get()==1)
   val stored=context.getSharedPreferences("cryon_audio_exports",Context.MODE_PRIVATE).getString(key,null)!!
   val uri=android.net.Uri.parse(stored)
   check(context.contentResolver.openInputStream(uri)!!.use{it.readBytes()}.contentEquals(audio)) {"Saved audio truncated"}
   val retriever=android.media.MediaMetadataRetriever();try{retriever.setDataSource(context,uri);check(retriever.extractMetadata(android.media.MediaMetadataRetriever.METADATA_KEY_DURATION)!!.toLong()>=45000)}finally{retriever.release()}
   check(!AudioDownloads.delete(context,"../../bad"))
   check(AudioDownloads.delete(context,key))
   context.contentResolver.query(unrelated,arrayOf(MediaStore.MediaColumns._ID),null,null,null)!!.use{check(it.moveToFirst()) {"Unrelated file deleted"}}
  } finally { AudioDownloads.delete(context,key);context.contentResolver.delete(unrelated,null,null);AudioDownloads.results.remove(ticket);socket.close();worker.shutdownNow() }
 }
 @Test fun remuxedFileContainsEntireDuration() {
  val path=InstrumentationRegistry.getArguments().getString("remuxPath") ?: return
  val retriever=android.media.MediaMetadataRetriever()
  try { retriever.setDataSource(path);val duration=retriever.extractMetadata(android.media.MediaMetadataRetriever.METADATA_KEY_DURATION)!!.toLong();check(duration>=150000) {"Remux still reads only $duration ms"} } finally {retriever.release()}
 }
}

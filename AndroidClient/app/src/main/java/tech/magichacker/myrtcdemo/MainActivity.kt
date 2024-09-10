package tech.magichacker.myrtcdemo

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.util.Log
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.PeerConnectionFactory
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import tech.magichacker.myrtcdemo.databinding.ActivityMainBinding
import java.util.Collections

class MainActivity : AppCompatActivity() {
    private lateinit var binding: ActivityMainBinding

    private lateinit var peerConnection: PeerConnection
    private val mediaConstraints = MediaConstraints().apply {
        mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveAudio", "true"))
        mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveVideo", "false"))
    }
    private var hasRemoteDesc = false
    private var isConnected = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)
        if (checkSelfPermission(android.Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(
                android.Manifest.permission.RECORD_AUDIO
            ), 100)
            return
        }

        binding.action.setOnClickListener {
            binding.action.text = if (isConnected) "Stop" else "Start"
            if (isConnected) {
                stop()
            } else {
                start()
            }
        }
    }

    private fun start() {
        val initializationOptions = PeerConnectionFactory.InitializationOptions.builder(this)
            .createInitializationOptions()
        PeerConnectionFactory.initialize(initializationOptions)
        val factory = PeerConnectionFactory.builder().createPeerConnectionFactory()

        val iceServers = listOf(
            PeerConnection.IceServer.builder("stun:stun.l.google.com:19302").createIceServer()
        )
        val rtcConfig = PeerConnection.RTCConfiguration(iceServers)
        peerConnection = factory.createPeerConnection(rtcConfig, object : PeerConnection.Observer {
            override fun onIceCandidate(iceCandidate: IceCandidate) {
                lifecycleScope.launch {
                    if (hasRemoteDesc) {
                        return@launch
                    }
                    hasRemoteDesc = true
                    val descriptor = peerConnection.localDescription
                    val answer = sendOffer(offer = descriptor.description.orEmpty())
                    log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                    log("after send answer: $answer")
                    log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                    val remoteDesc = SessionDescription(
                        SessionDescription.Type.ANSWER,
                        answer
                    )
                    peerConnection.setRemoteDescription(object : SdpObserver {
                        override fun onCreateSuccess(p0: SessionDescription?) {
                        }

                        override fun onSetSuccess() {
                            log("remote set success")
                        }

                        override fun onCreateFailure(p0: String?) {
                        }

                        override fun onSetFailure(p0: String?) {
                            log("remote set failure $p0")
                        }
                    }, remoteDesc)
                }
            }

            override fun onIceCandidatesRemoved(p0: Array<out IceCandidate>?) {
            }

            override fun onAddStream(mediaStream: MediaStream) {
            }

            override fun onSignalingChange(signalingState: PeerConnection.SignalingState) {}
            override fun onIceConnectionChange(iceConnectionState: PeerConnection.IceConnectionState) {}
            override fun onIceConnectionReceivingChange(receiving: Boolean) {}
            override fun onIceGatheringChange(iceGatheringState: PeerConnection.IceGatheringState) {}
            override fun onRemoveStream(mediaStream: MediaStream) {}
            override fun onDataChannel(dataChannel: DataChannel) {}
            override fun onRenegotiationNeeded() {}
        })!!

        peerConnection.createDataChannel("results", DataChannel.Init().apply {
            ordered = true
        })

        val mediaStreamLabels = Collections.singletonList("ARDAMS")
        val audioSource = factory.createAudioSource(mediaConstraints)
        val localAudioTrack = factory.createAudioTrack("1000", audioSource)
        localAudioTrack.setEnabled(true)
        peerConnection.addTrack(localAudioTrack, mediaStreamLabels)

        peerConnection.createOffer(object : SdpObserver {
            override fun onCreateSuccess(desc: SessionDescription?) {
                log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                log("onCreateSuccess ${desc?.type} ${desc?.description.orEmpty()}")
                log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                peerConnection.setLocalDescription(object : SdpObserver {
                    override fun onCreateSuccess(d: SessionDescription?) {
                    }

                    override fun onSetSuccess() {
                        log("onSetSuccess")
                    }

                    override fun onCreateFailure(p0: String?) {
                    }

                    override fun onSetFailure(p0: String?) {
                    }
                })
            }

            override fun onSetSuccess() {
            }

            override fun onCreateFailure(p0: String?) {
            }

            override fun onSetFailure(p0: String?) {
            }
        }, mediaConstraints)
    }

    private fun stop() {

    }

    suspend fun sendOffer(offer: String): String? {
        val client = OkHttpClient()

        val json = JSONObject()
        json.put("offer", offer)

        val mediaType = "application/json; charset=utf-8".toMediaTypeOrNull()
        val requestBody = json.toString().toRequestBody(mediaType)

        val request = Request.Builder()
            .url("http://10.193.198.129:9000/session")
            .post(requestBody)
            .build()

        return withContext(Dispatchers.IO) {
            val response = client.newCall(request).execute()
            if (response.isSuccessful) {
                val responseBody = response.body?.string()
                val responseJson = JSONObject(responseBody)
                responseJson.getString("answer")
            } else {
                log("request error: ${response.code}")
                null
            }
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (resultCode == Activity.RESULT_OK && requestCode == 100) {
            finish()
            startActivity(Intent(this@MainActivity, MainActivity::class.java))
        }
    }

    private fun log(message: String) {
        Log.d("tsss", message)
    }
}
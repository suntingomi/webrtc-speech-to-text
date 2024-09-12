package tech.magichacker.myrtcdemo

import android.os.Bundle
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
import org.webrtc.AudioTrack
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.PeerConnectionFactory
import org.webrtc.RtpReceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import tech.magichacker.myrtcdemo.databinding.ActivityConnectionBinding
import java.util.Collections

class NormalModeActivity : AppCompatActivity() {
    private lateinit var binding: ActivityConnectionBinding

    private var dataChannel: DataChannel? = null
    private var peerConnection: PeerConnection? = null
    private val mediaConstraints = MediaConstraints().apply {
        mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveAudio", "true"))
        mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveVideo", "false"))
    }
    private var hasRemoteDesc = false
    private var isConnected = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityConnectionBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.action.setOnClickListener {
            if (isConnected) {
                stop()
            } else {
                start()
            }
            isConnected = !isConnected
            binding.action.text = if (isConnected) "Stop" else "Start"
        }
    }

    private fun start() {
        val initializationOptions = PeerConnectionFactory.InitializationOptions.builder(this)
            .createInitializationOptions()
        PeerConnectionFactory.initialize(initializationOptions)
        val factory = PeerConnectionFactory
            .builder()
            .createPeerConnectionFactory()

//        val iceServers = listOf(
//            PeerConnection.IceServer.builder("stun:stun.l.google.com:19302").createIceServer()
//        )
        val rtcConfig = PeerConnection.RTCConfiguration(emptyList())
        peerConnection = factory.createPeerConnection(rtcConfig, object : PeerConnection.Observer {
            override fun onIceCandidate(iceCandidate: IceCandidate) {
                log("onIceCandidate")
                lifecycleScope.launch {
                    if (hasRemoteDesc) {
                        return@launch
                    }
                    hasRemoteDesc = true
                    val descriptor = peerConnection?.localDescription
                    val answer = sendOffer(offer = descriptor?.description.orEmpty())
                    // log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                    // log("after send answer: $answer")
                    // log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                    val remoteDesc = SessionDescription(
                        SessionDescription.Type.ANSWER,
                        answer
                    )
                    peerConnection?.setRemoteDescription(object : SdpObserver {
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
                log("onAddStream")
            }

            override fun onSignalingChange(signalingState: PeerConnection.SignalingState) {}
            override fun onIceConnectionChange(iceConnectionState: PeerConnection.IceConnectionState) {}
            override fun onIceConnectionReceivingChange(receiving: Boolean) {}
            override fun onIceGatheringChange(iceGatheringState: PeerConnection.IceGatheringState) {}
            override fun onRemoveStream(mediaStream: MediaStream) {}
            override fun onDataChannel(dataChannel: DataChannel) {
                log("onDataChannel")
            }
            override fun onRenegotiationNeeded() {
                log("onRenegotiationNeeded")
            }
            override fun onAddTrack(receiver: RtpReceiver?, mediaStreams: Array<out MediaStream>?) {
                log("onAddTrack")
                // 这里也可以不写，默认就是开启的
                (receiver?.track() as? AudioTrack)?.setEnabled(true)
            }
        })!!

        dataChannel = peerConnection?.createDataChannel("results", DataChannel.Init().apply {
            ordered = true
        })

        val mediaStreamLabels = Collections.singletonList("ARDAMS")
        val audioSource = factory.createAudioSource(mediaConstraints)
        val localAudioTrack = factory.createAudioTrack("1000", audioSource)
        localAudioTrack.setEnabled(true)
        peerConnection?.addTrack(localAudioTrack, mediaStreamLabels)

        peerConnection?.createOffer(object : SdpObserver {
            override fun onCreateSuccess(desc: SessionDescription?) {
                // log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                // log("onCreateSuccess ${desc?.type} ${desc?.description.orEmpty()}")
                // log(">>>>>>>>>>>>>>>>>>>>>>>>>=====================================")
                peerConnection?.setLocalDescription(object : SdpObserver {
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
        dataChannel?.close()
        peerConnection?.transceivers?.forEach {
            it.sender.track()?.setEnabled(false)
        }
        peerConnection?.close()
        peerConnection?.dispose()
    }

    suspend fun sendOffer(offer: String): String? {
        val client = OkHttpClient()

        val json = JSONObject()
        json.put("offer", offer)

        val mediaType = "application/json; charset=utf-8".toMediaTypeOrNull()
        val requestBody = json.toString().toRequestBody(mediaType)

        val request = Request.Builder()
            .url("http://10.193.199.41:9000/session")
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
}
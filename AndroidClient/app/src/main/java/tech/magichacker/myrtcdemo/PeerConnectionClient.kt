package tech.magichacker.myrtcdemo

import android.content.Context
import android.util.Log
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.PeerConnectionFactory
import org.webrtc.RtpReceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import org.webrtc.audio.JavaAudioDeviceModule
import org.webrtc.audio.JavaAudioDeviceModule.AudioRecordStateCallback
import org.webrtc.audio.JavaAudioDeviceModule.AudioTrackStateCallback
import java.util.Collections
import java.util.concurrent.Executors

class PeerConnectionClient(private val context: Context) {

    private val mediaConstraints by lazy { MediaConstraints() }
    private var webSocket: WebSocket? = null
    private var peerConnectionFactory: PeerConnectionFactory? = null
    private var peerConnection: PeerConnection? = null

    private val executor = Executors.newSingleThreadExecutor()
    private var makingOffer = false
    private var ignoreOffer = false

    companion object {
        private const val DEBUG = true
        private const val TAG = "PeerConnectionClient"
    }

    private val peerObserver = object : PeerConnection.Observer {
        override fun onSignalingChange(state: PeerConnection.SignalingState?) {
            log("onSignalingChange state = $state")
        }

        override fun onIceConnectionChange(state: PeerConnection.IceConnectionState?) {
            log("onIceConnectionChange state = $state")
        }

        override fun onIceConnectionReceivingChange(changed: Boolean) {
            log("onIceConnectionReceivingChange changed = $changed")
        }

        override fun onIceGatheringChange(state: PeerConnection.IceGatheringState?) {
            log("onIceGatheringChange state = $state")
        }

        override fun onIceCandidate(candidate: IceCandidate?) {
            log("onIceCandidate candidate = $candidate")
            candidate?.let {
                executor.execute {
                    sendCandidate(it)
                }
            }
        }

        override fun onIceCandidatesRemoved(p0: Array<out IceCandidate>?) {
            log("onIceCandidatesRemoved")
        }

        override fun onAddStream(mediaStream: MediaStream?) {
            log("onAddStream")
        }

        override fun onRemoveStream(mediaStream: MediaStream?) {
            log("onRemoveStream")
        }

        override fun onDataChannel(channel: DataChannel?) {
            log("onDataChannel")
        }

        override fun onRenegotiationNeeded() {
            log("onRenegotiationNeeded")
            createOffer()
        }

        override fun onAddTrack(receiver: RtpReceiver?, mediaStreams: Array<out MediaStream>?) {
            log("onAddTrack")
        }
    }

    fun start() {
        val client = OkHttpClient.Builder().build()
        val request = Request.Builder().url("ws://10.193.199.41:9000/").build()
        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                log("Websocket onOpen")
                createPeerConnection()
                addAudioTrack()
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                log("Websocket onClosed code = $code, reason = $reason")
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                log("Websocket onMessage text = $text")
                executor.execute {
                    val json = JSONObject(text)
                    val type = json.optString("type")
                    when (type) {
                        "offer" -> {
                            makingOffer = false
                            val sdp = json.optString("sdp")
                            val desc = SessionDescription(SessionDescription.Type.OFFER, sdp)
                            setRemoteDescriptor(desc)
                        }
                        "answer" -> {
                            val sdp = json.optString("sdp")
                            val desc = SessionDescription(SessionDescription.Type.ANSWER, sdp)
                            setRemoteDescriptor(desc)
                        }
                        else -> {
                            val candidate = json.optString("candidate")
                            if (!candidate.isNullOrEmpty()) {
                                peerConnection?.addIceCandidate(
                                    IceCandidate(
                                        json.optString("sdpMid").orEmpty(),
                                        json.optInt("sdpMLineIndex"),
                                        json.optString("candidate").orEmpty(),
                                    )
                                )
                            }
                        }
                    }
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                log("Websocket onFailure")
            }
        })
        client.dispatcher.executorService.shutdown()
    }

    fun stop() {
        webSocket?.close(1000, "")
        webSocket = null
        peerConnection?.close()
        peerConnection = null
        executor.shutdown()
    }

    private fun createPeerConnection() {
        val initializationOptions = PeerConnectionFactory.InitializationOptions.builder(context)
            .createInitializationOptions()
        PeerConnectionFactory.initialize(initializationOptions)
        peerConnectionFactory = PeerConnectionFactory
            .builder()
            .setAudioDeviceModule(JavaAudioDeviceModule.builder(context)
                .setAudioTrackStateCallback(object : AudioTrackStateCallback {
                    override fun onWebRtcAudioTrackStart() {
                        log("onWebRtcAudioTrackStart")
                    }

                    override fun onWebRtcAudioTrackStop() {
                        log("onWebRtcAudioTrackStop")
                    }
                })
                .setAudioRecordStateCallback(object : AudioRecordStateCallback {
                    override fun onWebRtcAudioRecordStart() {
                        log("onWebRtcAudioRecordStart")
                    }

                    override fun onWebRtcAudioRecordStop() {
                        log("onWebRtcAudioRecordStop")
                    }
                })
                .createAudioDeviceModule())
            .createPeerConnectionFactory()
        val rtcConfig = PeerConnection.RTCConfiguration(emptyList())
        peerConnection = peerConnectionFactory?.createPeerConnection(rtcConfig, peerObserver)
    }

    private fun addAudioTrack() {
        val mediaStreamLabels = Collections.singletonList("assistant")
        val audioSource = peerConnectionFactory?.createAudioSource(mediaConstraints)
        val localAudioTrack = peerConnectionFactory?.createAudioTrack("0001", audioSource)
        localAudioTrack?.let {
            it.setEnabled(true)
            peerConnection?.addTrack(it, mediaStreamLabels)
        }
    }

    private fun createOffer() {
        executor.execute {
            makingOffer = true
            peerConnection?.createOffer(object : SdpObserver {
                override fun onCreateSuccess(desc: SessionDescription?) {
                    desc?.let {
                        setLocalDescriptor(it)
                    }
                }

                override fun onSetSuccess() {
                }

                override fun onCreateFailure(msg: String?) {
                }

                override fun onSetFailure(msg: String?) {
                }
            }, mediaConstraints)
        }
    }

    private fun setLocalDescriptor(desc: SessionDescription) {
        executor.execute {
            peerConnection?.setLocalDescription(object : SdpObserver {
                override fun onCreateSuccess(sd: SessionDescription?) {
                }

                override fun onSetSuccess() {
                    if (makingOffer) {
                        sendOffer(desc)
                    } else {
                        sendAnswer(desc)
                    }
                }

                override fun onCreateFailure(msg: String?) {
                }

                override fun onSetFailure(msg: String?) {
                }
            }, desc)
        }
    }

    private fun setRemoteDescriptor(desc: SessionDescription) {
        executor.execute {
            peerConnection?.setRemoteDescription(object : SdpObserver {
                override fun onCreateSuccess(sd: SessionDescription?) {
                }

                override fun onSetSuccess() {
                    executor.execute {
                        if (makingOffer) {
                            log("Remote description set success.")
                            makingOffer = false
                        } else {
                            createAnswer()
                        }
                    }
                }

                override fun onCreateFailure(msg: String?) {
                }

                override fun onSetFailure(msg: String?) {
                }
            }, desc)
        }
    }

    private fun createAnswer() {
        peerConnection?.createAnswer(object : SdpObserver {
            override fun onCreateSuccess(desc: SessionDescription?) {
                desc?.let { setLocalDescriptor(it) }
            }

            override fun onSetSuccess() {
            }

            override fun onCreateFailure(p0: String?) {
            }

            override fun onSetFailure(p0: String?) {
            }
        }, mediaConstraints)
    }

    private fun sendOffer(desc: SessionDescription) {
        val json = JSONObject()
        json.put("type", "offer")
        json.put("sdp", desc.description)
        webSocket?.send(json.toString())
    }

    private fun sendAnswer(desc: SessionDescription) {
        val json = JSONObject()
        json.put("type", "answer")
        json.put("sdp", desc.description)
        webSocket?.send(json.toString())
    }

    private fun sendCandidate(candidate: IceCandidate) {
        val json = JSONObject()
        json.put("type", "candidate")
        json.put("sdpMid", candidate.sdpMid)
        json.put("sdpMLineIndex", candidate.sdpMLineIndex)
        json.put("candidate", candidate.sdp)
        webSocket?.send(json.toString())
    }

    private fun log(message: String) {
        if (DEBUG) {
            Log.d(TAG, message)
        }
    }
}
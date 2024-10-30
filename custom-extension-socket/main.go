package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"github.com/gorilla/websocket"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WebSocketConnection struct {
	Conn           *websocket.Conn
	PeerConnection *webrtc.PeerConnection
}

var connections = make(map[*websocket.Conn]*WebSocketConnection)
var connectionsLock sync.Mutex

const sessionIDURI = "urn:ietf:params:rtp-hdrext:session-id"
var sessionExtId = -1

func updateSessionExtIdIfNeeded(peerConnection *webrtc.PeerConnection) {
	receivers := peerConnection.GetReceivers()
	log.Printf("senders: %d\n", len(receivers))
	for _, s := range receivers {
		for _, headerExt := range s.GetParameters().HeaderExtensions {
			log.Printf("id: %d, uri: %s\n", headerExt.ID, headerExt.URI)
			if headerExt.URI == sessionIDURI {
				sessionExtId = headerExt.ID
			}
		}
	}
	log.Printf("id: %d\n", sessionExtId)
}

func main() {
	http.HandleFunc("/", handleConnections)
	log.Println("HTTP server started on :9000")
	err := http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	mediaEngine := &webrtc.MediaEngine{}
	mediaEngine.RegisterDefaultCodecs()
	interceptorRegistry := &interceptor.Registry{}
	webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry)
	mediaEngine.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: sessionIDURI}, webrtc.RTPCodecTypeAudio)
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Fatal(err)
	}

	connectionsLock.Lock()
	connections[ws] = &WebSocketConnection{Conn: ws, PeerConnection: peerConnection}
	connectionsLock.Unlock()

	defer func() {
		connectionsLock.Lock()
		delete(connections, ws)
		connectionsLock.Unlock()
		peerConnection.Close()
	}()

	peerConnection.OnNegotiationNeeded(func() {
		log.Printf("OnNegotiationNeeded\n")
		desc, err := peerConnection.CreateOffer(nil)
		if err != nil {
			log.Printf("error create offer %v\n", err)
		}

		err = peerConnection.SetLocalDescription(desc)
		if err != nil {
			log.Printf("error set local descriptor%v\n", err)
		}
		offerJSON, err := json.Marshal(desc)
		ws.WriteMessage(websocket.TextMessage, offerJSON)
	})

	peerConnection.OnICEConnectionStateChange(func(connState webrtc.ICEConnectionState) {
		log.Printf("Connection state: %s \n", connState.String())
		if connState == webrtc.ICEConnectionStateConnected {
			updateSessionExtIdIfNeeded(peerConnection)
		}
	})

	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		candidateJSON, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			log.Println("Error marshaling ICE candidate:", err)
			return
		}
		ws.WriteMessage(websocket.TextMessage, candidateJSON)
	})

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("OnTrack %v\n", receiver.Track().Codec())
		go func() {
			for {
				rtpPacket, _, readErr := track.ReadRTP()
				if readErr != nil {
					log.Printf("Error reading RTP packet: %v\n", readErr)
					return
				}
				log.Printf("extension: %v, %d, %d\n", rtpPacket.Extension, len(rtpPacket.Extensions), sessionExtId)
				if sessionExtId >= 0 {
					payloadSession := rtpPacket.GetExtension(uint8(sessionExtId))
					log.Printf("payload len: %d, content: %v\n", len(payloadSession), string(payloadSession))
				}

				log.Printf("Received RTP packet: %v\n", rtpPacket)
			}
		}()
	})

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			break
		}

		log.Printf("message: %s", message)
		var msg map[string]interface{}
		err = json.Unmarshal(message, &msg)
		if err != nil {
			log.Println("Error unmarshaling message:", err)
			continue
		}

		switch msg["type"] {
		case "offer":
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg["sdp"].(string),
			}
			err = peerConnection.SetRemoteDescription(offer)
			if err != nil {
				log.Println("Error setting remote description:", err)
				continue
			}

			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Println("Error creating answer:", err)
				continue
			}

			err = peerConnection.SetLocalDescription(answer)
			if err != nil {
				log.Println("Error setting local description:", err)
				continue
			}

			answerJSON, err := json.Marshal(answer)
			if err != nil {
				log.Println("Error marshaling answer:", err)
				continue
			}
			ws.WriteMessage(websocket.TextMessage, answerJSON)

		case "answer":
			answer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  msg["sdp"].(string),
			}
			err = peerConnection.SetRemoteDescription(answer)
			if err != nil {
				log.Println("Error setting remote description:", err)
				continue
			}

		case "candidate":
			sdpMid := msg["sdpMid"].(string)
			sdpMLineIndex := uint16(msg["sdpMLineIndex"].(float64))
			candidate := webrtc.ICECandidateInit{
				Candidate:     msg["candidate"].(string),
				SDPMid:        &sdpMid,
				SDPMLineIndex: &sdpMLineIndex,
			}
			err = peerConnection.AddICECandidate(candidate)
			if err != nil {
				log.Println("Error adding ICE candidate:", err)
				continue
			}

		case "hello":
		    desc, err := peerConnection.CreateOffer(nil)
		    if err != nil {
		    	log.Printf("error create offer %v\n", err)
		    }

		    err = peerConnection.SetLocalDescription(desc)
		    if err != nil {
		    	log.Printf("error set local descriptor%v\n", err)
		    }
		    offerJSON, err := json.Marshal(desc)
		    ws.WriteMessage(websocket.TextMessage, offerJSON)
		}
	}
}

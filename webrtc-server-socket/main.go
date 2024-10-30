package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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

	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
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
			time.Sleep(5 * time.Second)
			log.Printf("begin to create new connection\n")
			localTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
			if err != nil {
				log.Printf("error create local track %v\n", err)
			}
			_, e1 := peerConnection.AddTrack(localTrack)
			if e1 != nil {
				log.Printf("error add local track %v\n", e1)
			}
		}()
		for {
			// rtp, _, err := track.ReadRTP()
			_, _, err := track.ReadRTP()
			if err != nil {
				log.Println("Error reading RTP packet:", err)
				return
			}
			// fmt.Printf("Received audio RTP packet: %v\n", rtp)
		}
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
		}
	}
}

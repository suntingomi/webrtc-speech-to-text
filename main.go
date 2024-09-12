package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
	"os"
	"errors"
	"io"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
	"github.com/pion/rtp"
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
		playFromDisk("output.ogg", peerConnection)
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

func playFromDisk(audioFileName string, pc *webrtc.PeerConnection) {
	_, err := os.Stat(audioFileName)
	haveAudioFile := !os.IsNotExist(err)
	if !haveAudioFile {
		log.Printf("No audio file!\n")
		return
	}
	if haveAudioFile {
		audioTrack, audioTrackErr := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
		if audioTrackErr != nil {
			panic(audioTrackErr)
		}

		rtpSender, audioTrackErr := pc.AddTrack(audioTrack)
		if audioTrackErr != nil {
			panic(audioTrackErr)
		}

		// Read incoming RTCP packets
		// Before these packets are returned they are processed by interceptors. For things
		// like NACK this needs to be called.
		go func() {
			rtcpBuf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					return
				}
			}
		}()

		go func() {
			file, oggErr := os.Open(audioFileName)
			if oggErr != nil {
				panic(oggErr)
			}

			ogg, _, oggErr := oggreader.NewWith(file)
			if oggErr != nil {
				panic(oggErr)
			}

			var lastGranule uint64

			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()

			sequenceNumber := uint16(0)
			for ; true; <-ticker.C {
				pageData, pageHeader, oggErr := ogg.ParseNextPage()
				if errors.Is(oggErr, io.EOF) {
					fmt.Printf("All audio pages parsed and sent")
					break
				}

				if oggErr != nil {
					panic(oggErr)
				}

				sampleCount := float64(pageHeader.GranulePosition - lastGranule)
				lastGranule = pageHeader.GranulePosition
				sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    111, // Opus payload type
						SequenceNumber: sequenceNumber,
						Timestamp:      uint32(lastGranule),
						SSRC:           12345,
					},
					Payload: pageData,
				}
				sequenceNumber++

				if oggErr = audioTrack.WriteRTP(packet); oggErr != nil {
					panic(oggErr)
				}
				log.Printf("success WriteRTP %v\n", sampleDuration)
			}
		}()
	}
}

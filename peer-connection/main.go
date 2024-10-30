package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/pion/webrtc/v3"
)

type OfferData struct {
	Offer string `json:"offer"`
}

type AnswerData struct {
	Answer string `json:"answer"`
}

func makeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		dec := json.NewDecoder(r.Body)
		req := OfferData{}

		if err := dec.Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		offer := req.Offer

		config := webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		}
		pc, err := webrtc.NewPeerConnection(config)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		// defer peer.Close()
		log.Printf("success create webrtc connection!\n")

		pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			if track.Kind() == webrtc.RTPCodecTypeAudio {
				log.Printf("Received audio track, id = %s\n", track.ID())

				go func() {
					for {
						rtpPacket, _, readErr := track.ReadRTP()
						if readErr != nil {
							log.Printf("Error reading RTP packet: %v\n", readErr)
							return
						}

						fmt.Printf("Received RTP packet: %v\n", rtpPacket)
					}
				}()
			}
		})

		pc.OnICEConnectionStateChange(func(connState webrtc.ICEConnectionState) {
			log.Printf("Connection state: %s \n", connState.String())
		})

		err = pc.SetRemoteDescription(webrtc.SessionDescription{
			SDP:  offer,
			Type: webrtc.SDPTypeOffer,
		})
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus,
		}, "audio", "pion")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		_, err = pc.AddTrack(track)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = pc.SetLocalDescription(answer)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		payload, err := json.Marshal(AnswerData{
			Answer: answer.SDP,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(payload)
	})
	return mux
}

var httpPort = "9000"

func main() {
	http.Handle("/", makeHandler())

	errors := make(chan error, 2)
	go func() {
		log.Printf("Starting signaling server on port %s", httpPort)
		errors <- http.ListenAndServe(fmt.Sprintf(":%s", httpPort), nil)
	}()

	go func() {
		interrupt := make(chan os.Signal)
		signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
		errors <- fmt.Errorf("Received %v signal", <-interrupt)
	}()

	err := <-errors
	log.Printf("%s, exiting.", err)
}

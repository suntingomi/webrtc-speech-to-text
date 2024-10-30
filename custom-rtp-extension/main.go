package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
)

const (
	audioFileName   = "output.ogg"
	oggPageDuration = time.Millisecond * 20
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

		_, err := os.Stat(audioFileName)
		haveAudioFile := !os.IsNotExist(err)
		if !haveAudioFile {
			log.Printf("No audio file!\n")
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

		mediaEngine := &webrtc.MediaEngine{}
		sessionIDURI := "urn:ietf:params:rtp-hdrext:ssrc-seq"
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
		pc, err := api.NewPeerConnection(config)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		// defer peer.Close()
		log.Printf("success create webrtc connection!\n")

		iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.Background())

		uri := "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"
		extensionId := -1
		sessionExtId := -1

		if haveAudioFile {
			// Create a audio track
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
				// Open a OGG file and start reading using our OGGReader
				file, oggErr := os.Open(audioFileName)
				if oggErr != nil {
					panic(oggErr)
				}

				// Open on oggfile in non-checksum mode.
				ogg, _, oggErr := oggreader.NewWith(file)
				if oggErr != nil {
					panic(oggErr)
				}

				// Wait for connection established
				<-iceConnectedCtx.Done()

				// Keep track of last granule, the difference is the amount of samples in the buffer
				var lastGranule uint64

				// It is important to use a time.Ticker instead of time.Sleep because
				// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
				// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()

				sequenceNumber := uint16(0)
				for ; true; <-ticker.C {
					pageData, pageHeader, oggErr := ogg.ParseNextPage()
					if errors.Is(oggErr, io.EOF) {
						fmt.Printf("All audio pages parsed and sent")
						os.Exit(0)
					}

					if oggErr != nil {
						panic(oggErr)
					}

					// The amount of samples is the difference between the last and current timestamp
					// sampleCount := float64(pageHeader.GranulePosition - lastGranule)
					lastGranule = pageHeader.GranulePosition
					// sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

					useExtension := true

					// Create RTP packet
					packet := &rtp.Packet{
						Header: rtp.Header{
							Version:          2,
							PayloadType:      111, // Opus payload type
							SequenceNumber:   sequenceNumber,
							Timestamp:        uint32(lastGranule),
							SSRC:             12345,
							Extension:        useExtension,
							ExtensionProfile: 0xBEDE,
						},
						Payload: pageData,
					}
					sequenceNumber++

					// if useExtension && extensionId > -1 {
					// 	bytes := make([]byte, 4)
					// 	binary.BigEndian.PutUint32(bytes, 12345)
					// 	fmt.Printf("extension id %d\n", extensionId)
					// 	packet.SetExtension(uint8(extensionId), bytes)
					// }

					if useExtension && sessionExtId > -1 {
						bytes := make([]byte, 4)
						binary.BigEndian.PutUint32(bytes, 12345)
						fmt.Printf("extension id %d\n", sessionExtId)
						packet.SetExtension(uint8(sessionExtId), bytes)
					}

					// Write RTP packet
					if oggErr = audioTrack.WriteRTP(packet); oggErr != nil {
						panic(oggErr)
					}
					// log.Printf("success WriteRTP %v\n", sampleDuration)
				}
			}()
		}

		pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			if track.Kind() == webrtc.RTPCodecTypeAudio {
				log.Printf("Received audio track, id = %s\n", track.ID())

				// go func() {
				// 	for {
				// 		rtpPacket, _, readErr := track.ReadRTP()
				// 		if readErr != nil {
				// 			log.Printf("Error reading RTP packet: %v\n", readErr)
				// 			return
				// 		}
				// 		fmt.Printf("extension: %v, %d, %d\n", rtpPacket.Extension, len(rtpPacket.Extensions), sessionExtId)
				// 		if sessionExtId >= 0 {
				// 			payloadSession := rtpPacket.GetExtension(uint8(sessionExtId))
				// 			fmt.Printf("payload len: %d, content: %v\n", len(payloadSession), binary.BigEndian.Uint32(payloadSession))
				// 		}

				// 		fmt.Printf("Received RTP packet: %v\n", rtpPacket)
				// 	}
				// }()
			}
		})

		pc.OnICEConnectionStateChange(func(connState webrtc.ICEConnectionState) {
			log.Printf("Connection state: %s \n", connState.String())
			if connState == webrtc.ICEConnectionStateConnected {
				iceConnectedCtxCancel()

				for _, s := range pc.GetSenders() {
					for _, headerExt := range s.GetParameters().HeaderExtensions {
						fmt.Printf("id: %d, uri: %s\n", headerExt.ID, headerExt.URI)
						if headerExt.URI == uri {
							extensionId = headerExt.ID
						} else if headerExt.URI == sessionIDURI {
							sessionExtId = headerExt.ID
						}
					}
				}
				fmt.Printf("id1: %d, id2: %d\n", extensionId, sessionExtId)
			}
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

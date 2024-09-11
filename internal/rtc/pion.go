package rtc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/rviscarra/webrtc-speech-to-text/internal/transcribe"
)

// PionPeerConnection is a webrtc.PeerConnection wrapper that implements the
// PeerConnection interface
type PionPeerConnection struct {
	pc *webrtc.PeerConnection
}

// PionRtcService is our implementation of the rtc.Service
type PionRtcService struct {
	stunServer  string
	transcriber transcribe.Service
}

// NewPionRtcService creates a new instances of PionRtcService
func NewPionRtcService(stun string, transcriber transcribe.Service) Service {
	return &PionRtcService{
		stunServer:  stun,
		transcriber: transcriber,
	}
}

// ProcessOffer handles the SDP offer coming from the client,
// return the SDP answer that must be passed back to stablish the WebRTC
// connection.
func (p *PionPeerConnection) ProcessOffer(offer string) (string, error) {
	err := p.pc.SetRemoteDescription(webrtc.SessionDescription{
		SDP:  offer,
		Type: webrtc.SDPTypeOffer,
	})
	if err != nil {
		return "", err
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	err = p.pc.SetLocalDescription(answer)
	if err != nil {
		return "", err
	}
	return answer.SDP, nil
}

// Close just closes the underlying peer connection
func (p *PionPeerConnection) Close() error {
	return p.pc.Close()
}

func (pi *PionRtcService) handleAudioTrack(pc *webrtc.PeerConnection, track *webrtc.TrackRemote, localTrack *webrtc.TrackLocalStaticRTP, dc *webrtc.DataChannel) error {
	decoder, err := newDecoder()
	if err != nil {
		log.Printf("Error create decoder %v\n", err)
		return err
	}
	trStream, err := pi.transcriber.CreateStream(localTrack)
	if err != nil {
		return err
	}
	defer func() {
		err := trStream.Close()
		if err != nil {
			log.Printf("Error closing stream %v", err)
			return
		}
		for result := range trStream.Results() {
			log.Printf("Result: %v", result)
			msg, err := json.Marshal(result)
			if err != nil {
				continue
			}
			err = dc.Send(msg)
			if err != nil {
				fmt.Printf("DataChannel error: %v", err)
			}
		}
		dc.Close()
	}()

	errs := make(chan error, 2)
	audioStream := make(chan []byte)
	response := make(chan bool)
	timer := time.NewTimer(5 * time.Second)
	go func() {
		for {
			packet, _, err := track.ReadRTP()
			timer.Reset(1 * time.Second)
			if err != nil {
				timer.Stop()
				if err == io.EOF {
					close(audioStream)
					return
				}
				errs <- err
				return
			}
			log.Printf("receive packet %v\n", packet)
			if trStream.NeedDecode() {
				if len(packet.Payload) > 0 {
					audioStream <- packet.Payload
					<-response
				}
			} else {
				packetBytes, err := packet.Marshal()
				if err != nil {
					log.Fatalf("Failed to marshal RTP packet: %v", err)
				}
				audioStream <- packetBytes
				<-response
			}
		}
	}()
	err = nil
	for {
		select {
		case audioChunk := <-audioStream:
			if trStream.NeedDecode() {
				payload, err := decoder.decode(audioChunk)
				if err != nil {
					fmt.Printf("decode error %v\n", err)
					return err
				}
				response <- true
				_, err = trStream.Write(payload)
			} else {
				response <- true
				_, err = trStream.Write(audioChunk)
			}
			if err != nil {
				return err
			}
		case <-timer.C:
			return fmt.Errorf("Read operation timed out")
		case err = <-errs:
			log.Printf("Unexpected error reading track %s: %v", track.ID(), err)
			return err
		}
	}
}

// CreatePeerConnection creates and configures a new peer connection for
// our purposes, receive one audio track and send data through one DataChannel
func (pi *PionRtcService) CreatePeerConnection() (PeerConnection, error) {
	pcconf := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			webrtc.ICEServer{
				URLs: []string{pi.stunServer},
			},
		},
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlanWithFallback,
	}
	pc, err := webrtc.NewPeerConnection(pcconf)
	if err != nil {
		return nil, err
	}

	dataChan := make(chan *webrtc.DataChannel)

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dataChan <- dc
	})

	// Create a new audio track
	localTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if err != nil {
		return nil, err
	}

	// Add the track to the peer connection
	rtpSender, err := pc.AddTrack(localTrack)
	if err != nil {
		return nil, err
	}

	// Handle RTCP packets (for example, to handle feedback from the client)
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

	pc.OnTrack(func(track *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeAudio {
            log.Printf("Received audio track, id = %s, codec = %v\n", track.ID(), track.Codec())
			err := pi.handleAudioTrack(pc, track, localTrack, <-dataChan)
			if err != nil {
				log.Printf("Error reading track (%s): %v\n", track.ID(), err)
			}
		}
	})

	pc.OnICEConnectionStateChange(func(connState webrtc.ICEConnectionState) {
		log.Printf("Connection state: %s \n", connState.String())
	})

	return &PionPeerConnection{
		pc: pc,
	}, nil
}

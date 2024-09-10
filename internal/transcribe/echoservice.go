package transcribe

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// EchoTranscriber is an implementation of the Service interface for echoing audio data back to the client
type EchoTranscriber struct {
	ctx context.Context
}

// CreateStream creates a new Stream for echoing audio data back to the client
func (s *EchoTranscriber) CreateStream(peerConnection *webrtc.PeerConnection) (Stream, error) {
	// Create a new audio track
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if err != nil {
		return nil, err
	}

	// Add the track to the peer connection
	rtpSender, err := peerConnection.AddTrack(track)
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

	return &EchoStream{
		track:   track,
		results: make(chan Result),
	}, nil
}

// EchoStream is an implementation of the Stream interface for echoing audio data back to the client
type EchoStream struct {
	track   *webrtc.TrackLocalStaticRTP
	results chan Result
	mu      sync.Mutex
}

// Write writes audio data to the remote track
func (es *EchoStream) Write(p []byte) (n int, err error) {
	es.mu.Lock()
	defer es.mu.Unlock()

	// Create an RTP packet
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: uint16(time.Now().UnixNano() / int64(time.Millisecond)),
			Timestamp:      uint32(time.Now().UnixNano() / int64(time.Millisecond)),
		},
		Payload: p,
	}

	// Write the RTP packet to the track
	err = es.track.WriteRTP(packet)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close closes the stream and the results channel
func (es *EchoStream) Close() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	close(es.results)
	return nil
}

// Results returns a channel for receiving transcription results
func (es *EchoStream) Results() <-chan Result {
	return es.results
}

func NewEchoService(ctx context.Context) (Service, error) {
	return &EchoTranscriber{
		ctx: ctx,
	}, nil
}

package transcribe

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v2"
)

// EchoTranscriber is an implementation of the Service interface for echoing audio data back to the client
type EchoTranscriber struct {
	ctx context.Context
}

// CreateStream creates a new Stream for echoing audio data back to the client
func (s *EchoTranscriber) CreateStream(peerConnection *webrtc.PeerConnection) (Stream, error) {
	// Create a new audio track
	track, err := peerConnection.NewTrack(webrtc.DefaultPayloadTypeOpus, 1234, "audio", "pion")
	if err != nil {
		return nil, err
	}

	// Add the track to the peer connection
	_, err = peerConnection.AddTrack(track)
	if err != nil {
		return nil, err
	}

	return &EchoStream{
		track:   track,
		results: make(chan Result),
	}, nil
}

// EchoStream is an implementation of the Stream interface for echoing audio data back to the client
type EchoStream struct {
	track   *webrtc.Track
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
			PayloadType:    webrtc.DefaultPayloadTypeOpus,
			SequenceNumber: uint16(time.Now().UnixNano() / int64(time.Millisecond)),
			Timestamp:      uint32(time.Now().UnixNano() / int64(time.Millisecond)),
			SSRC:           es.track.SSRC(),
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

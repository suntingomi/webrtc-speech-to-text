package transcribe

import (
	"context"
	"log"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// EchoTranscriber is an implementation of the Service interface for echoing audio data back to the client
type EchoTranscriber struct {
	ctx context.Context
}

// CreateStream creates a new Stream for echoing audio data back to the client
func (s *EchoTranscriber) CreateStream(track *webrtc.TrackLocalStaticRTP) (Stream, error) {
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

	var packet rtp.Packet
	if err := packet.Unmarshal(p); err != nil {
		log.Fatalf("Failed to unmarshal RTP packet: %v", err)
	}

	// Write the RTP packet to the track
	err = es.track.WriteRTP(&packet)
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

func (es *EchoStream) NeedDecode() bool {
	return false
}

func NewEchoService(ctx context.Context) (Service, error) {
	return &EchoTranscriber{
		ctx: ctx,
	}, nil
}

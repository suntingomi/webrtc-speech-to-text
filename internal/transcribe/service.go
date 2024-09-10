package transcribe

import (
	"io"

	"github.com/pion/webrtc/v3"
)

// Result is the struct used to serialize the results back to the client
type Result struct {
	Text       string  `json:"text"`
	Confidence float32 `json:"confidence"`
	Final      bool    `json:"final"`
}

// Service is an abstract representation of the transcription service
type Service interface {
	CreateStream(peerConnection *webrtc.PeerConnection) (Stream, error)
}

// Stream is an abstract representation of a transcription stream
type Stream interface {
	io.Writer
	io.Closer
	Results() <-chan Result
}

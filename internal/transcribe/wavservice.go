package transcribe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

// WavTranscriber is an implementation of the Service interface for saving WAV files
type WavTranscriber struct {
	ctx context.Context
}

// CreateStream creates a new WAV file and returns a Stream for writing audio data
func (s *WavTranscriber) CreateStream() (Stream, error) {
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d.wav", timestamp)
	filepath := filepath.Join(".", filename)

	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	encoder := wav.NewEncoder(file, 44100, 16, 1, 1)

	return &WavStream{
		file:    file,
		encoder: encoder,
		results: make(chan Result),
	}, nil
}

// WavStream is an implementation of the Stream interface for writing audio data to a WAV file
type WavStream struct {
	file    *os.File
	encoder *wav.Encoder
	results chan Result
	mu      sync.Mutex
}

// Write writes audio data to the WAV file
func (ws *WavStream) Write(p []byte) (n int, err error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	buf := &audio.IntBuffer{Data: make([]int, len(p)/2), Format: &audio.Format{SampleRate: 44100, NumChannels: 1}}
	for i := 0; i < len(p); i += 2 {
		buf.Data[i/2] = int(int16(p[i]) | int16(p[i+1])<<8)
	}

	if err := ws.encoder.Write(buf); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close closes the WAV file and the encoder
func (ws *WavStream) Close() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if err := ws.encoder.Close(); err != nil {
		return err
	}

	if err := ws.file.Close(); err != nil {
		return err
	}

	close(ws.results)
	return nil
}

// Results returns a channel for receiving transcription results
func (ws *WavStream) Results() <-chan Result {
	return ws.results
}

func NewWavService(ctx context.Context) (Service, error) {
	return &WavTranscriber{
		ctx: ctx,
	}, nil
}

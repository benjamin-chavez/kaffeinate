package control

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"kaffeinate/internal/session"
)

const (
	ProtocolVersion = 1
	MaxMessageBytes = 8192
	ExchangeTimeout = 8 * time.Second
)

var (
	ErrAlreadyRunning = errors.New("Kaffeinate is already running")
	ErrUnavailable    = errors.New("Kaffeinate is not running")
)

type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Session session.Request `json:"session"`
}

type Response struct {
	Version int              `json:"version"`
	ID      string           `json:"id"`
	State   session.Snapshot `json:"state"`
	Error   string           `json:"error,omitempty"`
}

func decodeMessage(reader io.Reader, message any) error {
	messageBytes, err := bufio.NewReaderSize(reader, MaxMessageBytes+1).ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("read complete message: %w", err)
	}
	if len(messageBytes) > MaxMessageBytes {
		return errors.New("control message is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(messageBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(message); err != nil {
		return fmt.Errorf("decode control message: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected one JSON message")
	}
	return nil
}

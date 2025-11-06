package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MessageType represents the type of protocol message
type MessageType byte

// Client → Server message types
const (
	MsgStatus     MessageType = 0x01
	MsgStdin      MessageType = 0x02
	MsgSignal     MessageType = 0x03
	MsgResize     MessageType = 0x04
	MsgAttach     MessageType = 0x05
	MsgDetach     MessageType = 0x06
	MsgCloseStdin MessageType = 0x07
	MsgWait       MessageType = 0x08
	MsgShutdown   MessageType = 0x10
)

// Server → Client message types
const (
	MsgStatusResponse MessageType = 0x80
	MsgOutput         MessageType = 0x81
	MsgSignalResponse MessageType = 0x82
	MsgResizeResponse MessageType = 0x83
	MsgWaitResponse   MessageType = 0x88
	MsgError          MessageType = 0x8F
	MsgProcessExit    MessageType = 0x90
)

// Stream identifiers for output
const (
	StreamStdout byte = 0x01
	StreamStderr byte = 0x02
	StreamBoth   byte = 0x03
)

// Wait types
const (
	WaitTypeExit       byte = 0x00 // Wait for process to exit
	WaitTypeForeground byte = 0x01 // Wait for foreground control (VTY only)
)

// Wait result status
const (
	WaitStatusCompleted     byte = 0x00 // Wait condition met
	WaitStatusTimeout       byte = 0x01 // Timeout occurred
	WaitStatusNotApplicable byte = 0x02 // Wait type not applicable (e.g., foreground wait on non-VTY)
)

// Message represents a protocol message
type Message struct {
	Type    MessageType
	Payload []byte
}

// StatusResponse contains process status information
type StatusResponse struct {
	PID       int      `json:"pid"`
	Running   bool     `json:"running"`
	ExitCode  *int     `json:"exit_code"`
	StartedAt string   `json:"started_at"`
	EndedAt   *string  `json:"ended_at,omitempty"`
	Command   []string `json:"command"`
	HasVTY    bool     `json:"has_vty"`
}

// ReadMessage reads a message from the reader
func ReadMessage(r io.Reader) (*Message, error) {
	// Read length (4 bytes, big-endian)
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	// Sanity check on length (max 10MB)
	if length < 1 || length > 10*1024*1024 {
		return nil, fmt.Errorf("invalid message length: %d", length)
	}

	// Read message type (1 byte)
	var msgType MessageType
	if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
		return nil, fmt.Errorf("failed to read message type: %w", err)
	}

	// Read payload (length - 1 bytes, since we already read the type)
	payloadLen := length - 1
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("failed to read payload: %w", err)
		}
	}

	return &Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

// WriteMessage writes a message to the writer
func WriteMessage(w io.Writer, msgType MessageType, payload []byte) error {
	// Calculate total length (type + payload)
	length := uint32(1 + len(payload))

	// Write length
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write message type
	if err := binary.Write(w, binary.BigEndian, msgType); err != nil {
		return fmt.Errorf("failed to write message type: %w", err)
	}

	// Write payload
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("failed to write payload: %w", err)
		}
	}

	return nil
}

// WriteError writes an error message
func WriteError(w io.Writer, err error) error {
	return WriteMessage(w, MsgError, []byte(err.Error()))
}

// WriteStatusResponse writes a status response message
func WriteStatusResponse(w io.Writer, status *StatusResponse) error {
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}
	return WriteMessage(w, MsgStatusResponse, data)
}

// WriteOutput writes an output message
func WriteOutput(w io.Writer, stream byte, data []byte) error {
	payload := append([]byte{stream}, data...)
	return WriteMessage(w, MsgOutput, payload)
}

// WriteProcessExit writes a process exit message
func WriteProcessExit(w io.Writer, exitCode int) error {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(exitCode))
	return WriteMessage(w, MsgProcessExit, payload)
}

// ParseStatusResponse parses a status response payload
func ParseStatusResponse(payload []byte) (*StatusResponse, error) {
	var status StatusResponse
	if err := json.Unmarshal(payload, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}
	return &status, nil
}

// ParseOutput parses an output message payload
func ParseOutput(payload []byte) (stream byte, data []byte, err error) {
	if len(payload) < 1 {
		return 0, nil, fmt.Errorf("output payload too short")
	}
	return payload[0], payload[1:], nil
}

// ParseProcessExit parses a process exit payload
func ParseProcessExit(payload []byte) (int, error) {
	if len(payload) != 4 {
		return 0, fmt.Errorf("invalid process exit payload length")
	}
	exitCode := int(binary.BigEndian.Uint32(payload))
	return exitCode, nil
}

// WriteWaitResponse writes a wait response message
func WriteWaitResponse(w io.Writer, status byte) error {
	return WriteMessage(w, MsgWaitResponse, []byte{status})
}

// ParseWait parses a wait message payload
func ParseWait(payload []byte) (timeoutSecs uint32, waitType byte, err error) {
	if len(payload) != 5 {
		return 0, 0, fmt.Errorf("invalid wait payload length: expected 5, got %d", len(payload))
	}
	timeoutSecs = binary.BigEndian.Uint32(payload[0:4])
	waitType = payload[4]
	return timeoutSecs, waitType, nil
}

// ParseWaitResponse parses a wait response payload
func ParseWaitResponse(payload []byte) (byte, error) {
	if len(payload) != 1 {
		return 0, fmt.Errorf("invalid wait response payload length")
	}
	return payload[0], nil
}

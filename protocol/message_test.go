package protocol

import (
	"bytes"
	"testing"
)

func TestReadWriteMessage(t *testing.T) {
	tests := []struct {
		name    string
		msgType MessageType
		payload []byte
	}{
		{
			name:    "empty payload",
			msgType: MsgStatus,
			payload: []byte{},
		},
		{
			name:    "small payload",
			msgType: MsgStdin,
			payload: []byte("hello world"),
		},
		{
			name:    "binary payload",
			msgType: MsgStdin,
			payload: []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
		},
		{
			name:    "large payload",
			msgType: MsgOutput,
			payload: bytes.Repeat([]byte("test"), 10000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Write message
			if err := WriteMessage(&buf, tt.msgType, tt.payload); err != nil {
				t.Fatalf("WriteMessage failed: %v", err)
			}

			// Read message
			msg, err := ReadMessage(&buf)
			if err != nil {
				t.Fatalf("ReadMessage failed: %v", err)
			}

			// Check message type
			if msg.Type != tt.msgType {
				t.Errorf("expected type %d, got %d", tt.msgType, msg.Type)
			}

			// Check payload
			if !bytes.Equal(msg.Payload, tt.payload) {
				t.Errorf("payload mismatch: expected %v, got %v", tt.payload, msg.Payload)
			}
		})
	}
}

func TestReadMessageErrors(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		error string
	}{
		{
			name:  "empty buffer",
			data:  []byte{},
			error: "failed to read message length",
		},
		{
			name:  "incomplete length",
			data:  []byte{0x00, 0x00},
			error: "failed to read message length",
		},
		{
			name:  "zero length",
			data:  []byte{0x00, 0x00, 0x00, 0x00},
			error: "invalid message length",
		},
		{
			name:  "incomplete message",
			data:  []byte{0x00, 0x00, 0x00, 0x05, 0x01},
			error: "failed to read payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(tt.data)
			_, err := ReadMessage(buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestStatusResponse(t *testing.T) {
	var buf bytes.Buffer

	status := &StatusResponse{
		PID:       12345,
		Running:   true,
		ExitCode:  nil,
		StartedAt: "2025-01-01T00:00:00Z",
		EndedAt:   nil,
		Command:   []string{"/bin/bash", "-c", "echo test"},
		HasVTY:    false,
	}

	// Write status response
	if err := WriteStatusResponse(&buf, status); err != nil {
		t.Fatalf("WriteStatusResponse failed: %v", err)
	}

	// Read message
	msg, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if msg.Type != MsgStatusResponse {
		t.Errorf("expected type %d, got %d", MsgStatusResponse, msg.Type)
	}

	// Parse status response
	parsedStatus, err := ParseStatusResponse(msg.Payload)
	if err != nil {
		t.Fatalf("ParseStatusResponse failed: %v", err)
	}

	if parsedStatus.PID != status.PID {
		t.Errorf("PID mismatch: expected %d, got %d", status.PID, parsedStatus.PID)
	}

	if parsedStatus.Running != status.Running {
		t.Errorf("Running mismatch: expected %v, got %v", status.Running, parsedStatus.Running)
	}
}

func TestOutput(t *testing.T) {
	var buf bytes.Buffer

	testData := []byte("test output data\x00\xFF")
	if err := WriteOutput(&buf, StreamStdout, testData); err != nil {
		t.Fatalf("WriteOutput failed: %v", err)
	}

	msg, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if msg.Type != MsgOutput {
		t.Errorf("expected type %d, got %d", MsgOutput, msg.Type)
	}

	stream, data, err := ParseOutput(msg.Payload)
	if err != nil {
		t.Fatalf("ParseOutput failed: %v", err)
	}

	if stream != StreamStdout {
		t.Errorf("stream mismatch: expected %d, got %d", StreamStdout, stream)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("data mismatch: expected %v, got %v", testData, data)
	}
}

func TestProcessExit(t *testing.T) {
	var buf bytes.Buffer

	exitCode := 42
	if err := WriteProcessExit(&buf, exitCode); err != nil {
		t.Fatalf("WriteProcessExit failed: %v", err)
	}

	msg, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if msg.Type != MsgProcessExit {
		t.Errorf("expected type %d, got %d", MsgProcessExit, msg.Type)
	}

	parsedExitCode, err := ParseProcessExit(msg.Payload)
	if err != nil {
		t.Fatalf("ParseProcessExit failed: %v", err)
	}

	if parsedExitCode != exitCode {
		t.Errorf("exit code mismatch: expected %d, got %d", exitCode, parsedExitCode)
	}
}

func TestBinarySafety(t *testing.T) {
	// Test that binary data with null bytes and special characters is preserved
	binaryData := []byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
		0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8,
	}

	var buf bytes.Buffer

	if err := WriteMessage(&buf, MsgStdin, binaryData); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	msg, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	if !bytes.Equal(msg.Payload, binaryData) {
		t.Errorf("binary data not preserved: expected %v, got %v", binaryData, msg.Payload)
	}
}

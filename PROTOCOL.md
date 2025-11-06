# bgrun Socket Protocol

## Overview
The control socket uses a binary-safe, length-prefixed protocol for all communication.

## Message Format

All messages follow this structure:

```
[4 bytes: length (uint32, big-endian)]
[1 byte: message type]
[length-5 bytes: payload]
```

## Message Types

### Client → Server

- `0x01` STATUS - Get process status
- `0x02` STDIN - Write data to stdin (payload: binary data)
- `0x03` SIGNAL - Send signal to process (payload: 1 byte signal number)
- `0x04` RESIZE - Resize VTY (payload: 4 bytes: uint16 rows big-endian, uint16 cols big-endian)
- `0x05` ATTACH - Attach to output stream (payload: 1 byte stream selector: 0x01=stdout, 0x02=stderr, 0x03=both)
- `0x06` DETACH - Stop receiving output
- `0x07` CLOSE_STDIN - Close stdin pipe
- `0x10` SHUTDOWN - Stop bgrun daemon

### Server → Client

- `0x80` STATUS_RESPONSE - Process status info
  - Payload: JSON object with status information
- `0x81` OUTPUT - Output from stdout/stderr
  - First byte: stream identifier (0x01=stdout, 0x02=stderr)
  - Remaining bytes: output data
- `0x82` SIGNAL_RESPONSE - Signal sent acknowledgment
- `0x83` RESIZE_RESPONSE - Resize acknowledgment
- `0x8F` ERROR - Error response
  - Payload: UTF-8 error message
- `0x90` PROCESS_EXIT - Process has exited
  - Payload: 4 bytes exit code (int32, big-endian)

## Status Response Format

The STATUS_RESPONSE message contains a JSON object:

```json
{
  "pid": 12345,
  "running": true,
  "exit_code": null,
  "started_at": "2025-01-01T00:00:00Z",
  "ended_at": null,
  "command": ["/bin/bash", "-c", "sleep 100"],
  "has_vty": false
}
```

## Example Flow

1. Client connects to control.sock
2. Client sends STATUS (0x01) to check if process is running
3. Server responds with STATUS_RESPONSE (0x80)
4. Client sends ATTACH (0x05) to start receiving output
5. Server streams OUTPUT (0x81) messages as data arrives
6. Client sends STDIN (0x02) to send input to process
7. Process exits
8. Server sends PROCESS_EXIT (0x90) with exit code
9. Client disconnects

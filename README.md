# bgrun - Background Process Runner

A lightweight background process manager with a clean binary-safe socket API for process control and I/O streaming.

## Features

- **Background Process Management**: Run any command in the background with full control
- **Binary-Safe Socket API**: Length-prefixed protocol supporting binary data
- **Flexible I/O Handling**: Configure stdin, stdout, and stderr independently
- **Output Streaming**: Attach/detach from process output at any time
- **Multi-Client Support**: Multiple clients can connect to the same process
- **Process Control**: Send signals, manage stdin, check status
- **Automatic Runtime Directory**: Uses `$XDG_RUNTIME_DIR` or `/tmp/.bgrun-<uid>`

## Installation

```bash
go build -o bgrun .
go build -o bgctl ./cmd/bgctl/
```

## Quick Start

### Starting a Background Process

```bash
# Run a simple command
./bgrun sleep 100

# Run with stdin streaming enabled
./bgrun -stdin stream bash

# Run in VTY mode (for interactive programs)
./bgrun -vty -stdin stream vim myfile.txt

# Custom I/O configuration
./bgrun -stdout /tmp/myapp.log -stderr /tmp/myapp.err myapp
```

After starting, bgrun will print the runtime directory and control socket path:

```
Process started successfully
Runtime directory: /run/user/1000/12345
Control socket: /run/user/1000/12345/control.sock
```

### Connecting to a Running Process

Use `bgctl` to interact with a running process:

```bash
# Check process status
./bgctl -socket /run/user/1000/12345/control.sock status

# Attach to process output (stdout/stderr)
./bgctl -socket /run/user/1000/12345/control.sock attach

# Send a signal to the process
./bgctl -socket /run/user/1000/12345/control.sock signal 15  # SIGTERM

# Shutdown the daemon
./bgctl -socket /run/user/1000/12345/control.sock shutdown
```

## Command Line Options

### bgrun

```
bgrun [options] <command> [args...]

Options:
  -stdin <mode>   stdin mode: null, stream, or file path (default: null)
  -stdout <mode>  stdout mode: null, log, or file path (default: log)
  -stderr <mode>  stderr mode: null, log, or file path (default: log)
  -vty            run in VTY mode (for interactive programs)
  -help           show help message
```

#### I/O Modes

- **null**: Redirect to /dev/null
- **stream**: Stream through socket (stdin only)
- **log**: Write to `output.log` in runtime directory (stdout/stderr only)
- **<filepath>**: Read from or write to specified file

### bgctl

```
bgctl -socket <path> <command> [args...]

Commands:
  status              Show process status
  attach              Attach to process output
  signal <signum>     Send signal to process
  shutdown            Shutdown the daemon
```

## Socket Protocol

The control socket uses a binary-safe, length-prefixed protocol. See [PROTOCOL.md](PROTOCOL.md) for full details.

### Message Format

```
[4 bytes: length (uint32, big-endian)]
[1 byte: message type]
[length-5 bytes: payload]
```

### Example: Checking Process Status

```go
import "github.com/KarpelesLab/bgrun/client"

c, err := client.Connect("/run/user/1000/12345/control.sock")
if err != nil {
    log.Fatal(err)
}
defer c.Close()

status, err := c.GetStatus()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("PID: %d, Running: %v\n", status.PID, status.Running)
```

### Example: Streaming Output

```go
c, err := client.Connect("/run/user/1000/12345/control.sock")
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// Attach to stdout and stderr
if err := c.Attach(protocol.StreamBoth); err != nil {
    log.Fatal(err)
}

// Read messages
err = c.ReadMessages(
    func(stream byte, data []byte) error {
        if stream == protocol.StreamStderr {
            os.Stderr.Write(data)
        } else {
            os.Stdout.Write(data)
        }
        return nil
    },
    func(exitCode int) {
        fmt.Printf("Process exited with code %d\n", exitCode)
    },
)
```

### Example: Writing to stdin

```go
c, err := client.Connect("/run/user/1000/12345/control.sock")
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// Write data to stdin
if err := c.WriteStdin([]byte("hello\n")); err != nil {
    log.Fatal(err)
}

// Close stdin when done
if err := c.CloseStdin(); err != nil {
    log.Fatal(err)
}
```

## Use Cases

### Running a Database Server

```bash
./bgrun -stdout /var/log/postgres.log -stderr /var/log/postgres.err \
    postgres -D /var/lib/postgresql/data
```

### Interactive Shell Session

```bash
./bgrun -vty -stdin stream -stdout log -stderr log bash

# In another terminal:
./bgctl -socket /run/user/1000/<pid>/control.sock attach
```

### Long-Running Build Process

```bash
./bgrun -stdout /tmp/build.log make all

# Monitor progress from another terminal:
./bgctl -socket /run/user/1000/<pid>/control.sock attach
```

### Background Script with Input

```bash
./bgrun -stdin stream python3 process_data.py

# Send data from another process:
echo "data" | nc -U /run/user/1000/<pid>/control.sock
```

## Runtime Directory Structure

```
$XDG_RUNTIME_DIR/<pid>/
├── control.sock    # Unix socket for control API
└── output.log      # Process output (when using 'log' mode)
```

Or if `$XDG_RUNTIME_DIR` is not set:

```
/tmp/.bgrun-<uid>/<pid>/
├── control.sock
└── output.log
```

## Client Library

The Go client library provides a simple API for interacting with bgrun processes:

```go
import "github.com/KarpelesLab/bgrun/client"
```

### API Methods

- `Connect(socketPath string) (*Client, error)` - Connect to daemon
- `GetStatus() (*StatusResponse, error)` - Get process status
- `WriteStdin(data []byte) error` - Write to stdin
- `CloseStdin() error` - Close stdin pipe
- `SendSignal(sig syscall.Signal) error` - Send signal
- `Attach(streams byte) error` - Attach to output streams
- `Detach() error` - Detach from output
- `Shutdown() error` - Shutdown daemon
- `ReadMessages(outputHandler, exitHandler) error` - Read output/events

## Testing

```bash
# Run all tests
go test -v ./...

# Run protocol tests
go test -v ./protocol/

# Run daemon tests
go test -v ./daemon/

# Run integration tests
go test -v . -run Integration
```

## VTY Support

VTY (virtual terminal) support is planned for interactive programs that require terminal control.

```bash
# Coming soon
./bgrun -vty -stdin stream vim myfile.txt
```

## Security

- Socket files are created with 0600 permissions (owner read/write only)
- Runtime directories are created with 0700 permissions
- All data transmission is binary-safe
- No authentication is built-in (relies on filesystem permissions)

## License

See LICENSE file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

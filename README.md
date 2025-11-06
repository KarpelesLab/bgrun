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
```

## Quick Start

### Starting a Background Process

```bash
# Run in foreground (shows runtime info)
./bgrun sleep 100

# Run in background (returns daemon PID for control)
PID=$(./bgrun -background sleep 100)
echo "Background process PID: $PID"

# Run with stdin streaming enabled
./bgrun -stdin stream bash

# Run in VTY mode (for interactive programs)
./bgrun -vty vim myfile.txt

# Custom I/O configuration
./bgrun -stdout /tmp/myapp.log -stderr /tmp/myapp.err myapp
```

When starting in foreground mode, bgrun prints the runtime directory and control socket path:

```
Process started successfully
Runtime directory: /run/user/1000/12345
Control socket: /run/user/1000/12345/control.sock
```

When using `-background`, bgrun outputs only the daemon PID, which you can use with `-ctl` commands.

### Connecting to a Running Process

Use `bgrun -ctl` to interact with a running process:

```bash
# Check process status (using PID)
./bgrun -ctl -pid 12345 status

# Attach to process output (stdout/stderr)
./bgrun -ctl -pid 12345 attach

# Wait for process to exit (with 30 second timeout)
./bgrun -ctl -pid 12345 wait exit 30

# Wait for foreground control to return (VTY mode)
./bgrun -ctl -pid 12345 wait foreground 60

# Send a signal to the process
./bgrun -ctl -pid 12345 signal 15  # SIGTERM

# Shutdown the daemon
./bgrun -ctl -pid 12345 shutdown
```

The PID is the daemon process ID printed by bgrun (or captured with `-background`).

## Command Line Options

### Daemon Mode

```
bgrun [options] <command> [args...]

Options:
  -stdin <mode>   stdin mode: null, stream, or file path (default: null)
  -stdout <mode>  stdout mode: null, log, or file path (default: log)
  -stderr <mode>  stderr mode: null, log, or file path (default: log)
  -vty            run in VTY mode (for interactive programs)
  -background     run daemon in background (outputs PID)
  -help           show help message
```

#### I/O Modes

- **null**: Redirect to /dev/null
- **stream**: Stream through socket (stdin only)
- **log**: Write to `output.log` in runtime directory (stdout/stderr only)
- **<filepath>**: Read from or write to specified file

### Control Mode

```
bgrun -ctl -pid <daemon-pid> <command> [args...]

Commands:
  status                       Show process status
  attach                       Attach to process output
  wait <exit|foreground> <sec> Wait for condition with timeout
  signal <signum>              Send signal to process
  shutdown                     Shutdown the daemon
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
PID=$(./bgrun -background -stdout /var/log/postgres.log -stderr /var/log/postgres.err \
    postgres -D /var/lib/postgresql/data)
echo "PostgreSQL daemon PID: $PID"

# Check status
./bgrun -ctl -pid $PID status
```

### Interactive Shell Session

```bash
PID=$(./bgrun -background -vty bash)

# In another terminal, attach interactively:
./bgrun -ctl -pid $PID attach
```

### Editing Files Remotely

```bash
# Start vim in background
PID=$(./bgrun -background -vty vim /path/to/file.txt)

# Attach from anywhere (even over SSH)
./bgrun -ctl -pid $PID attach
```

### Long-Running Build Process

```bash
PID=$(./bgrun -background -stdout /tmp/build.log make all)

# Monitor progress from another terminal:
./bgrun -ctl -pid $PID attach

# Wait for build to complete
./bgrun -ctl -pid $PID wait exit 3600  # 1 hour timeout
```

### Background Script with Input

```bash
PID=$(./bgrun -background -stdin stream python3 process_data.py)

# Send data from another process:
echo "data" | nc -U /run/user/1000/$PID/control.sock
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

[![Go Reference](https://pkg.go.dev/badge/github.com/KarpelesLab/bgrun/client.svg)](https://pkg.go.dev/github.com/KarpelesLab/bgrun/client)

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
- `Wait(timeoutSecs uint32, waitType byte) (byte, error)` - Wait for process exit or foreground
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

VTY (virtual terminal) support is fully implemented for interactive programs that require terminal control.

### Running Interactive Programs

```bash
# Start vim in VTY mode
./bgrun -vty vim myfile.txt

# Start an interactive bash session
./bgrun -vty bash

# Start any interactive program
./bgrun -vty htop
```

### Attaching to Interactive Sessions

When you attach to a VTY-enabled process, `bgrun -ctl` automatically detects it and provides full interactive terminal support:

```bash
# Attach interactively (automatic raw mode, resize handling)
./bgrun -ctl -pid <daemon-pid> attach

# Your terminal will be in raw mode and fully interactive
# Terminal resize events are automatically forwarded to the process
# Press Ctrl+C to detach (or the program will exit normally)
```

### Features

- **Automatic PTY allocation**: Programs run with a pseudo-terminal
- **Terminal size detection**: Initial terminal size is set correctly
- **Resize handling**: SIGWINCH signals automatically resize the remote PTY
- **Raw mode**: Client terminal switches to raw mode for full interactivity
- **Bidirectional I/O**: Full stdin/stdout streaming with binary safety
- **Multiple attach**: Multiple clients can attach to view output (one active controller)

## Security

- Socket files are created with 0600 permissions (owner read/write only)
- Runtime directories are created with 0700 permissions
- All data transmission is binary-safe
- No authentication is built-in (relies on filesystem permissions)

## License

See LICENSE file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

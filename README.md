# claude-clip-bridge

Paste clipboard images from your MacBook into Claude Code running on a remote Linux server over SSH.

## Background

Terminals only handle text streams. The clipboard is managed by the OS GUI server (WindowServer on macOS, X11/Wayland on Linux), and SSH does not bridge this channel.

Text copy-paste works because terminal emulators render text locally — paste just sends keystrokes through the SSH tunnel. Images cannot travel this way.

### Why existing solutions fall short

- **OSC 52**: Standard for text clipboard over SSH. No MIME types, so no images.
- **OSC 5522**: Kitty's extension with MIME support. Not implemented in Ghostty (parsed only as of 1.3.0).

### How Claude Code reads the clipboard

```
Ctrl+V
  1. OSC 52 request to terminal (text only)
  2. fallback → xclip -selection clipboard -t image/png -o
  3. fallback → wl-paste
```

Headless servers have neither `$DISPLAY` nor `$WAYLAND_DISPLAY`, so steps 2 and 3 always fail. The core idea: **intercept step 2 with an xclip shim.**

---

## How It Works

### On SSH connect

```
ssh myserver
  │
  ├─ LocalCommand: clip-bridge-start <hostname> $PPID
  │     └─ starts clip-bridge in background (TCP listen on 127.0.0.1:19876)
  │
  └─ RemoteForward: server:127.0.0.1:19876 ──SSH tunnel──→ mac:127.0.0.1:19876
```

### On Ctrl+V in Claude Code

```
Claude Code
  │  Ctrl+V
  ▼
xclip -selection clipboard -t TARGETS -o
  └─ shim: bridge reachable? → respond "TARGETS\nimage/png\n"

Claude Code sees image/png available
  ▼
xclip -selection clipboard -t image/png -o
  └─ shim reads token from ~/.config/claude-clip-bridge/token
  └─ connects to 127.0.0.1:19876
  └─ sends: [0x01 request type | 32-byte token]
       │
       └──SSH tunnel──→ clip-bridge on Mac
                            │ validates token
                            │ runs: pngpaste -
                            │ reads PNG from stdout
                            ▼
       ◀──SSH tunnel── [4-byte length | PNG bytes]
  └─ shim writes PNG to stdout → Claude Code receives image
```

### On SSH disconnect

`clip-bridge` polls `kill -0 <ssh_pid>` every 5 seconds. When the SSH process exits, `clip-bridge` shuts down automatically.

---

## Components

| Binary | Location | Role |
|--------|----------|------|
| `clip-bridge` | Mac `~/bin/` | TCP server, token validation, runs `pngpaste`, monitors SSH PID |
| `clip-bridge-start` | Mac `~/bin/` | Forks `clip-bridge`, polls for TCP readiness, returns immediately |
| `xclip` (shim) | Server `~/.local/bin/` | Intercepts image requests, proxies to bridge, falls through otherwise |

### Protocol

```
Client (shim) → Server (bridge):   1 byte request type + 32 bytes token
  0x01 = PNG image request

Server (bridge) → Client (shim):
  valid token:   4-byte big-endian length + PNG bytes  (length=0 means empty clipboard)
  invalid token: connection closed immediately, no response
```

### xclip shim routing

```
xclip args
  ├─ image read? (-selection clipboard -o -t image/*)
  │     ├─ -t TARGETS    → if bridge reachable: respond locally with "TARGETS\nimage/png\n"
  │     ├─ -t image/png  → connect to bridge, send request, write PNG to stdout
  │     └─ other image/* → passthrough to /usr/bin/xclip
  └─ anything else       → passthrough to /usr/bin/xclip
```

If the bridge is unreachable or the token file is missing, the shim falls through to `/usr/bin/xclip` transparently. The shim is a no-op on non-SSH sessions.

---

## Installation

### Prerequisites

```sh
brew install pngpaste
```

The target host must already have an alias in `~/.ssh/config`.

### Quick install

```sh
./install.sh --host myserver
```

Multiple hosts at once:

```sh
./install.sh --host myserver --host staging
```

What `install.sh` does:

1. Builds `clip-bridge` and `clip-bridge-start` for macOS
2. Cross-compiles the `xclip` shim for Linux (amd64 by default)
3. Generates a 32-byte random token, stores it at `~/.config/claude-clip-bridge/token`
4. Deploys the xclip shim and token to each host via `scp`
5. Writes a managed SSH config fragment per host
6. Adds `Include ~/.config/claude-clip-bridge/hosts/*.conf` to `~/.ssh/config`

After install, plain `ssh myserver` is all you need.

### Manual SSH config

If you prefer not to use the install script, add this to `~/.ssh/config`:

```sshconfig
Host myserver
    PermitLocalCommand yes
    LocalCommand clip-bridge-start %h $PPID
    RemoteForward 19876 localhost:19876
    ExitOnForwardFailure yes
```

And copy the xclip shim and token to the server manually:

```sh
go build -o ~/bin/clip-bridge ./cmd/clip-bridge
go build -o ~/bin/clip-bridge-start ./cmd/clip-bridge-start
GOOS=linux GOARCH=amd64 go build -o xclip ./cmd/xclip

openssl rand -hex 32 > ~/.config/claude-clip-bridge/token
scp xclip myserver:~/.local/bin/xclip
scp ~/.config/claude-clip-bridge/token myserver:~/.config/claude-clip-bridge/token
ssh myserver 'chmod +x ~/.local/bin/xclip && chmod 600 ~/.config/claude-clip-bridge/token'
```

### Options

```
--host <name>          Target SSH host alias (repeatable)
--port <port>          Bridge TCP port (default: 19876)
--local-bin-dir <path> Local binary install directory (default: ~/bin)
--remote-bin-dir <path> Remote binary directory (default: .local/bin)
--remote-goarch <arch> Target architecture (default: amd64)
--manual-ssh-config    Print Include line instead of editing ~/.ssh/config
--dry-run              Show planned actions without executing
```

---

## Usage

1. Copy an image on your Mac (`Cmd+Shift+Ctrl+4` for screenshot to clipboard, or `Cmd+C` on any image)
2. `ssh myserver`
3. Open Claude Code and press `Ctrl+V`

### Verify the bridge is working

```sh
# Check that the shim advertises image/png
xclip -selection clipboard -t TARGETS -o

# Read image from Mac clipboard (should produce a valid PNG)
xclip -selection clipboard -t image/png -o > /tmp/out.png
file /tmp/out.png

# Debug shim behavior
CC_CLIP_DEBUG=1 xclip -selection clipboard -t image/png -o > /tmp/out.png
```

### Check Mac-side logs

```sh
tail -f ~/Library/Logs/claude-clip-bridge/<hostname>.log
```

---

## Security

| Concern | Design |
|---------|--------|
| Network exposure | localhost-only TCP port, exists only during SSH session |
| Cross-user access | 32-byte random token required; wrong token → silent close |
| Lingering daemon | SSH PID monitoring, auto-exit on disconnect |
| Multiple sessions | Same port — second session's `ExitOnForwardFailure` drops the connection |
| PATH pollution | Shim falls through to `/usr/bin/xclip` when bridge is absent |
| Data persistence | Image is never written to disk; passed directly through stdout |
| Transport | Inside SSH tunnel |

> **Note on multi-session:** Opening two SSH sessions to the same server simultaneously will fail for the second connection because the forwarded port is already in use. Single-session-per-server is the intended use case.

---

## Limitations

- macOS only (Mac side)
- `image/png` only (v1)
- Single concurrent session per server
- Requires `pngpaste` on the Mac
- Requires `AllowTcpForwarding yes` on the server (default on most servers; `AllowStreamLocalForwarding` is not required)

---

## Project Structure

```
cmd/
  clip-bridge/        Mac-side TCP server entry point
  clip-bridge-start/  Launcher entry point
  xclip/              Linux xclip shim entry point

internal/
  bridge/
    server.go         TCP server, connection handler, token validation
    protocol.go       Wire protocol (request/response encoding)
    clipboard.go      pngpaste invocation and error handling
    monitor.go        SSH process liveness polling
    sanitize.go       Hostname sanitization for log file names
    token.go          Token file reading
  launcher/
    launcher.go       clip-bridge subprocess management, TCP readiness polling
  xclip/
    shim.go           Main shim logic and routing
    matcher.go        xclip argument parsing
    discovery.go      TCP bridge discovery
    passthrough.go    Real xclip fallback
    dial_linux.go     Linux TCP dialer with TCP_NODELAY fallback
    dial_default.go   Default TCP dialer (non-Linux)
  testutil/
    testutil.go       Test helpers
```

# sshtui

SSH session manager with persistent sessions and scrollback.

## Features

- Multiple SSH sessions in parallel
- Detach/resume with Ctrl+Space
- 1MB scrollback buffer per session (searchable)
- Parses `~/.ssh/config`
- One dependency: `creack/pty`

## Install

```bash
go build -o sshtui
```

## Usage

```bash
./sshtui
```

**Menu:**
- `[1]` - Connect to host #1
- `[!1]` - Resume session #1
- `v` - View scrollback
- `m` - Multi-host command
- `f` - Port forward info
- `x` - Close session
- `q` - Quit

**In session:**
- `Ctrl+Space` - Detach

**Scrollback viewer:**
- `/term` - Search
- `n/N` - Next/prev match
- `j/k` - Scroll
- `g/G` - Top/bottom
- `q` - Quit

**Multi-host:**
- Select hosts with checkbox
- Execute command on multiple hosts
- Live streaming or collected results

**Port forwarding:**
- Configure in `~/.ssh/config`
- LocalForward, RemoteForward, DynamicForward supported
- Automatically applied to sessions

## SSH Agent

Multi-host commands require ssh-agent for passphrase-protected keys:

```bash
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/id_rsa
```

## Security

Input (passwords, passphrases) goes directly to SSH - never logged. Only output is captured.

## License

GNU General Public License v3.0

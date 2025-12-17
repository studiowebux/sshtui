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
- `x` - Close session
- `q` - Quit

**In session:**
- `Ctrl+Space` - Detach

**Scrollback viewer:**
- `/term` - Search
- `n/N` - Next/prev match
- `j/k` - Scroll
- `q` - Quit

## Security

Input (passwords) goes directly to SSH - never logged. Only output is captured.

## License

GNU General Public License v3.0

# sync

P2P sync that just works. No cloud, no central server. Just the devices talking to each other. It uses consensus to decide filenames.

## What It Does

sync keeps folders consistent across multiple machines on the same network. When devices disagree on a filename for the same file content, the network resolves it through majority voting. If two out of three devices call a file `report.pdf`, that becomes the agreed-upon name. Ties are flagged for manual resolution.

## Quick Start

```
go build -o sync ./cmd/sync
./sync --folder ./documents --port 9000
```

Devices on the same LAN discover each other automatically via UDP broadcast. No configuration needed.

## Commands

| Command | Description |
|---|---|
| `status` | Scan folder, sync state with peers, display the network-wide file list |
| `sync all` | Download every file you're missing |
| `sync [n]` | Download a specific file by its index number |
| `rename [n] [name]` | Rename a file locally and broadcast the new name as a network vote |
| `vote [n] [name]` | Cast a name vote without renaming locally |
| `exit` | Shut down |

## Architecture

```
cmd/sync/main.go            CLI and application entry point
internal/
  indexer/indexer.go         Parallel file scanner with SHA256 hashing
  logger/logger.go           ANSI-colored terminal output
  network/
    protocol.go              Wire format and TLS certificate generation
    discovery.go             UDP peer discovery
    peer.go                  Connection management and file transfer
    state.go                 Consensus engine
```

### Key Design Decisions

**Full-mesh topology** — Every device maintains a direct TLS connection to every other device. Index updates propagate immediately across the entire network.

**Hash-based identity** — Files are identified by their SHA256 hash, not their name. Two files with different names but identical content are recognized as the same file. This is what enables consensus — the network votes on which name a given hash should carry.

**Parallel scanning** — The indexer spawns one goroutine per CPU core. File metadata (size, modification time) is cached so unchanged files skip re-hashing entirely.

**JSON-over-TLS protocol** — Messages are newline-delimited JSON over persistent TLS connections. File transfers embed binary data directly after a JSON header containing the expected size and hash for verification.

## Requirements

- Go 1.27+
- LAN with UDP broadcast support (port 9999 for discovery, configurable port for sync)

## How Consensus Works

1. Each device shares its file index on connect
2. For each unique file hash, the system counts how many devices use each name
3. The name with the most votes wins — marked as `Consensus` in the status view
4. Equal vote counts are marked as `TIE!` and can be resolved with `rename` or `vote`
5. Votes are broadcast instantly — all devices see the resolution in real time

Built entirely with the Go standard library. Zero external dependencies.

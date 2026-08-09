# Eugen

Eugen is an open-source **starboard bot** for Discord, written in Go using `bwmarrin/discordgo`. Users react to messages with a star emote (⭐ by default) and, once a message reaches a configurable threshold, Eugen reposts it to a dedicated starboard channel as a nicely formatted embed.

Server configuration and starboard records are persisted in MongoDB.

## Features

- **Starboard** — react with ⭐ and Eugen copies the message into the configured starboard channel once it hits the minimum star count. Each guild can customize:
  - the starboard channel,
  - the star emote (must be a guild emoji),
  - the minimum star threshold,
  - whether the author's own reaction counts (`selfstar`),
  - whether bot reactions are ignored (`ignorebots`),
  - the embed color.
- **Per-channel star requirements** — override the server-wide threshold for specific channels.
- **Banned channels** — prevent any of a list of channels from ever reaching the starboard.
- **Blacklisted users** — keep specific users off the starboard entirely.
- **Interactive setup wizard** — a guided, step-by-step configuration.

## Commands

All commands are prefixed (defaults: `e!`, `e.`, `e `; the bot also responds to being mentioned).

| Command | Description |
| --- | --- |
| `ping` | Checks that the bot is online and measures Discord API latency. |
| `help [command]` | Lists all visible commands, or detailed help for one. |
| `set` / `set <setting> <value>` | View or change the server's configuration (`prefix`, `enabled`, `selfstar`, `ignorebots`, `starboard`, `emote`, `stars`, `color`). |
| `setup` | Runs an interactive wizard to configure the starboard. |
| `ban <channel> [...]` | Bans channels from being posted to the starboard. |
| `unban <channel> [...]` | Re-allows banned channels. |
| `blacklist <user> [...]` | Blacklists users from the starboard. |
| `unblacklist <user> [...]` | Removes users from the blacklist. |
| `req <channel> <stars\|default>` | Sets a custom star requirement for a channel, or resets it to the server default. |
| `invite` | Sends the bot's invite link. |

## Requirements

- [Go](https://go.dev/dl/) 1.25 or newer
- A running [MongoDB](https://www.mongodb.com/) instance (local `mongodb://localhost:27017` works)
- A Discord bot application + token ([Discord Developer Portal](https://discord.com/developers/applications)) invited with the necessary intents for message content, reactions, and guilds

## Running locally

### 1. Set up configuration

All configuration is read from environment variables or a JSON config file. There is no `.env` loading, so export the variables in your shell (or use a process manager such as `direnv`).

Required:

| Variable | Description |
| --- | --- |
| `EUGEN_BOT_TOKEN` | Your Discord bot token. |
| `EUGEN_MONGODB_URL` | MongoDB connection URI (e.g. `mongodb://localhost:27017`). |

Optional:

| Variable | Description |
| --- | --- |
| `EUGEN_DB_NAME` | Database name (default: `eugen`). |
| `EUGEN_DB_TIMEOUT` | MongoDB connect timeout as a Go duration (default: `10s`). |
| `EUGEN_PREFIXES` | Comma-separated command prefixes (default: `e!,e.,e `). |
| `EUGEN_CONFIG` | Path to a JSON config file that sets the same values; env vars take precedence. |

Example with a local MongoDB:

```sh
export EUGEN_BOT_TOKEN="your-bot-token"
export EUGEN_MONGODB_URL="mongodb://localhost:27017"
```

### 2. Run the bot

```sh
go run .
```

Or build and run the binary:

```sh
go build -o eugen .
./eugen
```

Mongo collections (`guilds`, `messages`) and their indexes are created automatically on startup.

## Tests

The full suite (what CI runs):

```sh
go test -v -race -count=1 ./...
```

> Note: some suites spin up a MongoDB container via testcontainers and need a local Docker daemon.

## License

[MIT](LICENSE)
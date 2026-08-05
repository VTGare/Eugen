# Privacy Policy

**Eugen** is a Discord bot that provides starboard functionality — tracking
message reactions and reposting positively-reacted-to messages to a dedicated
starboard channel. This policy describes what data Eugen collects, how it is
used, and how long it is retained.

## Data Controller

- **Project**: Eugen — https://github.com/VTGare/Eugen
- **Developer**: VTGare
- **License**: MIT

The developer operates the bot and is the data controller for any user data
processed.

## Information Discord Provides to the Bot

When Eugen is added to a server, Discord provides data through the gateway
based on the intents the bot is granted. Eugen requests the following privileged
and standard intents:

- **MESSAGE CONTENT INTENT** — Required to read message content for the
  following purposes:
  - Parsing text-based commands (e.g. `e!set`, `e!ban`, `e!help`).
  - Extracting message content from messages so that it can be featured on the starboard.

Message content is processed in memory only. **Eugen does not persist the text
content of any message to its database.** Only message and channel identifiers
are stored, as described below.

## Data Stored by the Bot

### Guild Settings (MongoDB collection: `guilds`)

Per-server configuration stored while the bot is in that server:

| Field | Purpose |
|---|---|
| `guild_id` | Discord server ID (primary key for lookups) |
| `name` | Server name, used for logging and identification |
| `prefix` | Server-specific command prefix (defaults to `e!`) |
| `emote` | The star reaction emote to track |
| `color` | Embed color for starboard messages |
| `enabled` | Whether starboard is active for this server |
| `starboard` | Channel IDs for starboard output |
| `selfstar` | Whether the author's own reaction counts |
| `ignorebots` | Whether reactions from bots are skipped |
| `stars` | Minimum reaction count to post |
| `channel_settings` | Per-channel star thresholds |
| `blacklisted_users` | User IDs whose messages never reach the starboard |
| `banned` | Channel IDs excluded from starboarding |
| `created_at` / `updated_at` | Timestamps for record management |

### Message Records (MongoDB collection: `messages`)

When a message is starboardled (posted to the starboard channel), Eugen stores
only identifiers — never the message text:

| Field | Purpose |
|---|---|
| `guild_id` | Server ID |
| `original.channel_id` | Channel where the original message was posted |
| `original.message_id` | The original message ID |
| `starboard.channel_id` | Channel where the starboard post was made |
| `starboard.message_id` | The starboard message ID |
| `created_at` | When the starboard record was created |

### In-Memory Caches

- **Guild settings cache** — Loaded at startup into a concurrency-safe in-memory
  map (`sync.RWMutex`) for fast per-guild lookups during event processing. This
  cache is not persistent across restarts.
- **Message cache** — Recent message-to-starboard pairings are cached in memory
  with a TTL-based eviction policy (10-hour TTL, half-TTL sweep interval). This
  accelerates reverse lookups (e.g. finding the starboard post for a given
  original message). This cache is not persistent across restarts.

## How the Bot Uses Your Data

1. **Guild settings** are read on every reaction-add event to determine whether
   the reacting server has starboard enabled, what emote to track, how many
   stars are required, whether the channel is banned, and whether the author or
   bots should be ignored.
2. **Message records** link an original message to its starboard counterpart so
   that reaction removals, count updates, and message deletions can be
   synchronized between the two.
3. **Message content** is read at runtime to build the starboard embed (extracting
   media URLs, formatting reply context, and reading command invocations) but is
   never written to persistent storage.

## Data Retention

- **Guild settings** are retained as long as the bot remains in the server. When
  the bot is removed from a server, no automated deletion of that server's data
  occurs. Server administrators can request data deletion by contacting the
  developer (see below).
- **Message records** are retained indefinitely, as they are needed to maintain
  the link between original messages and their starboard posts. If either the
  original message or its starboard counterpart is deleted, the corresponding
  record is removed.
- **In-memory caches** are ephemeral and discarded on bot restart.

## Data Sharing and Third Parties

- **Discord** — The bot communicates with Discord's API to receive gateway
  events, fetch messages and channels, post starboard messages, and manage
  reactions. Discord's own privacy policy governs data Discord collects
  independently: https://discord.com/privacy
- **MongoDB** — If self-hosted, data is stored in the bot operator's MongoDB
  database. If using MongoDB Atlas, data is stored in MongoDB's cloud
  infrastructure. See MongoDB's privacy policy: https://www.mongodb.com/legal/privacy-policy

The developer does not sell, trade, or rent user data to third parties. The bot
does not transmit collected data to any analytics, advertising, or tracking
service.

## Data Subject Rights

Users may:

- **Request access** to the data the bot holds about their messages and
  server.
- **Request deletion** of guild settings or message records by contacting the
  developer. Note that deleting message records will break the link between
  existing starboard posts and their original messages.
- **Opt out** of the starboard for their messages by asking a server
  administrator to blacklist their user ID, or by leaving the server.

## Changes to This Policy

This policy may be updated as the bot evolves. Substantial changes will be
documented in the project's commit history. The current version always reflects
the latest state of the codebase.

## Contact

For questions about this privacy policy or to request data deletion, please
open an issue or contact the developer @vtgare on Discord

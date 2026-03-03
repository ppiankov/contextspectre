# Claude Code Session Architecture

## How sessions are stored

Claude Code stores conversation history as JSONL files under `~/.claude/projects/`. The directory name is the **encoded absolute path** of the working directory where Claude Code was launched:

```
~/.claude/projects/-Users-user-dev-myproject/          # launched from ~/dev/myproject
~/.claude/projects/-Users-user-dev-myproject-subdir/    # launched from ~/dev/myproject/subdir
```

Each session gets a UUID filename: `<uuid>.jsonl`. The same directory can contain multiple session files from different Claude Code instances launched at different times.

## Multiple instances, same codebase

Claude Code CLI and Claude for Mac both use `~/.claude/projects/`. If both are launched from the same directory, their sessions live side by side:

```
~/.claude/projects/-Users-user-dev-myproject/
├── a1b2c3d4-...jsonl    # Claude for Mac session
├── e5f6g7h8-...jsonl    # CLI session (terminal 1)
└── i9j0k1l2-...jsonl    # CLI session (terminal 2)
```

If launched from **different directories** in the same repo, they get **different project directories**:

```
~/.claude/projects/-Users-user-dev-myproject/           # CLI from repo root
~/.claude/projects/-Users-user-dev-myproject-src/       # Mac app from src/
```

Key point: the project path is determined by launch directory, not by git root.

## Session identity

Each session has several identifying fields in its JSONL entries:

| Field | Example | Description |
|-------|---------|-------------|
| `sessionId` | `88275ecc-6767-...` | UUID, matches the filename |
| `slug` | `toasty-jumping-hare` | Human-readable session name |
| `version` | `2.1.51` | Claude Code version |
| `userType` | `external` | Authentication type |
| `permissionMode` | `acceptEdits` | Permission level |

The `slug` is the most useful for distinguishing sessions visually.

## Entry types

Not all JSONL entries are conversation messages sent to the API:

| Type | Sent to API | Description |
|------|-------------|-------------|
| `user` | Yes | User messages and tool results |
| `assistant` | Yes | Model responses and tool calls |
| `system` | No | Hook summaries, stop reasons |
| `queue-operation` | No | Internal scheduling (enqueue/dequeue) |
| `progress` | No | Streaming progress indicators |
| `file-history-snapshot` | No | File state snapshots |

Only `user` and `assistant` entries consume context window tokens. The others are JSONL bookkeeping — they take disk space but zero context.

## Safety implications for contextspectre

### Cleaning affects all readers

When contextspectre modifies a session JSONL file, **every Claude instance reading that file is affected**. If Claude for Mac and CLI share a session file, cleaning it from one terminal affects the other.

This is especially dangerous because:

1. **You may not know another instance is using the file.** Claude for Mac's Code tab and a CLI terminal can both work on the same codebase independently.
2. **Invalid modifications break session resume.** If a cleanup operation produces content the API rejects (e.g., malformed images), the session becomes stuck — it cannot send messages because the API rejects the conversation history.
3. **The damage is silent.** The instance that ran the cleanup works fine. The other instance fails on next resume.

### The image replacement incident

In v0.4.6, `clean --all` replaced base64 images with a 1x1 PNG placeholder but kept the original `media_type: "image/jpeg"`. This created a JPEG/PNG mismatch that the API rejected:

```
messages.4.content.0.image.source.base64: The image was specified using
the image/jpeg media type, but the image appears to be a image/png image
```

The session became permanently stuck — every resume attempt hit the same error. Fixed in v0.4.8 by replacing image blocks with text blocks (`[image removed by contextspectre]`) instead of substituting image data.

### Best practices

1. **Know which session you're cleaning.** Use `contextspectre sessions` to list all sessions in a project. Check the slug and last-modified time to identify the right one.
2. **Don't clean sessions other Claude instances are actively using.** If Claude for Mac is open on the same project, its session file should not be modified.
3. **Use `--live` for active sessions.** The `--live` flag enables mtime-based race detection and restricts operations to safe tiers.
4. **Backup is mandatory.** Every modification creates a `.bak` file. Use `contextspectre fix` or TUI undo (`u`) to restore if something goes wrong.

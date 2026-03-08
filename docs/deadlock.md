# Context Deadlock

A session can reach a state where it cannot continue AND cannot compact. This is a terminal condition that cleanup cannot fix.

## How it happens

Claude Code's context meter shows conversation tokens as a percentage of 200K. But the API request includes more than the conversation - system prompt, tool definitions, project instructions, and framework overhead add ~40-60K tokens that the meter doesn't show. A session reporting 75% (150K tokens) is actually consuming ~190-210K at the API level. When the total exceeds the API limit, new messages are rejected with "Prompt is too long."

## Why compaction fails too

Compaction is itself an API call - Claude Code sends the full conversation and asks the model to summarize it. If the conversation is too large for a normal API call, it's too large for the compaction call. The session is stuck: too big to continue, too big to compress.

## Why `clean --all` doesn't fix it

Cleanup removes disk noise: progress messages, file-history-snapshots, stale reads, and failed retries. These are JSONL metadata that bloat the file on disk but are NOT sent to the API. The entries that fill API context are user messages, assistant responses, and system entries - the actual conversation. Cleanup reduces the file from 36MB to 17MB while the API request stays exactly the same size.

## Recovery: amputate

The `amputate` command surgically removes entries from the end of the conversation, dropping the token count below the compaction threshold:

```bash
contextspectre amputate <session-id> --last 20 --apply
```

This removes the last 20 entries (typically the oversized message that triggered the deadlock plus error responses), allowing Claude Code to resume and compact normally. Always runs in dry-run mode first - add `--apply` to execute.

See [Commands](commands.md) for the full CLI reference.

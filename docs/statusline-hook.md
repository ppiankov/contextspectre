# Status line hook - full reference

Complete hook script for Claude Code's status line integration. See [workflow.md](workflow.md#cli-status-line-integration) for setup instructions and indicator explanations.

## Hook script

Save as `~/.claude/statusline.sh` and make executable (`chmod +x`).

```bash
#!/bin/bash
# Status line: model, session ID, context %, signal grade, cleanable tokens, IPS, cost
# contextspectre data via background cache (never blocks)

input=$(cat)

root=$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")
repo=$(basename "$root")
mode=$(head -1 "/tmp/claude-mode-$PPID" 2>/dev/null)
sid=$(echo "$input" | jq -r '.transcript_path // ""' | xargs basename 2>/dev/null | sed 's/\.jsonl$//' | cut -c1-8)
model=$(echo "$input" | jq -r '.model.display_name // "?"')
ctx_pct=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
echo "$ctx_pct" > "/tmp/claude-ctx-$PPID"
cost=$(printf '%.2f' "$(echo "$input" | jq -r '.cost.total_cost_usd // 0')")

# contextspectre cache: read if fresh (<60s), fork refresh in background
cache="/tmp/contextspectre-status-$PPID.json"
signal=""
cleanable=""

if [ -f "$cache" ]; then
  cache_age=$(( $(date +%s) - $(stat -f %m "$cache" 2>/dev/null || echo 0) ))
  if [ "$cache_age" -lt 60 ]; then
    signal=$(jq -r '.signal_grade // empty' "$cache" 2>/dev/null)
    ips_raw=$(jq -r '.input_purity // empty' "$cache" 2>/dev/null)
    cleanable_raw=$(jq -r '.cleanable_tokens // 0' "$cache" 2>/dev/null)
    if [ "$cleanable_raw" -gt 1000 ] 2>/dev/null; then
      cleanable="$(( cleanable_raw / 1000 ))K"
    fi
  fi
fi

# Fork background refresh (non-blocking)
if [ ! -f "$cache" ] || [ "${cache_age:-999}" -ge 60 ]; then
  (contextspectre summary --cwd --format json > "$cache" 2>/dev/null &)
fi

# Color context based on usage
if [ "$ctx_pct" -ge 80 ]; then
  ctx_color="\033[31m"  # red
elif [ "$ctx_pct" -ge 60 ]; then
  ctx_color="\033[33m"  # yellow
else
  ctx_color="\033[32m"  # green
fi
reset="\033[0m"

# Assemble label (includes mode if set)
label="${repo}"
[ -n "$mode" ] && label="${repo}:${mode}"

# Color cleanable tokens based on severity
clean_seg=""
if [ -n "$cleanable" ]; then
  if [ "$cleanable_raw" -ge 500000 ] 2>/dev/null; then
    clean_color="\033[31m"  # red: >500K — manual clean --all needed
  elif [ "$cleanable_raw" -ge 100000 ] 2>/dev/null; then
    clean_color="\033[33m"  # yellow: >100K — consider cleaning
  else
    clean_color="\033[32m"  # green: <100K — healthy
  fi
  clean_seg=" clean:${clean_color}${cleanable}${reset}"
fi

# Color signal grade based on health
sig_seg=""
if [ -n "$signal" ]; then
  case "$signal" in
    A|B) sig_color="\033[32m" ;;  # green: healthy
    C|D) sig_color="\033[33m" ;;  # yellow: degrading
    *)   sig_color="\033[31m" ;;  # red: F or unknown
  esac
  sig_seg=" | sig:${sig_color}${signal}${reset}"
fi

# Color input purity score
ips_seg=""
if [ -n "$ips_raw" ] && [ "$ips_raw" != "0" ]; then
  ips_int=$(printf '%.0f' "$ips_raw")
  if [ "$ips_int" -ge 80 ] 2>/dev/null; then
    ips_color="\033[32m"  # green: well-purified
  elif [ "$ips_int" -ge 50 ] 2>/dev/null; then
    ips_color="\033[33m"  # yellow: room to improve
  else
    ips_color="\033[31m"  # red: mostly raw input
  fi
  ips_seg=" ips:${ips_color}${ips_int}${reset}"
fi

# Chain integrity — red ⚠ if broken
chain_seg=""
chain_raw=$(jq -r '.chain_healthy // true' "$cache" 2>/dev/null)
if [ "$chain_raw" = "false" ]; then
  chain_seg=" \033[31m⚠\033[0m"
fi

# Assemble contextspectre segment
cs_seg="${sig_seg}${clean_seg}${ips_seg}${chain_seg}"

sid_seg=""
[ -n "$sid" ] && sid_seg=" ${sid}"

printf '%b' "${label} | ${model}${sid_seg} | ctx:${ctx_color}${ctx_pct}%${reset}${cs_seg} | \$${cost}"
```

## How it works

The hook runs on every Claude Code turn. Claude Code passes session metadata as JSON on stdin.

- **Claude Code fields** (always available): repo, model, context %, cost
- **ContextSpectre fields** (from background cache): signal grade, cleanable tokens, IPS, chain health
- The background cache refreshes every 60 seconds via `contextspectre summary --cwd --format json`
- Cache reads are instant; the hook never blocks the CLI prompt
- Context percentage is written to `/tmp/claude-ctx-$PPID` for use by other hooks (e.g., auto-checkpoint)
- Mode label (e.g., `repo:plan`) is read from `/tmp/claude-mode-$PPID` if present

## Output format

```
contextspectre:plan | Opus 4.6 15c93cef | ctx:73% | sig:A clean:3K ips:77 | $86.66
```

Fields: `repo[:mode]`, model, session ID (first 8 chars), context fill, signal grade, cleanable tokens, input purity score, session cost. A red `⚠` appears at the end when the session has a broken parent chain.

## Requirements

- `jq` for JSON parsing
- `contextspectre` on PATH
- macOS `stat -f %m` for mtime (Linux: use `stat -c %Y`)

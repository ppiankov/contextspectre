# Status line hook - full reference

Complete hook script for Claude Code's status line integration. See [workflow.md](workflow.md#cli-status-line-integration) for setup instructions and indicator explanations.

## Hook script

```bash
#!/bin/bash
# Status line: model, session ID, context %, signal grade, cleanable tokens, IPS, cost
# contextspectre data via background cache (never blocks)

input=$(cat)

root=$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")
repo=$(basename "$root")
sid=$(echo "$input" | jq -r '.transcript_path // ""' | xargs basename 2>/dev/null | sed 's/\.jsonl$//' | cut -c1-8)
model=$(echo "$input" | jq -r '.model.display_name // "?"')
ctx_pct=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
cost=$(printf '%.2f' "$(echo "$input" | jq -r '.cost.total_cost_usd // 0')")

# contextspectre: read cached data (signal, cleanable, IPS)
# Background refresh keeps data fresh without blocking
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

# Fork background refresh (non-blocking, never stalls the prompt)
if [ ! -f "$cache" ] || [ "${cache_age:-999}" -ge 60 ]; then
  (contextspectre summary --cwd --format json > "$cache" 2>/dev/null &)
fi

# Color context by usage level
if [ "$ctx_pct" -ge 80 ]; then ctx_color="\033[31m"      # red
elif [ "$ctx_pct" -ge 60 ]; then ctx_color="\033[33m"     # yellow
else ctx_color="\033[32m"; fi                              # green
reset="\033[0m"

# Color cleanable tokens by severity
clean_seg=""
if [ -n "$cleanable" ]; then
  if [ "$cleanable_raw" -ge 500000 ] 2>/dev/null; then
    clean_color="\033[31m"    # red: >500K
  elif [ "$cleanable_raw" -ge 100000 ] 2>/dev/null; then
    clean_color="\033[33m"    # yellow: >100K
  else
    clean_color="\033[32m"    # green: <100K
  fi
  clean_seg=" clean:${clean_color}${cleanable}${reset}"
fi

# Color signal grade by health
sig_seg=""
if [ -n "$signal" ]; then
  case "$signal" in
    A|B) sig_color="\033[32m" ;;  # green
    C|D) sig_color="\033[33m" ;;  # yellow
    *)   sig_color="\033[31m" ;;  # red
  esac
  sig_seg=" | sig:${sig_color}${signal}${reset}"
fi

# Color input purity score
ips_seg=""
if [ -n "$ips_raw" ] && [ "$ips_raw" != "0" ]; then
  ips_int=$(printf '%.0f' "$ips_raw")
  if [ "$ips_int" -ge 80 ] 2>/dev/null; then
    ips_color="\033[32m"      # green: well-purified
  elif [ "$ips_int" -ge 50 ] 2>/dev/null; then
    ips_color="\033[33m"      # yellow: room to improve
  else
    ips_color="\033[31m"      # red: mostly raw input
  fi
  ips_seg=" ips:${ips_color}${ips_int}${reset}"
fi

# Chain integrity - red ⚠ if broken
chain_seg=""
chain_raw=$(jq -r '.chain_healthy // true' "$cache" 2>/dev/null)
if [ "$chain_raw" = "false" ]; then
  chain_seg=" \033[31m⚠\033[0m"
fi

# Assemble and print
cs_seg="${sig_seg}${clean_seg}${ips_seg}${chain_seg}"
sid_seg=""
[ -n "$sid" ] && sid_seg=" ${sid}"
printf '%b' "${repo} | ${model}${sid_seg} | ctx:${ctx_color}${ctx_pct}%${reset}${cs_seg} | \$${cost}"
```

## How it works

The hook runs on every Claude Code turn. Claude Code passes session metadata as JSON on stdin.

- **Claude Code fields** (always available): repo, model, context %, cost
- **ContextSpectre fields** (from background cache): signal grade, cleanable tokens, IPS, chain health, session ID
- The background cache refreshes every 60 seconds via `contextspectre summary --cwd --format json`
- Cache reads are instant; the hook never blocks the CLI prompt

## Requirements

- `jq` for JSON parsing
- `contextspectre` on PATH
- macOS `stat -f %m` for mtime (Linux: use `stat -c %Y`)

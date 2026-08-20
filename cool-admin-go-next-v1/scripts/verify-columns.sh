#!/usr/bin/env bash
set -euo pipefail

SCOPE="${1:-.}"
MAPPING="$(dirname "$0")/column-mapping.txt"

if [ ! -f "$MAPPING" ]; then
  echo "Mapping file not found: $MAPPING" >&2
  exit 1
fi

remaining=0
# 第一遍:带双引号的精确字面量
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  pattern="\"$old\""
  # `|| true` swallows grep's exit-1 on 0 matches, otherwise `set -e` + `pipefail` aborts
  count=$(grep -rE "$pattern" "$SCOPE" \
    --include='*.go' --include='*.yaml' --include='*.yml' --include='*.json' --include='*.md' \
    --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist \
    --exclude-dir=.superpowers --exclude-dir=docs \
    --exclude='*.min.*' 2>/dev/null | wc -l | tr -d ' ' || true)
  if [ -n "$count" ] && [ "$count" -gt 0 ]; then
    echo "  $old → $new: $count remaining"
    remaining=$((remaining + count))
  fi
done < "$MAPPING"

# 第二遍:裸列名(无 alias 前缀,出现在 SQL 字符串内),只针对 Go 源文件
# 用字符类边界替代 BSD sed 不支持的 \b,避免把 substrings(如 Mylock_expire_time)误报
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  pattern="(^|[^A-Za-z0-9_])${old}([^A-Za-z0-9_]|\$)"
  count=$(grep -rE "$pattern" "$SCOPE" \
    --include='*.go' \
    --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist \
    --exclude-dir=.superpowers --exclude-dir=docs \
    --exclude='*.min.*' 2>/dev/null | wc -l | tr -d ' ' || true)
  if [ -n "$count" ] && [ "$count" -gt 0 ]; then
    echo "  $old → $new (bare): $count remaining"
    remaining=$((remaining + count))
  fi
done < "$MAPPING"

echo "---"
echo "Total remaining: $remaining"
exit $remaining

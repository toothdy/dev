#!/usr/bin/env bash
set -euo pipefail

SCOPE="${1:-.}"
MAPPING="$(dirname "$0")/column-mapping.txt"

if [ ! -f "$MAPPING" ]; then
  echo "Mapping file not found: $MAPPING" >&2
  exit 1
fi

total=0
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  # 第一遍:精确匹配带双引号的旧字面量
  pattern="\"$old\""
  replacement="\"$new\""
  while IFS= read -r -d '' file; do
    before=$(grep -c "$pattern" "$file" 2>/dev/null || true)
    [ "$before" = "0" ] && continue
    sed -i.tmp "s|$pattern|$replacement|g" "$file"
    rm -f "$file.tmp"
    echo "  $file: $old → $new × $before"
    total=$((total + before))
  done < <(find "$SCOPE" \( -name '*.go' -o -name '*.yaml' -o -name '*.yml' -o -name '*.json' -o -name '*.md' \) \
           -not -path '*/\.git/*' -not -path '*/node_modules/*' -not -path '*/dist/*' \
           -not -path '*/.superpowers/*' -not -path '*/docs/*' \
           -not -name '*.min.*' -print0)
done < "$MAPPING"

# 第二遍:处理别名限定的列名(如 "a.user_id" → "a.userId")——只针对 Go 源文件
# 注:BSD sed(macOS)不支持 \b 与 | 交替,使用字符类边界 + 4 个独立 sed 表达式分别覆盖:
# 行首后跟非词字符 / 行首即行尾 / 中段后跟非词字符 / 中段即行尾
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  while IFS= read -r -d '' file; do
    before=$(grep -cE "(^|[^A-Za-z0-9_])([a-z])\.${old}([^A-Za-z0-9_]|\$)" "$file" 2>/dev/null || true)
    [ "$before" = "0" ] && continue
    sed -i.tmp -E \
      -e "s|^([a-z])\\.${old}([^A-Za-z0-9_])|\\1.${new}\\2|g" \
      -e "s|^([a-z])\\.${old}\$|\\1.${new}|g" \
      -e "s|([^A-Za-z0-9_])([a-z])\\.${old}([^A-Za-z0-9_])|\\1\\2.${new}\\3|g" \
      -e "s|([^A-Za-z0-9_])([a-z])\\.${old}\$|\\1\\2.${new}|g" \
      "$file"
    rm -f "$file.tmp"
    echo "  $file: alias $old → $new × $before"
    total=$((total + before))
  done < <(find "$SCOPE" -name '*.go' \
           -not -path '*/\.git/*' -not -path '*/node_modules/*' -not -path '*/dist/*' \
           -not -path '*/.superpowers/*' -not -path '*/docs/*' \
           -not -name '*.min.*' -print0)
done < "$MAPPING"

# 第三遍:处理无 alias 前缀的裸列名(如 "lock_expire_time" → "lockExpireTime")——只针对 Go 源文件
# 用于捕获内嵌在较大 SQL 字符串字面量里的裸 snake_case 列名(第一遍只匹配全字符串字面量,第二遍要求 alias.X 前缀)
# 注:BSD sed(macOS)不支持 \b 与 | 交替,使用字符类边界 + 4 个独立 sed 表达式
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  while IFS= read -r -d '' file; do
    before=$(grep -cE "(^|[^A-Za-z0-9_])${old}([^A-Za-z0-9_]|\$)" "$file" 2>/dev/null || true)
    [ "$before" = "0" ] && continue
    sed -i.tmp -E \
      -e "s|^${old}([^A-Za-z0-9_])|${new}\\1|g" \
      -e "s|^${old}\$|${new}|g" \
      -e "s|([^A-Za-z0-9_])${old}([^A-Za-z0-9_])|\\1${new}\\2|g" \
      -e "s|([^A-Za-z0-9_])${old}\$|\\1${new}|g" \
      "$file"
    rm -f "$file.tmp"
    echo "  $file: bare $old → $new × $before"
    total=$((total + before))
  done < <(find "$SCOPE" -name '*.go' \
           -not -path '*/\.git/*' -not -path '*/node_modules/*' -not -path '*/dist/*' \
           -not -path '*/.superpowers/*' -not -path '*/docs/*' \
           -not -name '*.min.*' -print0)
done < "$MAPPING"

echo "Total replacements: $total"

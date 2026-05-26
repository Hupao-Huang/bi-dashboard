#!/bin/bash
# 发送钉钉通知 (从 server/config.json 读凭证, 与后端 Go 共用一份)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CFG="${SCRIPT_DIR}/config.json"
if [ ! -f "$CFG" ]; then
    echo "[send_ding.sh] 找不到 $CFG, 请按 server/.env.example 配置凭证" >&2
    exit 1
fi
TOKEN=$(python -c "import json,sys; print(json.load(open(sys.argv[1]))['dingtalk']['webhook_token'])" "$CFG" 2>/dev/null)
SECRET=$(python -c "import json,sys; print(json.load(open(sys.argv[1]))['dingtalk']['webhook_secret'])" "$CFG" 2>/dev/null)
if [ -z "$TOKEN" ] || [ -z "$SECRET" ]; then
    echo "[send_ding.sh] server/config.json 缺 dingtalk.webhook_token 或 webhook_secret" >&2
    exit 1
fi
MSG="$1"

TIMESTAMP=$(date +%s%3N)
SIGN_STR="${TIMESTAMP}\n${SECRET}"
SIGN=$(echo -ne "$SIGN_STR" | openssl dgst -sha256 -hmac "$SECRET" -binary | base64 | python3 -c "import sys,urllib.parse;print(urllib.parse.quote(sys.stdin.read().strip()))" 2>/dev/null || echo -ne "$SIGN_STR" | openssl dgst -sha256 -hmac "$SECRET" -binary | base64 | sed 's/+/%2B/g;s/\//%2F/g;s/=/%3D/g')

URL="https://oapi.dingtalk.com/robot/send?access_token=${TOKEN}&timestamp=${TIMESTAMP}&sign=${SIGN}"

curl -s -X POST "$URL" \
  -H "Content-Type: application/json" \
  -d "{\"msgtype\":\"text\",\"text\":{\"content\":\"${MSG}\"}}"

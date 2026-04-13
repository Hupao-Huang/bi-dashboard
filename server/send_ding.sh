#!/bin/bash
# 发送钉钉通知
TOKEN="8969f9eb2e775d6c2b7c825b090f4b13c948b7ae74271e712e11140cf920fd35"
SECRET="SEC7fc41ffb9de85b7136195100f56fe4e796d0760627e378e125645ed41dce5096"
MSG="$1"

TIMESTAMP=$(date +%s%3N)
SIGN_STR="${TIMESTAMP}\n${SECRET}"
SIGN=$(echo -ne "$SIGN_STR" | openssl dgst -sha256 -hmac "$SECRET" -binary | base64 | python3 -c "import sys,urllib.parse;print(urllib.parse.quote(sys.stdin.read().strip()))" 2>/dev/null || echo -ne "$SIGN_STR" | openssl dgst -sha256 -hmac "$SECRET" -binary | base64 | sed 's/+/%2B/g;s/\//%2F/g;s/=/%3D/g')

URL="https://oapi.dingtalk.com/robot/send?access_token=${TOKEN}&timestamp=${TIMESTAMP}&sign=${SIGN}"

curl -s -X POST "$URL" \
  -H "Content-Type: application/json" \
  -d "{\"msgtype\":\"text\",\"text\":{\"content\":\"${MSG}\"}}"

#!/usr/bin/env sh
set -eu

# 对已切换到 block 模式的网站执行最小 CRS 回归集。
BASE_URL=${BASE_URL:-http://127.0.0.1}
UA='workmesh-waf-regression'

check() {
  name="$1"
  url="$2"
  expected="$3"
  user_agent="${4:-$UA}"
  actual=$(curl -ksS -o /dev/null -w '%{http_code}' -A "$user_agent" "$url")
  if [ "$actual" != "$expected" ]; then
    echo "FAIL $name: expected $expected, got $actual" >&2
    exit 1
  fi
  echo "PASS $name: $actual"
}

check_method() {
  name="$1"
  method="$2"
  url="$3"
  expected="$4"
  actual=$(curl -ksS -X "$method" -o /dev/null -w '%{http_code}' -A "$UA" "$url")
  case ",$expected," in
    *,"$actual",*) ;;
    *)
      echo "FAIL $name: expected one of $expected, got $actual" >&2
      exit 1
      ;;
  esac
  echo "PASS $name: $actual"
}

check_post() {
  name="$1"
  url="$2"
  payload="$3"
  expected="$4"
  actual=$(curl -ksS -o /dev/null -w '%{http_code}' -A "$UA" -H 'Content-Type: application/x-www-form-urlencoded' --data "$payload" "$url")
  if [ "$actual" != "$expected" ]; then
    echo "FAIL $name: expected $expected, got $actual" >&2
    exit 1
  fi
  echo "PASS $name: $actual"
}

check 'normal request' "$BASE_URL/?id=42" 200
check 'SQL injection' "$BASE_URL/?id=1%20union%20select%201" 403
check 'XSS' "$BASE_URL/?q=%3Cscript%3Ealert(1)%3C/script%3E" 403
check 'path traversal' "$BASE_URL/?file=../../etc/passwd" 403
check 'command injection' "$BASE_URL/?cmd=%3Bid%20%7C%20whoami" 403
check 'PHP injection' "$BASE_URL/?code=%3C%3Fphp%20eval%28%24x%29" 403
check 'scanner user-agent' "$BASE_URL/" 403 'sqlmap/1.8'
check_method 'TRACE method' TRACE "$BASE_URL/" '403,405'
check_post 'XSS request body' "$BASE_URL/submit" 'payload=%3Cscript%3Ealert%281%29%3C%2Fscript%3E' 403
check_post 'command request body' "$BASE_URL/submit" 'cmd=%3Bid%20%7C%20whoami' 403

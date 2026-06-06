#!/usr/bin/env bash
# Rebuild assets/luasocket/linux-amd64/core.so (Lua 5.4 / luasocket 3.1) for BizHawk on Linux x86_64.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
out="$root/assets/luasocket/linux-amd64/core.so"
if [ -s "$out" ] && [ "${FORCE:-}" != 1 ]; then
  echo "LuaSocket core already present: $out ($(wc -c <"$out") bytes); set FORCE=1 to rebuild"
  exit 0
fi
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL 'https://downloads.sourceforge.net/project/luabinaries/5.4.2/Linux%20Libraries/lua-5.4.2_Linux54_64_lib.tar.gz' \
  | tar -xz -C "$tmpdir"
git clone --depth 1 --branch v3.1.0 https://github.com/lunarmodules/luasocket.git "$tmpdir/luasocket"
make -C "$tmpdir/luasocket/src" PLAT=linux LUAV=5.4 "LUAINC_linux=$tmpdir/include" all
mkdir -p "$(dirname "$out")"
cp "$tmpdir/luasocket/src/socket-3.0.0.so" "$out"
echo "wrote $out ($(wc -c <"$out") bytes)"

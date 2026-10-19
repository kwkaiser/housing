#!/usr/bin/env sh
set -eu

if out=$(nix build --no-link .#default.goModules 2>&1); then
  echo "vendorHash is up to date"
  exit 0
fi
got=$(printf '%s\n' "$out" | sed -n 's/.*got: *\(sha256-[A-Za-z0-9+/=]*\).*/\1/p' | head -n1)
if [ -z "$got" ]; then
  printf '%s\n' "$out" >&2
  exit 1
fi
sed -i.bak "s|vendorHash = \".*\";|vendorHash = \"$got\";|" nix/package.nix
rm nix/package.nix.bak
nix build --no-link .#default.goModules
echo "vendorHash updated to $got"

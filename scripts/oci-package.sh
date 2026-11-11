#!/usr/bin/env bash
# Build the provider for each platform, zip each build the way a registry serves it,
# and assemble an OpenTofu OCI provider index in a local OCI layout (ADR-0066).
# Pushing the layout is the caller's job (`oras cp --from-oci-layout`).
#
# Usage: scripts/oci-package.sh <version> <layout-dir>
#   version     SemVer without a leading v, e.g. 0.1.0 or 0.0.0-spike.1
#   layout-dir  a directory this script creates (replaced if it exists)
set -euo pipefail

version="${1:-}"
layout="${2:-}"
semver='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'
if [[ ! "$version" =~ $semver || -z "$layout" ]]; then
  echo "Usage: $0 <version> <layout-dir>  (version: SemVer, no leading v, no build metadata)" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
platforms=(linux_amd64 linux_arm64 darwin_arm64)
name="terraform-provider-netcup"
dist="${root}/dist/${version}"
layout="$(mkdir -p "$layout" && cd "$layout" && pwd)"
rm -rf "$dist" "${layout:?}"/*
mkdir -p "$dist"

# The pinned ORAS from tools/go.mod, built once.
CGO_ENABLED=0 go build -modfile=tools/go.mod -o "${root}/bin/oras" oras.land/oras/cmd/oras
oras="${root}/bin/oras"

# Reproducible: no paths, no build ID, every file stamped with the commit time.
epoch="$(git log -1 --format=%ct)"
for platform in "${platforms[@]}"; do
  os="${platform%_*}"
  arch="${platform#*_}"
  binary="${name}_v${version}"
  work="${dist}/${platform}"
  mkdir -p "$work"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -buildvcs=true \
    -ldflags "-s -w -buildid= -X main.version=${version}" -o "${work}/${binary}" .
  zip_file="${name}_${version}_${platform}.zip"
  # A deterministic zip: one entry, fixed mode 0755, the commit time, no extra fields.
  python3 -I - "${work}/${binary}" "${dist}/${zip_file}" "$epoch" <<'PY'
import sys, time, zipfile
src, dst, epoch = sys.argv[1], sys.argv[2], int(sys.argv[3])
info = zipfile.ZipInfo(src.rsplit("/", 1)[-1], date_time=time.gmtime(epoch)[:6])
info.external_attr = 0o100755 << 16
info.compress_type = zipfile.ZIP_DEFLATED
with open(src, "rb") as f, zipfile.ZipFile(dst, "w") as z:
    z.writestr(info, f.read(), compresslevel=9)
PY
  (cd "$dist" && "$oras" push --oci-layout "${layout}:${platform}" \
    --artifact-type application/vnd.opentofu.provider-target \
    --artifact-platform "${os}/${arch}" \
    "${zip_file}:archive/zip" >/dev/null)
  echo "Packaged ${zip_file}"
done

"$oras" manifest index create --oci-layout "${layout}:${version}" \
  --artifact-type application/vnd.opentofu.provider \
  --annotation "org.opencontainers.image.source=https://github.com/quitordinary/${name}" \
  "${platforms[@]}" >/dev/null

(cd "$dist" && sha256sum ./*.zip | sed 's| \./| |' >"${name}_${version}_SHA256SUMS")
echo "Index ${version} in ${layout}; checksums in ${dist}/${name}_${version}_SHA256SUMS:"
cat "${dist}/${name}_${version}_SHA256SUMS"

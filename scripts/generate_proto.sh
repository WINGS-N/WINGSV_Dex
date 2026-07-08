#!/usr/bin/env bash
set -euo pipefail

# Generates the Go proto stubs WINGSV_Dex needs:
#   wingsvpb     - the wingsv:// config schema (messages only) for the link codec.
#   appcontrolpb - the local AppControl IPC (messages + gRPC) to drive the vkturn child.
#
# Dex is a sibling client of the WINGS V Android app, so it takes the client proto from that
# repo rather than the panel. wingsv.proto is owned by WINGS V (app/src/main/proto/wingsv.proto)
# and synced here over a blobless sparse checkout - not a submodule and not a hand-maintained
# copy - mirroring how the panel syncs control.proto. appcontrol.proto is owned by vk-turn-proxy
# and read straight from that submodule (the Android app symlinks it from there too). Both protos
# declare an upstream go_package, so an M override remaps each into this module.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROTO_DIR="$ROOT_DIR/external/wingsv-proto"
VKTP_PROTO_DIR="$ROOT_DIR/external/vk-turn-proxy/proto"
MODULE="github.com/WINGS-N/wingsv-dex"
OUT_DIR="$ROOT_DIR/internal/gen"

PATH="$PATH:$(go env GOPATH)/bin"
mkdir -p "$PROTO_DIR" "$OUT_DIR/wingsvpb" "$OUT_DIR/appcontrolpb"

# Sync wingsv.proto from the WINGS V client repo. Override with WINGSV_PROTO_REPO / WINGSV_PROTO_REF.
WINGSV_PROTO_REPO="${WINGSV_PROTO_REPO:-https://github.com/WINGS-N/WINGSV.git}"
WINGSV_PROTO_REF="${WINGSV_PROTO_REF:-dev}"
sync_wingsv_proto() {
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  git clone --quiet --depth 1 --filter=blob:none --sparse \
    --branch "$WINGSV_PROTO_REF" "$WINGSV_PROTO_REPO" "$tmp"
  git -C "$tmp" sparse-checkout set app/src/main/proto >/dev/null
  cp "$tmp/app/src/main/proto/wingsv.proto" "$PROTO_DIR/wingsv.proto"
}
sync_wingsv_proto

rm -f "$OUT_DIR/wingsvpb"/*.pb.go
protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module="$MODULE" \
  --go_opt=Mwingsv.proto="$MODULE/internal/gen/wingsvpb" \
  "$PROTO_DIR/wingsv.proto"

rm -f "$OUT_DIR/appcontrolpb"/*.pb.go
protoc \
  --proto_path="$VKTP_PROTO_DIR" \
  --go_out="$ROOT_DIR" \
  --go_opt=module="$MODULE" \
  --go_opt=Mappcontrol.proto="$MODULE/internal/gen/appcontrolpb" \
  --go-grpc_out="$ROOT_DIR" \
  --go-grpc_opt=module="$MODULE" \
  --go-grpc_opt=Mappcontrol.proto="$MODULE/internal/gen/appcontrolpb" \
  "$VKTP_PROTO_DIR/appcontrol.proto"

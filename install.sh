#!/bin/sh
set -eu

LOCAL_BIN_DIR="${HOME}/bin"
REMOTE_BIN_DIR=".local/bin"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/claude-clip-bridge"
HOSTS_DIR="$CONFIG_DIR/hosts"
TOKEN_FILE="$CONFIG_DIR/token"
BASE_SSH_CONFIG="${HOME}/.ssh/config"
COMPAT_WRAPPER_NAME="ccb-ssh"
REMOTE_GOOS="linux"
REMOTE_GOARCH="amd64"
BRIDGE_PORT=19876
AUTO_EDIT_SSH_CONFIG=1
DRY_RUN=0
HOSTS=""

usage() {
	cat <<'EOF'
Usage:
  ./install.sh --host <ssh-alias> [--host <ssh-alias> ...] [options]

What it does:
  - Builds local macOS binaries: clip-bridge, clip-bridge-start
  - Builds the remote Linux xclip shim and deploys it to selected hosts
  - Generates a shared token for authentication and deploys it to each host
  - Generates per-host managed SSH fragments
  - Integrates one Include line into your SSH config so plain ssh works for configured hosts

Important:
  - Existing ~/.ssh/config host blocks are not rewritten
  - Plain `ssh <configured-host>` becomes the primary workflow after setup
  - A compatibility `ccb-ssh` wrapper is installed, but it is not the main path
  - Re-running install regenerates the token, invalidating previous sessions

Options:
  --host <name>               Add a host alias. Repeatable.
  --port <port>               TCP port for bridge forwarding. Default: 19876
  --local-bin-dir <path>      Install local binaries and compatibility wrapper here. Default: ~/bin
  --remote-bin-dir <path>     Remote directory for xclip shim. Default: .local/bin
  --config-dir <path>         Managed config directory. Default: ~/.config/claude-clip-bridge
  --base-ssh-config <path>    Existing SSH config to integrate with. Default: ~/.ssh/config
  --compat-wrapper-name <n>   Compatibility wrapper name. Default: ccb-ssh
  --remote-goos <value>       GOOS for remote shim build. Default: linux
  --remote-goarch <value>     GOARCH for remote shim build. Default: amd64
  --manual-ssh-config         Do not edit ~/.ssh/config; print manual Include instruction instead
  --dry-run                   Print planned actions without writing files or using ssh/scp
  --help                      Show this help
EOF
}

info() {
	printf '%s\n' "$*" >&2
}

fail() {
	info "error: $*"
	exit 1
}

run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '[dry-run]' >&2
		for arg in "$@"; do
			printf ' %s' "$arg" >&2
		done
		printf '\n' >&2
		return 0
	fi

	"$@"
}

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		fail "required command not found: $1"
	fi
}

append_host() {
	if [ -z "$HOSTS" ]; then
		HOSTS="$1"
	else
		HOSTS="$HOSTS $1"
	fi
}

sanitize_component() {
	sanitized=$(printf '%s' "$1" | sed 's/[^A-Za-z0-9._-]/-/g; s/^-*//; s/-*$//')
	if [ -z "$sanitized" ]; then
		printf 'unknown'
		return 0
	fi

	printf '%s' "$sanitized"
}

generate_token() {
	if [ "$DRY_RUN" -eq 1 ]; then
		info "[dry-run] generate token -> $TOKEN_FILE"
		return 0
	fi

	mkdir -p "$(dirname "$TOKEN_FILE")"
	openssl rand -hex 32 >"$TOKEN_FILE"
	chmod 600 "$TOKEN_FILE"
	info "generated token: $TOKEN_FILE"
}

build_local_binaries() {
	run mkdir -p "$LOCAL_BIN_DIR"
	run go build -o "$LOCAL_BIN_DIR/clip-bridge" ./cmd/clip-bridge
	run go build -o "$LOCAL_BIN_DIR/clip-bridge-start" ./cmd/clip-bridge-start
}

stage_remote_binary() {
	STAGE_DIR="$CONFIG_DIR/bin"
	REMOTE_ARTIFACT="$STAGE_DIR/xclip-${REMOTE_GOOS}-${REMOTE_GOARCH}"
	REMOTE_SHA_FILE="$STAGE_DIR/xclip-${REMOTE_GOOS}-${REMOTE_GOARCH}.sha256"

	if [ "$DRY_RUN" -eq 1 ]; then
		info "[dry-run] build remote shim GOOS=$REMOTE_GOOS GOARCH=$REMOTE_GOARCH -> $REMOTE_ARTIFACT"
		return 0
	fi

	mkdir -p "$STAGE_DIR"
	env GOOS="$REMOTE_GOOS" GOARCH="$REMOTE_GOARCH" go build -o "$REMOTE_ARTIFACT" ./cmd/xclip
	shasum -a 256 "$REMOTE_ARTIFACT" | awk '{print $1}' >"$REMOTE_SHA_FILE"
}

deploy_remote_binary() {
	host="$1"
	remote_bin="$REMOTE_BIN_DIR/xclip"
	remote_state_dir=".config/claude-clip-bridge"
	remote_sha="$remote_state_dir/xclip.sha256"
	remote_token="$remote_state_dir/token"
	local_sha=$(cat "$REMOTE_SHA_FILE")

	run ssh \
		-F "$BASE_SSH_CONFIG" \
		-o PermitLocalCommand=no \
		-o ClearAllForwardings=yes \
		-o ControlMaster=no \
		-o ControlPath=none \
		"$host" \
		"mkdir -p '$REMOTE_BIN_DIR' '$remote_state_dir'"
	run scp \
		-F "$BASE_SSH_CONFIG" \
		-o ClearAllForwardings=yes \
		-o ControlMaster=no \
		-o ControlPath=none \
		"$REMOTE_ARTIFACT" \
		"$host:$remote_bin"
	run scp \
		-F "$BASE_SSH_CONFIG" \
		-o ClearAllForwardings=yes \
		-o ControlMaster=no \
		-o ControlPath=none \
		"$TOKEN_FILE" \
		"$host:$remote_token"
	run ssh \
		-F "$BASE_SSH_CONFIG" \
		-o PermitLocalCommand=no \
		-o ClearAllForwardings=yes \
		-o ControlMaster=no \
		-o ControlPath=none \
		"$host" \
		"chmod 755 '$remote_bin' && chmod 600 '$remote_token' && printf '%s\n' '$local_sha' > '$remote_sha'"
}

write_host_fragment() {
	host="$1"
	fragment_path="$HOSTS_DIR/$(sanitize_component "$host").conf"
	include_local_command="CLIP_BRIDGE_BIN=$LOCAL_BIN_DIR/clip-bridge CC_BRIDGE_PORT=$BRIDGE_PORT $LOCAL_BIN_DIR/clip-bridge-start %h \$PPID"

	if [ "$DRY_RUN" -eq 1 ]; then
		info "[dry-run] write managed host fragment $fragment_path"
		return 0
	fi

	mkdir -p "$HOSTS_DIR"
	cat >"$fragment_path" <<EOF
# Managed by claude-clip-bridge/install.sh
Host $host
    ControlMaster no
    ControlPath none
    PermitLocalCommand yes
    LocalCommand $include_local_command
    RemoteForward $BRIDGE_PORT localhost:$BRIDGE_PORT
    ExitOnForwardFailure yes
EOF
}

ensure_include() {
	include_line="Include $HOSTS_DIR/*.conf"
	marker="# claude-clip-bridge managed include"

	if [ "$AUTO_EDIT_SSH_CONFIG" -ne 1 ]; then
		info "manual SSH config step required:"
		info "  add this line to $BASE_SSH_CONFIG"
		info "  $include_line"
		return 0
	fi

	if [ "$DRY_RUN" -eq 1 ]; then
		info "[dry-run] ensure Include in $BASE_SSH_CONFIG: $include_line"
		return 0
	fi

	base_dir=$(dirname "$BASE_SSH_CONFIG")
	mkdir -p "$base_dir"

	if [ ! -f "$BASE_SSH_CONFIG" ]; then
		: >"$BASE_SSH_CONFIG"
		chmod 600 "$BASE_SSH_CONFIG"
	fi

	backup_path="$BASE_SSH_CONFIG.ccb-backup.$(date +%Y%m%d%H%M%S)"
	cp "$BASE_SSH_CONFIG" "$backup_path" || fail "failed to backup $BASE_SSH_CONFIG"

	tmp_config=$(mktemp "${BASE_SSH_CONFIG}.tmp.XXXXXX")
	trap 'rm -f "$tmp_config"' EXIT INT TERM

	grep -Fvx "$marker" "$BASE_SSH_CONFIG" | grep -Fvx "$include_line" >"$tmp_config" || true

	{
		printf '%s\n' "$marker"
		printf '%s\n' "$include_line"
		printf '\n'
		cat "$tmp_config"
	} >"$BASE_SSH_CONFIG"

	rm -f "$tmp_config"
	trap - EXIT INT TERM
}

write_compat_wrapper() {
	wrapper_path="$LOCAL_BIN_DIR/$COMPAT_WRAPPER_NAME"

	if [ "$DRY_RUN" -eq 1 ]; then
		info "[dry-run] write compatibility wrapper $wrapper_path"
		return 0
	fi

	mkdir -p "$LOCAL_BIN_DIR"
	cat >"$wrapper_path" <<EOF
#!/bin/sh
set -eu
if [ -f "$BASE_SSH_CONFIG" ]; then
    exec ssh -F "$BASE_SSH_CONFIG" "\$@"
fi
exec ssh "\$@"
EOF
	chmod 755 "$wrapper_path"
}

print_summary() {
	include_line="Include $HOSTS_DIR/*.conf"

	cat <<EOF
Installed:
  Local binaries:  $LOCAL_BIN_DIR/clip-bridge, $LOCAL_BIN_DIR/clip-bridge-start
  Remote shim:     $REMOTE_ARTIFACT
  Token:           $TOKEN_FILE (also deployed to each host)
  Host fragments:  $HOSTS_DIR
  SSH config:      $BASE_SSH_CONFIG
  Hosts updated:   $HOSTS
  Bridge port:     $BRIDGE_PORT

Primary usage:
  ssh <configured-host>

Compatibility:
  $LOCAL_BIN_DIR/$COMPAT_WRAPPER_NAME just calls ssh with your base config.

Managed SSH include:
  $include_line

Next:
  1. Ensure $LOCAL_BIN_DIR is in your PATH
  2. Open a new SSH session: ssh <configured-host>
  3. In the remote session, test:
       xclip -selection clipboard -t TARGETS -o
       xclip -selection clipboard -t image/png -o >/tmp/out.png
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--host)
		[ "$#" -ge 2 ] || fail "--host requires a value"
		append_host "$2"
		shift 2
		;;
	--port)
		[ "$#" -ge 2 ] || fail "--port requires a value"
		BRIDGE_PORT="$2"
		shift 2
		;;
	--local-bin-dir)
		[ "$#" -ge 2 ] || fail "--local-bin-dir requires a value"
		LOCAL_BIN_DIR="$2"
		shift 2
		;;
	--remote-bin-dir)
		[ "$#" -ge 2 ] || fail "--remote-bin-dir requires a value"
		REMOTE_BIN_DIR="$2"
		shift 2
		;;
	--config-dir)
		[ "$#" -ge 2 ] || fail "--config-dir requires a value"
		CONFIG_DIR="$2"
		HOSTS_DIR="$CONFIG_DIR/hosts"
		TOKEN_FILE="$CONFIG_DIR/token"
		shift 2
		;;
	--base-ssh-config)
		[ "$#" -ge 2 ] || fail "--base-ssh-config requires a value"
		BASE_SSH_CONFIG="$2"
		shift 2
		;;
	--compat-wrapper-name)
		[ "$#" -ge 2 ] || fail "--compat-wrapper-name requires a value"
		COMPAT_WRAPPER_NAME="$2"
		shift 2
		;;
	--remote-goos)
		[ "$#" -ge 2 ] || fail "--remote-goos requires a value"
		REMOTE_GOOS="$2"
		shift 2
		;;
	--remote-goarch)
		[ "$#" -ge 2 ] || fail "--remote-goarch requires a value"
		REMOTE_GOARCH="$2"
		shift 2
		;;
	--manual-ssh-config)
		AUTO_EDIT_SSH_CONFIG=0
		shift
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--help|-h)
		usage
		exit 0
		;;
	*)
		fail "unknown option: $1"
		;;
	esac
done

[ -n "$HOSTS" ] || fail "at least one --host is required"

require_cmd go
require_cmd ssh
require_cmd scp
require_cmd shasum
require_cmd openssl

generate_token
build_local_binaries
stage_remote_binary

for host in $HOSTS; do
	deploy_remote_binary "$host"
	write_host_fragment "$host"
done

ensure_include
write_compat_wrapper
print_summary

#!/usr/bin/env sh
# claude-tools SessionStart bootstrap: sets plugin-foundation's download-script.sh
# PF_* env contract (see download-script.sh's own header) to this plugin's name, data
# dir and pinned version, then execs it unmodified. All provisioning logic lives in
# download-script.sh; this file only supplies claude-tools' own values.
#
# Known gap: download-script.sh's release layout is
# <base>/<name>-v<version>/<name>-<version>-<arch>, which needs the tag itself to read
# "claude-tools-v<version>" to resolve against GitHub Releases' flat asset namespace.
# SC-VERSIONING requires a bare "vX.Y.Z" tag for this repo (its one Go module lives at
# the repo root, so its path-from-root is empty -- see release/guard/tag-prefix.sh).
# Until a release-side mirroring layer reconciles that, a live fetch here 404s and
# download-script.sh takes its documented soft-fail path (exit 1, no export) --
# forced-use-hook.sh then fails open to the raw tool, same as any other CLI that
# isn't installed. Nothing crashes; nothing silently claims success.
set -eu

if [ -z "${CLAUDE_PLUGIN_ROOT:-}" ] || [ -z "${CLAUDE_PLUGIN_DATA:-}" ]; then
  echo "claude-tools bootstrap: CLAUDE_PLUGIN_ROOT/CLAUDE_PLUGIN_DATA not set -- skipping (not running under the plugin runtime?)" >&2
  exit 0
fi

export PF_CLI_NAME="claude-tools"
export PF_PLUGIN_DATA="${CLAUDE_PLUGIN_DATA}"
export PF_VERSION_FILE="${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json"
export PF_RELEASE_BASE_URL="${CLAUDE_TOOLS_RELEASE_BASE_URL:-https://github.com/johnrichter/claude-tools/releases/download}"
export PF_ENV_FILE="${CLAUDE_ENV_FILE:-}"

exec "${CLAUDE_PLUGIN_ROOT}/hooks/download-script.sh"

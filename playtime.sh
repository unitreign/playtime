#!/bin/sh
# PlayTime launcher for Knulli, installed under /userdata/roms/tools.
# EmulationStation runs this through configgen which injects
# SDL_GAMECONTROLLERCONFIG — do not overwrite it here.

# shellcheck shell=sh
set -eu

if [ "${PLAYTIME_LAUNCHER_RELOCATED:-}" = "1" ]; then
  DIR="$PLAYTIME_LAUNCHER_DIR"
  trap 'rm -f "$PLAYTIME_LAUNCHER_TEMP"' 0
else
  DIR="$(CDPATH='' cd "$(dirname "$0")" && pwd)"
  export PLAYTIME_LAUNCHER_DIR="$DIR"
  PLAYTIME_LAUNCHER_TEMP=""
  if PLAYTIME_LAUNCHER_TEMP="$(umask 077; mktemp "${TMPDIR:-/tmp}/playtime-launch.XXXXXX" 2>/dev/null)" &&
    cp -f "$0" "$PLAYTIME_LAUNCHER_TEMP" 2>/dev/null; then
    export PLAYTIME_LAUNCHER_TEMP
    export PLAYTIME_LAUNCHER_RELOCATED=1
    if exec /bin/sh "$PLAYTIME_LAUNCHER_TEMP"; then
      exit 0
    fi
    unset PLAYTIME_LAUNCHER_RELOCATED
  fi
  [ -z "$PLAYTIME_LAUNCHER_TEMP" ] || rm -f "$PLAYTIME_LAUNCHER_TEMP"
  unset PLAYTIME_LAUNCHER_TEMP
  export PLAYTIME_LAUNCHER_OPEN=1
fi
cd "$DIR" || exit 1

export CFW=KNULLI

while :; do
  "$DIR/playtime" hooks repair >/dev/null 2>&1 || true
  if LD_LIBRARY_PATH="$DIR/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}" "$DIR/playtime"; then
    status=0
  else
    status=$?
  fi
  [ "$status" -eq 75 ] || exit "$status"
done

#!/bin/sh
# Installed by PlayTime into /userdata/system/scripts/
# Batocera/Knulli ES calls every script in scripts/ for every event:
#   $1=action  $2=system  $3=emulator  $4=rom_path  $5=gameinfo_json
set -eu
APPDIR="$(CDPATH='' cd "$(dirname "$0")/.." && pwd)"
case "$1" in
  gameStart)
    "$APPDIR/roms/tools/PlayTime/playtime" session start "$2" "$4" "$(basename "$4")" &
    ;;
  gameStop)
    "$APPDIR/roms/tools/PlayTime/playtime" session end "$2" "$4" "$(basename "$4")"
    ;;
esac

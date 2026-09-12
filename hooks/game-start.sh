#!/bin/sh
# Called by EmulationStation/Knulli on game launch.
# $1=system  $2=rom_path  $3=rom_name
set -eu
APPDIR="$(CDPATH='' cd "$(dirname "$0")/.." && pwd)"
# Run in background — must not block ES from launching the game
"$APPDIR/playtime" session start "$1" "$2" "$3" &

#!/bin/sh
# Called by EmulationStation/Knulli when game exits.
# $1=system  $2=rom_path  $3=rom_name
set -eu
APPDIR="$(CDPATH='' cd "$(dirname "$0")/.." && pwd)"
"$APPDIR/playtime" session end "$1" "$2" "$3"

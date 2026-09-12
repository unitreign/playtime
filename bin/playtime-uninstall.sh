#!/bin/sh
# Remove PlayTime hooks from EmulationStation scripts before deleting the app.
set -eu

APPDIR="$(CDPATH='' cd "$(dirname "$0")/.." && pwd)"
export CFW=KNULLI
exec "$APPDIR/playtime" uninstall "$@"

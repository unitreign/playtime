# PlayTime

Gameplay time tracker for **Knulli CFW** on Anbernic handhelds.

<p align="center">
  <a href="https://github.com/unitreign/playtime/releases/latest"><img src="https://img.shields.io/github/v/release/unitreign/playtime?label=version&color=black" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-GPL--3.0-black" alt="License: GPL v3"></a>
  <a href="https://ko-fi.com/unitreign"><img src="https://img.shields.io/badge/Ko--fi-support-ff5e5b?logo=ko-fi&logoColor=white" alt="Ko-fi"></a>
</p>

Knulli has no built-in play time tracking. PlayTime adds it. A hook script records every session automatically, and a shelf-style UI shows your most-played games sorted by time — across all consoles or filtered to one.

## Install

1. Download the latest release from the [Releases](https://github.com/unitreign/playtime/releases) page.
2. Extract and copy the `PlayTime` folder to `/userdata/roms/tools/` on your device.
3. Refresh Knulli's game list so the tool appears under **Tools**.
4. Launch **PlayTime** from the Tools menu. It installs its hooks and opens the UI.

No reboot required. Tracking starts immediately for any game launched after installation.

## Features

### Time Tracking

* Hooks into Knulli's `gameStart` and `gameStop` events
* Every session is recorded with system, game name, start time, and duration
* Short or accidental launches are filtered out automatically
* Play history survives firmware updates — data lives in `/userdata/system/configs/playtime/`

### Library View

* **All Games** — every game you have played, sorted by total play time
* **Per-console views** — switch between consoles with L and R to see that system's games only
* Game names pulled from each system's `gamelist.xml`; cover art loaded from the same source
* Session count and average session length shown alongside total time

### UI

* Monochrome design — no colour, no clutter
* Adapts to any screen resolution — tested on RG34XX and RG35XXSP
* Controller-native: no touch required
* B to exit, L / R to switch consoles

## How It Works

PlayTime installs a single hook script at `/userdata/system/scripts/playtime-hook.sh`. Knulli calls every script in that directory for every game event.

When a game starts, the hook calls `playtime session start` in the background. When the game exits, it calls `playtime session end`. Sessions are stored in a SQLite database at `/userdata/system/configs/playtime/playtime.db`.

ROM paths are always under `/userdata/roms/`. Core and emulator paths are under `/usr/lib/` or `/usr/bin/`. PlayTime identifies the ROM from the positional arguments by looking for a path that starts with `/userdata/roms/` — no emulator name lists, no hardcoding.

The UI reads the database and the system gamelists, then renders a sorted shelf of your games.

## Uninstall

**Option 1 — command:**

Run `playtime uninstall` from a shell. This removes the hook script. Then delete these two folders manually to remove everything:

* `/userdata/roms/tools/PlayTime/` — the app
* `/userdata/system/configs/playtime/` — all play history

**Option 2 — manual:**

Delete both folders above directly. Also check `/userdata/system/scripts/` and remove `playtime-hook.sh` if it is there.

## System Details

|               |                                                           |
| ------------- | --------------------------------------------------------- |
| **Firmware**  | Knulli CFW (Batocera-based)                               |
| **Devices**   | All Knulli-supported devices · Tested on RG34XX and RG35XXSP |
| **Data**      | SQLite · `/userdata/system/configs/playtime/playtime.db`  |
| **Hooks**     | `/userdata/system/scripts/playtime-hook.sh`               |
| **Install**   | `/userdata/roms/tools/PlayTime/`                          |
| **License**   | GPL-3.0                                                   |

## License

PlayTime is licensed under the **GNU General Public License v3.0**.

See [LICENSE](LICENSE) for the full license text.

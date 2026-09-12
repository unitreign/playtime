BINARY   := playtime
CROSS_CC := aarch64-linux-gnu-gcc

# Host build (Linux/macOS only — SDL2 must be installed)
.PHONY: build
build:
	go build -o $(BINARY) .

# Cross-compile for Knulli/Batocera ARM64
# Requires: sudo apt install gcc-aarch64-linux-gnu libsdl2-dev
.PHONY: build-arm64
build-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=$(CROSS_CC) \
		go build -ldflags="-s -w" -o $(BINARY) .

# Bundle for device — mirrors retroshelf's on-device layout:
#   /userdata/roms/tools/PlayTime/
#       playtime.sh        ← ES entry point
#       playtime           ← binary
#       font.ttf
#       bin/
#           playtime-uninstall.sh
#       icons/
#           playtime.png   ← add manually before shipping
#       lib/               ← add any bundled .so files here if needed
.PHONY: dist
dist: build-arm64
	rm -rf dist/PlayTime
	mkdir -p dist/PlayTime/bin dist/PlayTime/icons dist/PlayTime/lib
	cp playtime        dist/PlayTime/
	cp playtime.sh     dist/PlayTime/
	cp bin/playtime-uninstall.sh dist/PlayTime/bin/
	@if [ -f font.ttf ]; then cp font.ttf dist/PlayTime/; else echo "WARNING: font.ttf not found — add one before deploying"; fi
	@if [ -f icons/playtime.png ]; then cp icons/playtime.png dist/PlayTime/icons/; fi
	chmod +x dist/PlayTime/playtime dist/PlayTime/playtime.sh dist/PlayTime/bin/playtime-uninstall.sh
	@echo ""
	@echo "Copy dist/PlayTime/ to /userdata/roms/tools/PlayTime/ on device."
	@echo "First launch via ES will auto-install the hooks."

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -rf dist/

.PHONY: tidy
tidy:
	go mod tidy

MAKEFILE_DIR := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
INSTALL_DIR := $(MAKEFILE_DIR)/bin

# Default to the host GOOS so native builds pick the correct executable suffix; override with
# `make GOOS=windows` to cross-compile for Windows from another platform.
GOOS ?= $(shell go env GOOS)
export GOOS

ifeq ($(GOOS),windows)
	EXE := .exe
else
	EXE :=
endif

# Release builds strip debug symbols and the DWARF table with `-s -w` to shrink the binaries. Set
# RELEASE=true (also accepts 1, TRUE, True) for release builds; plain `make` keeps symbols for
# local debugging. The GitHub release workflow sets RELEASE=true.
ifneq (,$(filter $(RELEASE),true 1 TRUE True))
	LDFLAGS := -s -w
else
	LDFLAGS :=
endif

install:
	mkdir -p $(INSTALL_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(INSTALL_DIR)/git-auto-sync$(EXE) .
	cd daemon && go build -ldflags "$(LDFLAGS)" -o $(INSTALL_DIR)/git-auto-sync-daemon$(EXE) .

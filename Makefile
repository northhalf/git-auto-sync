MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
INSTALL_DIR= $(MAKEFILE_DIR)/bin

# Default to the host GOOS so native builds pick the correct executable suffix; override with
# `make GOOS=windows` to cross-compile for Windows from another platform.
GOOS ?= $(shell go env GOOS)
export GOOS

ifeq ($(GOOS),windows)
    EXE := .exe
else
    EXE :=
endif

install:
	mkdir -p $(INSTALL_DIR)
	go build -o $(INSTALL_DIR)/git-auto-sync$(EXE) .
	cd daemon && go build -o $(INSTALL_DIR)/git-auto-sync-daemon$(EXE) .

MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
INSTALL_DIR= $(MAKEFILE_DIR)/bin

install:
	mkdir -p $(INSTALL_DIR)
	go build -o $(INSTALL_DIR)/git-auto-sync .
	cd daemon && go build -o $(INSTALL_DIR)/git-auto-sync-daemon .

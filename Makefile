.PHONY: build install dev

build:
	go build -o ./dist/ssh-pve .

install:
	cp ./dist/ssh-pve ~/.local/bin/

# Run against the repo-local config in ./.dev/ssh-pve/ (gitignored) instead of ~/.config.
dev:
	XDG_CONFIG_HOME=$(CURDIR)/.dev go run .

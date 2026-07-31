.PHONY: build install

build:
	go build -o ./dist/ssh-pve .

install:
	cp ./dist/ssh-pve ~/.local/bin/
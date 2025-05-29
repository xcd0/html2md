BIN           := ./html2md
VERSION       := 0.0.2
FLAGS_VERSION := -X main.version=$(VERSION) -X main.revision=$(git rev-parse --short HEAD)
FLAG          := -a -tags netgo -trimpath -ldflags='-s -w -extldflags="-static" $(FLAGS_VERSION) -buildid='

ifeq ($(OS),Windows_NT)
	BIN := $(BIN).exe
endif

all:
	cat ./makefile
build:
	go build
release:
	go build $(FLAG)
	make upx 
	@echo Success!
upx:
	upx --lzma $(BIN)


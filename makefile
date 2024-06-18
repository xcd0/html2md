BIN           := ./html2md
REVISION      := `git rev-parse --short HEAD`
FLAG          :=  -a -tags netgo -trimpath -ldflags='-s -w -extldflags="-static" -buildid='

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


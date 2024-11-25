TARGET := $(shell basename $(CURDIR))

DELIVERABLES := ${TARGET}.exe ${TARGET}.linux ${TARGET}.macos ${TARGET}.m1mac README.md

BUILDTIME := $(shell date +"%Y-%m-%dT%H:%M:%S%z")
COMMITID := $(shell git rev-parse --short HEAD)
CHECKOUTS := $(shell git status --porcelain)
ifneq ($(CHECKOUTS),)
	COMMITID:=${COMMITID}-dirty
endif
VERSIONID=$(shell grep "cmdToolVersion = " "main.go" | sed 's/.*"\(.*\)".*/\1/')

SRC=main.go
BUILD_FLAGS=-ldflags '-X "main.buildTime='${BUILDTIME}'" -X "main.commitID='${COMMITID}'"'

.PHONY: all
all: hello clean vet build

.PHONY: hello
hello:
	@echo TARGET=${TARGET}
	@echo BUILDTIME=${BUILDTIME}
	@echo COMMITID=${COMMITID}
	@echo VERSIONID=${VERSIONID}

.PHONY: vet
vet:
	go vet

.PHONY: test
test: ${TARGET}
	./${TARGET} testdata/*.vcf
	open testdata/*.png

.PHONY: test-clean
test-clean:
	rm testdata/*.png

.PHONY: windows
windows: ${TARGET}.exe

.PHONY: linux
linux: ${TARGET}.linux

.PHONY: macos
macos: ${TARGET}.macos ${TARGET}.m1mac

.PHONY: binaries
binaries: ${TARGET}.exe ${TARGET}.linux ${TARGET}.macos ${TARGET}.m1mac

.PHONY: zip
zip: ${TARGET}.zip

.PHONY: install
install: instclean
	go install ${BUILD_FLAGS}

.PHONY: clean instclean
clean:
	-rm -f ${TARGET} ${TARGET}.exe ${TARGET}.linux ${TARGET}.macos ${TARGET}.m1mac
	-rm -f ${TARGET}.zip

instclean:
	go clean -i

.PHONY: build
build: ${TARGET}

${TARGET}: $(SRC)
	go build ${BUILD_FLAGS}

${TARGET}.exe: $(SRC)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ${BUILD_FLAGS} -o $@

${TARGET}.linux: $(SRC)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ${BUILD_FLAGS} -o $@

${TARGET}.macos: $(SRC)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ${BUILD_FLAGS} -o $@

${TARGET}.m1mac: $(SRC)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ${BUILD_FLAGS} -o $@

${TARGET}.zip: $(DELIVERABLES)
	zip -r $@ $?

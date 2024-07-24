all:	build/linux-amd64/sdcm	build/macos-amd64/sdcm	build/windows-amd64/sdcm.exe	build/macos-arm64/sdcm
	@echo "Done"
.PHONY : all

# override with 'make BUILD=Release'
BUILD := Debug

DROPTS=
ifeq ($(BUILD),Release)
  DROPTS=-w -s
endif

build/linux-amd64/sdcm: sdcm.go
	env GOOS=linux GOARCH=amd64 go build -ldflags "$(DROPTS) -X main.compileDate=`date -u +.%Y%m%d.%H%M%S`" -o build/linux-amd64/sdcm sdcm.go
	chmod +x build/linux-amd64/sdcm

build/macos-amd64/sdcm: sdcm.go
	env GOOS=darwin GOARCH=amd64 go build -ldflags "$(DROPTS) -X main.compileDate=`date -u +.%Y%m%d.%H%M%S`" -o build/macos-amd64/sdcm sdcm.go
	chmod +x build/macos-amd64/sdcm

build/windows-amd64/sdcm.exe: sdcm.go
	env GOOS=windows GOARCH=amd64 go build -ldflags "$(DROPTS) -X main.compileDate=`date -u +.%Y%m%d.%H%M%S`" -o build/windows-amd64/sdcm.exe sdcm.go

build/macos-arm64/sdcm: sdcm.go
	env GOOS=darwin GOARCH=arm64 go build -ldflags "$(DROPTS) -X main.compileDate=`date -u +.%Y%m%d.%H%M%S`" -o build/macos-arm64/sdcm sdcm.go

# Prepare release archives with binaries.
release:
	mkdir -p release
	rm -rf release/*

	GOARCH=amd64 GOOS=linux go build -o release/bandcamp-download .
	cd release && tar czf bandcamp-download.amd64.tar.gz bandcamp-download
	rm release/bandcamp-download

	GOARCH=386 GOOS=linux go build -o release/bandcamp-download .
	cd release && tar czf bandcamp-download.386.tar.gz bandcamp-download
	rm release/bandcamp-download

	GOARCH=amd64 GOOS=windows go build -o release/bandcamp-download.exe .
	cd release/ && zip bandcamp-download.amd64.zip bandcamp-download.exe
	rm release/bandcamp-download.exe

	GOARCH=386 GOOS=windows go build -o release/bandcamp-download.exe .
	cd release/ && zip bandcamp-download.386.zip bandcamp-download.exe
	rm release/bandcamp-download.exe

# This is required for building windows binaries from linux.
install-windows-dependencies:
	GOOS=windows go get github.com/konsorten/go-windows-terminal-sequences
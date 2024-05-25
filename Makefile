all: bloki

bloki: *.go
	go build .

cross:
	GOOS=linux GOARCH=amd64 go build -a -o bloki-amd64-linux .
	GOOS=linux GOARCH=arm go build -a -o bloki-arm-linux .
	GOOS=linux GOARCH=arm64 go build -a -o bloki-arm64-linux .
	GOOS=darwin GOARCH=amd64 go build -a -o bloki-amd64-macos .
	GOOS=darwin GOARCH=arm64 go build -a -o bloki-arm64-macos .
	GOOS=freebsd GOARCH=amd64 go build -a -o bloki-amd64-freebsd .
	GOOS=openbsd GOARCH=amd64 go build -a -o bloki-amd64-openbsd .
	GOOS=netbsd GOARCH=amd64 go build -a -o bloki-amd64-netbsd .
	GOOS=solaris GOARCH=amd64 go build -a -o bloki-adm64-solaris .
	#GOOS=aix GOARCH=ppc64 go build -a -o bloki-ppc64-aix .
	GOOS=plan9 GOARCH=amd64 go build -a -o bloki-amd64-plan9 .
	GOOS=plan9 GOARCH=arm go build -a -o bloki-arm-plan9 .
	GOOS=windows GOARCH=amd64 go build -a -o bloki-amd64-win64.exe
	GOOS=windows GOARCH=arm64 go build -a -o bloki-arm64-win64.exe

docker:
	GOOS=linux GOARCH=amd64 go build -a -o bloki-amd64-linux .
	GOOS=linux GOARCH=arm64 go build -a -o bloki-arm64-linux .
	docker buildx build --platform linux/amd64,linux/arm64 -t tenox7/bloki:latest --push .

clean:
	rm -f bloki-* bloki
default: build

build:
	go build -v -tags netgo,osusergo -o binaries/lbfeedback agent/lbfeedback.go

tar:
	make build
	tar -zcvf binaries/lbfeedback-linux-x86_64-current.tar.gz binaries/lbfeedback LICENSE README.md

clean:
	go clean
	rm -f binaries/lbfeedback binaries/lbfeedback-linux-x86_64-current.tar.gz

#!/bin/bash
go build -v -tags netgo,osusergo lbfeedback.go
tar -zcvf ../binaries/lbfeedback-linux-x86_64-current.tar.gz lbfeedback ../LICENSE ../README.md
exit
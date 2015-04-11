# LogChecker

[![GoDoc](https://godoc.org/github.com/z0rr0/logchecker/logchecker?status.svg)](https://godoc.org/github.com/z0rr0/logchecker/logchecker) [![Build Status](https://travis-ci.org/z0rr0/logchecker.svg?branch=master)](https://travis-ci.org/z0rr0/logchecker)

It is a simple library to check a list of logs files and send notification about their abnormal activities.

**IMPORTANT:** _**It is in development now.**_

### Usage

Only Linux is supported now.

API descriptions can be found on [godoc.org](http://godoc.org/github.com/z0rr0/logchecker/logchecker).

```go
import "logchecker"
import "log"
// ...

logger := logchecker.New()
if err := logchecker.InitConfig(logger, "config.json"); err != nil {
    log.Panicf("logchecker error: %v\n", err)
}
```


### Configuration

Files for observation can be added using a configuration file, see examples in [config.example.json](https://github.com/z0rr0/logchecker/blob/master/config.example.json).

```

### Testing

Use standard Go testing mechanism:

```shell
cd $GOPATH/src/github.com/z0rr0/logchecker
go test
```

There is a [nice article](http://blog.golang.org/cover) about tests covering.

### Dependencies

* standard [Go library](http://golang.org/pkg/)
* [inotify](https://godoc.org/golang.org/x/exp/inotify) package

### Design guidelines

There are recommended style guides:

* [The Go Programming Language Specification](https://golang.org/ref/spec)
* [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

A compliance with the second style guide can be checked using [go-lint](http://go-lint.appspot.com/github.com/z0rr0/logchecker) tool.

### License

This source code is governed by a [LGPLv3](https://www.gnu.org/licenses/lgpl-3.0.txt) license that can be found in the [LICENSE](https://github.com/z0rr0/logchecker/blob/master/LICENSE) file.

<img src="https://www.gnu.org/graphics/lgplv3-147x51.png" title="LGPLv3 logo">

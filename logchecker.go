// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// LogChecker package is a simple library to check a list of logs files
// and send notification about their abnormal activities.
//
// Error logger is activated by default,
// use DebugMode method to turn on debug mode:
//
//  DebugMode(true)
//
//
package logchecker
import (
    "os"
    "log"
    // "fmt"
    "io/ioutil"
)

var (
    LoggerError *log.Logger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
    LoggerDebug *log.Logger = log.New(ioutil.Discard, "DEBUG: ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
)

type LogChecker struct {
    Name string
}

// Initialization of Logger handlers
func DebugMode(debugmode bool) {
    debugHandle := ioutil.Discard
    if debugmode {
        debugHandle = os.Stdout
    }
    LoggerDebug = log.New(debugHandle, "DEBUG: ",
        log.Ldate|log.Lmicroseconds|log.Lshortfile)
}

func New() *LogChecker {
    return &LogChecker{}
}

// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// LogChecker testing methods
//
package logchecker

import (
    "os"
    // "fmt"
    "testing"
    "path/filepath"
)

func TestDebugMode(t *testing.T) {
    if (LoggerError == nil) || (LoggerDebug == nil) {
        t.Errorf("Incorrect references")
    }
    DebugMode(false)
    if (LoggerError.Prefix() != "LogChecker ERROR: ") || (LoggerDebug.Prefix() != "LogChecker DEBUG: ") {
        t.Errorf("Incorrect loggers settings")
    }
    DebugMode(true)
    if (LoggerError.Flags() != 19) || (LoggerDebug.Flags() != 21) {
        t.Errorf("Incorrect loggers settings")
    }
}

func TestNew(t *testing.T) {
    obj := New()
    if obj == nil {
        t.Errorf("Incorrect reference")
    }
}

func TestFilePath(t *testing.T) {
    if _, err := FilePath("invalid_name"); err == nil {
        t.Errorf("Incorrect response")
    }
    if _, err := FilePath(""); err == nil {
        t.Errorf("Incorrect response")
    }
    pwd := os.Getenv("PWD")
    os.Setenv("PWD", "")
    if _, err := FilePath("unknown"); err == nil {
        t.Errorf("Incorrect response")
    }
    os.Setenv("PWD", pwd)
    realfile := filepath.Join(os.Getenv("GOPATH"), "src/github.com/z0rr0/logchecker/config.example.json")
    if path, err := FilePath(realfile); err != nil {
        t.Errorf("Incorrect response, the file should exist")
    } else {
        if path != realfile {
            t.Errorf("Ivalid paths")
        }
    }
}

func TestInitConfig(t *testing.T) {
    logger := New()
    example := filepath.Join(os.Getenv("GOPATH"), "src/github.com/z0rr0/logchecker/config.example.json")
    if err := InitConfig(logger, example); err != nil {
        t.Errorf("Error during InitConfig")
    }
    if l := len(logger.Cfg.String()); l <= 0 {
        t.Errorf("Error, config should be initiated")
    }
    if err := InitConfig(logger, "invalid_name"); err == nil {
        t.Errorf("Need error during InitConfig")
    }
    if err := InitConfig(logger, "/etc/shadow"); err == nil {
        t.Errorf("Need permissions error during InitConfig")
    }
    if err := InitConfig(logger, "/etc/passwd"); err == nil {
        t.Errorf("Need json error during InitConfig")
    }
}

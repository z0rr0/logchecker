// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// Main package
//
package main

import (
    "os"
    "fmt"
    "time"
    "flag"
    "sync"
    "syscall"
    "os/signal"
    "golang.org/x/exp/inotify"
    "github.com/z0rr0/logchecker/logchecker"
)

const (
    Config string = "config.json"
    // Period time.Duration = 60 * time.Minute
    Period time.Duration = 30 * time.Second
)

var (
    Version string = "uknown"
)

func main() {
    var group sync.WaitGroup
    defer func() {
        if r := recover(); r != nil {
            logchecker.LoggerError.Println(r)
            fmt.Println("Program is terminated abnormally.")
        }
    }()

    debug := flag.Bool("debug", false, "debug mode")
    version := flag.Bool("version", false, "show version")
    config := flag.String("config", Config, "configuration file")

    flag.Parse()
    if *version {
        fmt.Println(Version)
        flag.PrintDefaults()
        return
    }
    logchecker.DebugMode(*debug)

    logger := logchecker.New()
    if err := logchecker.InitConfig(logger, *config); err != nil {
        logchecker.LoggerError.Panicln(err)
    }
    logger.Name = "LogChecker"
    logchecker.LoggerDebug.Println(logger.Cfg)

    // process start
    finish, err := logger.Start(&group)
    if err != nil {
        logchecker.LoggerError.Printf("can't start the process: %v\n", err)
        logchecker.LoggerError.Panicln(err)
    }
    // config monitoring
    watcher, err := inotify.NewWatcher()
    if err != nil {
        logchecker.LoggerError.Printf("can't create config watcher: %v\n", err)
        logchecker.LoggerError.Panicln(err)
    }
    if err = watcher.AddWatch(logger.Cfg.Path, inotify.IN_CLOSE_WRITE | inotify.IN_DELETE_SELF); err != nil {
        logchecker.LoggerError.Printf("can't activate config watcher: %v\n", err)
        close(finish)
        group.Wait()
        logchecker.LoggerError.Panicln(err)
    }
    timestat := time.Tick(Period)
    sigchan := make(chan os.Signal, 2)
    signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
    // process event monitor
    for {
        select {
            case <-sigchan:
                logchecker.LoggerInfo.Println("process will be stopped")
                close(finish)
                group.Wait()
                os.Exit(0)
            case event := <-watcher.Event:
                logchecker.LoggerInfo.Println("process will be resarted due to reconfiguration")
                if (event.Mask & inotify.IN_DELETE_SELF) != 0 {
                    watcher, err = logchecker.IsMoved(logger.Cfg.Path, watcher)
                    if err != nil {
                        logchecker.LoggerError.Printf("re-creation watcher error: %v\n", err)
                        logchecker.LoggerError.Panicln(err)
                    }
                }
                if err = logger.Stop(finish, &group); err != nil {
                    logchecker.LoggerError.Panicln(err)
                }
                err = logchecker.InitConfig(logger, logger.Cfg.Path)
                if err != nil {
                    logchecker.LoggerError.Panicln(err)
                }
                finish, err = logger.Start(&group)
                if err != nil {
                    logchecker.LoggerError.Printf("can't start the process: %v\n", err)
                    logchecker.LoggerError.Panicln(err)
                }
            case werr := <-watcher.Error:
                logchecker.LoggerError.Printf("config watcher error: %v\n", werr)
                if err = logger.Stop(finish, &group); err != nil {
                    logchecker.LoggerError.Panicln(err)
                }
                logchecker.LoggerError.Panicln(werr)
            case <- timestat:
                logchecker.LoggerInfo.Printf("statictics: %v", logger)
        }
    }
}

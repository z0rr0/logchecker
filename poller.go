// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// Package logchecker is a simple library to check a list of logs files
// and send notification about their abnormal activities.
//
package logchecker

import (
    "os"
    "fmt"
    "time"
    "bufio"
    "strings"
)

const (
    maxpollers int = 32
    buffsize int = 8
)

type Task struct {
    QLogChecker *LogChecker
    QService *Service
    QFile *File
}

// Stop finishes a logger observation. It changes a state of LogChecker object
// after that it will not run new tasks and notify then incoming queue will be empty
// a work can be finished with any problems.
func (logger *LogChecker) Stop() {
    logger.Active = false
}

// Start starts a logger observation.
func (logger *LogChecker) Start(finished chan bool) {
    var poll_size int = maxpollers
    logger.Active = true
    if len(logger.Cfg.Observed) < poll_size {
        poll_size = len(logger.Cfg.Observed)
    }
    // create incoming and output channels
    pending, complete := make(chan *Task), make(chan *Task)
    // start tasks
    for i := 0; i < poll_size; i++ {
        go Poller(pending, complete)
    }
    // put tasks to pending channel
    go func() {
        for _, serv := range logger.Cfg.Observed {
            for _, f := range serv.Files {
                if err := f.Validate(); err != nil {
                    LoggerError.Printf("incorrect file was skipped [%v / %v]\n", serv.Name, f.Base())
                } else {
                    pending <- &Task{logger, &serv, &f}
                    LoggerDebug.Printf("=> added in pending [%v, %v, %v]\n", logger.Name, serv.Name, f.Base())
                }
            }
        }
    }()
    for task := range complete {
        go task.Sleep(pending, finished)
    }
}

// Poller handles incoming task and places it to output channel.
func Poller(in chan *Task, out chan *Task) {
    for t, ok := range in {
        if count, pos, err := t.Poll(); err != nil {
            LoggerDebug.Printf("task was handled incorrect [%v, %v]\n", t.QService.Name, t.QFile.Base())
        } else {
            t.QFile.Pos = pos
            t.log(fmt.Sprintf("<= task is completed (count=%v, pos=%v)", count, pos))
        }
        if t.QLogChecker.Active {
            out <- t
        } else {
            if ok {
                close(in)
            }
        }
    }
}

func (task *Task) log(msg string) {
    LoggerDebug.Printf("%v [%v %v %v]", msg, task.QLogChecker.Name, task.QService.Name, task.QFile.Base())
}

// Poll reads file lines and counts needed from them.
// It skips "pos" lines.
func (task *Task) Poll() (uint, uint, error) {
    var counter, clines uint
    file, err := os.Open(task.QFile.Log)
    if err != nil {
        LoggerError.Printf("can't open file: %v\n", task.QFile.Log)
        return counter, clines, err
    }
    defer file.Close()
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        clines++
        if task.QFile.Pos < clines {
            if line := scanner.Text(); line != "" {
                if len(task.QFile.Pattern) > 0 {
                    if strings.Contains(line, task.QFile.Pattern) {
                        counter++
                    }
                } else {
                    counter++
                }
            }
        }
    }
    return counter, clines, nil
}

// Sleep delays next task running.
func (task *Task) Sleep(done chan *Task, finished chan bool) {
    task.log("sleep called")
    time.Sleep(time.Duration(task.QFile.Delay) * time.Second)
    if task.QLogChecker.Active {
        done <- task
    } else {
        close(done)
        finished <- true
    }
}

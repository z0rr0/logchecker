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
    // MaxPollers is maximum number of task handlers.
    MaxPollers int = 5
)

// Task is an object of logging task.
type Task struct {
    QLogChecker *LogChecker
    QService *Service
    QFile *File
}

// Stop finishes a logger observation. It changes a state of LogChecker object
// after that it will not run new tasks and notify then incoming queue will be empty
// a work can be finished with any problems.
func (logger *LogChecker) Stop() {
    logger.Completed = true
    LoggerDebug.Println("complete flag is set")
}

// Works checks that LogChecker in already running.
func (logger *LogChecker) Works() bool {
    LoggerDebug.Println(logger.Completed, logger.Finished)
    return (!logger.Completed) || (!logger.Finished)
}

// Start starts a logger observation.
func (logger *LogChecker) Start(finished chan bool) {
    if logger.Works() {
        finished <- false
        return
    }
    poolSize := MaxPollers
    logger.Completed, logger.Finished = false, false
    if len(logger.Cfg.Observed) < poolSize {
        poolSize = len(logger.Cfg.Observed)
    }
    // create incoming and output channels
    pending, complete := make(chan *Task), make(chan *Task)
    // start tasks
    for i := 0; i < poolSize; i++ {
        go Poller(pending, complete, finished)
    }
    // put tasks to pending channel
    go func() {
        for i, serv := range logger.Cfg.Observed {
            for j, f := range serv.Files {
                if err := f.Validate(); err != nil {
                    LoggerError.Printf("incorrect file was skipped [%v / %v]\n", serv.Name, f.Base())
                } else {
                    serv.Files[j].Begin = time.Now()
                    serv.Files[j].RealLimits = serv.Files[j].Limits
                    serv.Files[j].ModTime = time.Date(1970, time.January, 1, 0, 0, 1, 0, time.UTC )
                    pending <- &Task{logger, &logger.Cfg.Observed[i], &serv.Files[j]}
                }
            }
        }
    }()
    for task := range complete {
        go task.Sleep(pending)
    }
}

// Poller handles incoming task and places it to output channel.
func Poller(in chan *Task, out chan *Task, finished chan bool) {
    var logger *LogChecker
    for {
        t, ok := <-in
        if !ok {
            break
        }
        if logger == nil {
            logger = t.QLogChecker
        }
        logger.InWork++
        t.log("=> poll enter")
        if count, pos, err := t.Poll(); err != nil {
            t.log("task was handled incorrect")
        } else {
            t.QFile.Pos = pos
            t.log(fmt.Sprintf("poll is completed (count=%v, pos=%v)", count, pos))
        }
        t.log("<= poll exit")
        logger.InWork--
        out <- t
    }
    if (logger != nil) && (logger.InWork == 0) {
        finished <- true
    }
}

func (task *Task) log(msg string) {
    LoggerDebug.Printf("%p: [%v %v %v] %v\n", task, task.QLogChecker.Name, task.QService.Name, task.QFile.Base(), msg)
}

// Check validates conditions before sending email notifications.
func (f *File) Check(count uint) (string, error) {
    // period := time.Since(f.Begin)
    return "", nil
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
    // info, err := file.Stat()
    // if err != nil {
    //     return counter, clines, err
    // }
    // mod := time.Since(info.ModTime())
    // if mod. <= task.QFile.ModTime {
    //     task.log("file not changed")
    //     return counter, clines, nil
    // }

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
func (task *Task) Sleep(done chan *Task) {
    if !task.QLogChecker.Completed {
        task.log("sleep")
        time.Sleep(time.Duration(task.QFile.Delay) * time.Second)
        done <- task
    } else {
        task.QLogChecker.mutex.Lock()
        if !task.QLogChecker.Finished {
            task.QLogChecker.Finished = true
            close(done)
        }
        task.QLogChecker.mutex.Unlock()
    }
}

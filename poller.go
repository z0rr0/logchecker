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
    // buffsize int = 8
)

// Task is an object of logging task.
type Task struct {
    Num int
    QLogChecker *LogChecker
    QService *Service
    QFile *File
}

// Stop finishes a logger observation. It changes a state of LogChecker object
// after that it will not run new tasks and notify then incoming queue will be empty
// a work can be finished with any problems.
func (logger *LogChecker) Stop() {
    LoggerDebug.Println("stop command is gooten")
    logger.Completed = true
}

// Start starts a logger observation.
func (logger *LogChecker) Start(finished chan bool) {
    var poolSize = MaxPollers
    logger.Completed = false
    // if len(logger.Cfg.Observed) < poolSize {
    //     poolSize = len(logger.Cfg.Observed)
    // }
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
                    task := Task{logger, &logger.Cfg.Observed[i], &serv.Files[j]}
                    pending <- &task
                    task.log("=> added in pending")
                }
            }
        }
    }
    fmt.Println(tasks)


    go func() {
        for _, task := range tasks {
            pending <- task
            task.log("=> added in pending")
        }

        // for _, serv := range logger.Cfg.Observed {
        //     for _, f := range serv.Files {
        //         if err := f.Validate(); err != nil {
        //             LoggerError.Printf("incorrect file was skipped [%v / %v]\n", serv.Name, f.Base())
        //         } else {
        //             j++
        //             task := Task{j, logger, &serv, &f}
        //             // LoggerDebug.Printf("=> added in pending [%v, %v, %v]\n", logger.Name, serv.Name, f.Base())
        //         }
        //     }
        // }
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
            LoggerDebug.Println("channel was closed")
            break
        }
        if logger == nil {
            logger = t.QLogChecker
        }
        t.log("=> handling start")
        logger.InWork++
        if count, pos, err := t.Poll(); err != nil {
            t.log("task was handled incorrect")
        } else {
            t.QFile.Pos = pos
            t.log(fmt.Sprintf("<= task is completed (count=%v, pos=%v)", count, pos))
        }
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

// Poll reads file lines and counts needed from them.
// It skips "pos" lines.
func (task *Task) Poll() (uint, uint, error) {
    task.log("Poll start")
    time.Sleep(4*time.Second)
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
    task.log("Poll finish")
    return counter, clines, nil
}

// Sleep delays next task running.
func (task *Task) Sleep(done chan *Task) {
    task.log("sleep called")
    if !task.QLogChecker.Completed {
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

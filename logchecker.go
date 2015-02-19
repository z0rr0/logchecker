// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// Package logchecker is a simple library to check a list of logs files
// and send notification about their abnormal activities.
//
// Error logger is activated by default,
// use DebugMode method to turn on debug mode:
//
//     DebugMode(true)
//
// Initialization from file:
//
//     logger := logchecker.New()
//     if err := logchecker.InitConfig(logger, "filiename"); err != nil {
//         // error detected
//     }
//
// Manually initialization of setting to send emails:
//
//     logger := logchecker.New()
//     logger.Cfg.Sender = map[string]string{
//      "user": "user@host.com",
//      "password": "password",
//      "host": "smtp.host.com",
//      "addr": "smtp.host.com:25",
//     }
//
package logchecker

import (
    "os"
    "log"
    "fmt"
    "sync"
    "time"
    "bufio"
    "strings"
    "net/smtp"
    "io/ioutil"
    "encoding/json"
    "path/filepath"
)

const (
    // MaxPollers is maximum number of task handlers.
    MaxPollers int = 5
)

var (
    // LoggerError implements error logger.
    LoggerError = log.New(os.Stderr, "LogChecker ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
    // LoggerDebug implements debug logger, it's disabled by default.
    LoggerDebug = log.New(ioutil.Discard, "LogChecker DEBUG: ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
)

// Backender is an interface to handle data storage operations.
type Backender interface {
    GetName() string
}

// File is a type of settings for a watched file.
type File struct {
    Log string      `json:"file"`
    Delay uint      `json:"delay"`
    Pattern string  `json:"pattern"`
    Boundary uint   `json:"boundary"`
    Increase bool   `json:"increase"`
    Emails []string `json:"emails"`
    Limits [3]uint   `json:"limits"`
    Counter [3]uint
    RealLimits [3]uint
    Pos uint
    Begin time.Time   // 1970-01-01 00:00:01.000 UTC
    ModTime time.Time // 1970-01-01 00:00:00.000 UTC
}

// Service is a type of settings for a watched service.
type Service struct {
    Name string   `json:"name"`
    Files []File  `json:"files"`
}

// Config is main configuration settings.
type Config struct {
    Path string
    Sender map[string]string  `json:"sender"`
    Observed []Service        `json:"observed"`
    Storage string            `json:"storage"`
}

// MemoryBackend is a type for the implementation of memory storage methods.
type MemoryBackend struct {
    Name string
    Active bool
}

// LogChecker is a main object for logging. It is completed
// when stop commnad was called. It is finished when tasks were done
// and the pending channel was closed. LogChecker is finished only
// if it is completed.
type LogChecker struct {
    Name string
    Cfg Config
    Backend Backender
    Completed bool
    Finished bool
    InWork int
    mutex sync.RWMutex
}

// Task is an object of logging task.
type Task struct {
    QLogChecker *LogChecker
    QService *Service
    QFile *File
}

// Check validates conditions before sending email notifications.
func (f *File) Check(count uint) (string, error) {
    // period := time.Since(f.Begin)
    return "", nil
}

// Base returns the last element of log file path.
func (f *File) Base() string {
    // get file name
    return filepath.Base(f.Log)
}

// Validate checks that File is correct: has absolute path and exists.
func (f *File) Validate() error {
    var err error
    if !filepath.IsAbs(f.Log) {
        return fmt.Errorf("path should be absolute")
    }
    _, err = os.Stat(f.Log);
    return err
}

// GetName of MemoryBackend returns a name of the logger back-end.
func (bk *MemoryBackend) GetName() string {
    return bk.Name
}

// String return a details about the configuration.
func (cfg Config) String() string {
    services := make([]string, len(cfg.Observed))
    for i, service := range cfg.Observed {
        // services[i] = fmt.Sprintf("%v", service.Name)
        files := make([]string, len(service.Files))
        for j, file := range service.Files {
            files[j] = fmt.Sprintf("File: %v; Delay: %v; Pattern: %v; Boundary: %v; Increase: %v; Emails: %v; Limits: %v", file.Log, file.Delay, file.Pattern, file.Boundary, file.Increase, file.Emails, file.Limits)
        }
        services[i] = fmt.Sprintf("%v\n\t%v", service.Name, strings.Join(files, "\n\t"))
    }
    return fmt.Sprintf("Config: %v\n sender: %v backend: %v\n---\n%v", cfg.Path, cfg.Sender, cfg.Storage, strings.Join(services, "\n---\n"))
}

// New created new LogChecker object and returns its reference.
func New() *LogChecker {
    res := &LogChecker{}
    res.Name = "LogChecker"
    res.Completed, res.Finished = true, true
    return res
}

// HasService checks that the Service is included to the LogChecker.
// It can use locked mode to guarantee that service array will be
// immutable during reading.
func (logger *LogChecker) HasService(serv *Service, lock bool) bool {
    if lock {
        logger.mutex.RLock()
        defer func() {
            logger.mutex.RUnlock()
        }()
    }
    for _, s := range logger.Cfg.Observed {
        if s.Name == serv.Name {
            return true
        }
    }
    return false
}

// AddService includes a new Service to the LogChecker.
func (logger *LogChecker) AddService(serv *Service) error {
    if logger.Works() {
        return fmt.Errorf("logchecker is already running")
    }
    logger.mutex.Lock()
    defer func() {
        logger.mutex.Unlock()
    }()
    if len(serv.Name) == 0 {
        return fmt.Errorf("service name should not be empty")
    }
    if logger.HasService(serv, false) {
        return fmt.Errorf("service [%v] is already used", serv.Name)
    }
    logger.Cfg.Observed = append(logger.Cfg.Observed, *serv)
    LoggerDebug.Printf("new service is added: %v\n", serv.Name)
    return nil
}

// Validate checks the configuration.
func (logger *LogChecker) Validate() error {
    logger.mutex.RLock()
    defer func() {
        logger.mutex.RUnlock()
    }()
    // check services
    services := map[string]bool{}
    for _, serv := range logger.Cfg.Observed {
        _, ok := services[serv.Name]
        if ok {
            return fmt.Errorf("service names should be unique [%v]", serv.Name)
        }
        services[serv.Name] = true
        for _, f := range serv.Files {
            if err := f.Validate(); err != nil {
                return fmt.Errorf("file is incorrect [%v] %v", f.Log, err)
            }
        }
    }
    // check sender fields
    mandatory := [4]string{"user", "password", "host", "addr"}
    for _, field := range mandatory {
        v, ok := logger.Cfg.Sender[field]
        if !ok {
            return fmt.Errorf("missing sender field [%v]", field)
        }
        if len(v) == 0 {
            return fmt.Errorf("sender field can't be empty [%v]", field)
        }
    }
    // check backend
    var backend Backender
    switch logger.Cfg.Storage {
        case "memory":
            backend = &MemoryBackend{"Memory", true}
    }
    if backend == nil {
        return fmt.Errorf("unknown backend")
    }
    logger.Backend = backend
    return nil
}

// Notify sends a prepared email message.
func (logger *LogChecker) Notify(msg string, to []string) error {
    const mime string = "MIME-version: 1.0;\nContent-Type: text/plain; charset=\"UTF-8\";\n\n";
    content := []byte("Subject: LogChecker notification\n" + mime + msg)
    auth := smtp.PlainAuth(
        "",
        logger.Cfg.Sender["user"],
        logger.Cfg.Sender["password"],
        logger.Cfg.Sender["host"],
    )
    return smtp.SendMail(logger.Cfg.Sender["addr"], auth, logger.Cfg.Sender["user"], to, content)
}

// Works checks that LogChecker in already running.
func (logger *LogChecker) Works() bool {
    LoggerDebug.Println(logger.Completed, logger.Finished)
    return (!logger.Completed) || (!logger.Finished)
}

// Stop finishes a logger observation. It changes a state of LogChecker object
// after that it will not run new tasks and notify then incoming queue will be empty
// a work can be finished with any problems.
func (logger *LogChecker) Stop() {
    logger.Completed = true
    LoggerDebug.Println("complete flag is set")
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
                    pending <- &Task{logger, &logger.Cfg.Observed[i], &serv.Files[j]}
                }
            }
        }
    }()
    for task := range complete {
        go task.Sleep(pending)
    }
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

// Poll reads file lines and counts needed from them.
// It skips "pos" lines.
func (task *Task) Poll() (uint, uint, error) {
    var counter, clines uint
    info, err := os.Stat(task.QFile.Log)
    if err != nil {
        return counter, clines, err
    }
    if task.QFile.ModTime.Equal(info.ModTime()) {
        // file is not chnaged
        return counter, clines, nil
    }
    file, err := os.Open(task.QFile.Log)
    if err != nil {
        LoggerError.Printf("can't open file: %v\n", task.QFile.Log)
        return counter, clines, err
    }
    defer file.Close()
    // read file line by line
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

func (task *Task) log(msg string) {
    LoggerDebug.Printf("%p: [%v %v %v] %v\n", task, task.QLogChecker.Name, task.QService.Name, task.QFile.Base(), msg)
}

// DebugMode is a initialization of Logger handlers.
func DebugMode(debugmode bool) {
    debugHandle := ioutil.Discard
    if debugmode {
        debugHandle = os.Stdout
    }
    LoggerDebug = log.New(debugHandle, "LogChecker DEBUG: ",
        log.Ldate|log.Lmicroseconds|log.Lshortfile)
}

// FilePath validates file name, converts its path from relative to absolute
// using current directory address.
func FilePath(name string) (string, error) {
    var (
        fullpath string
        err error
    )
    fullpath = strings.Trim(name, " ")
    if len(fullpath) < 1 {
        return fullpath, fmt.Errorf("empty file name")
    }
    fullpath, err = filepath.Abs(fullpath)
    if err != nil {
        return fullpath, err
    }
    _, err = os.Stat(fullpath);
    return fullpath, err
}

// InitConfig initializes configuration from a file.
func InitConfig(logger *LogChecker, name string) error {
    if logger.Works() {
        return fmt.Errorf("logchecker is already running")
    }
    path, err := FilePath(name)
    if err != nil {
        LoggerError.Println("Can't check config file")
        return err
    }
    logger.Cfg.Path = path
    jsondata, err := ioutil.ReadFile(path)
    if err != nil {
        LoggerError.Println("Can't read config file")
        return err
    }
    err = json.Unmarshal(jsondata, &logger.Cfg)
    if err != nil {
        LoggerError.Println("Can't parse config file")
        return err
    }
    return logger.Validate()
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

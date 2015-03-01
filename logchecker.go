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
    "github.com/z0rr0/taskqueue"
)

const (
    // MaxPollers is maximum number of task handlers.
    MaxPollers int = 5
)

var (
    // LoggerError implements error logger.
    LoggerError = log.New(os.Stderr, "ERROR [logchecker]: ", log.Ldate|log.Ltime|log.Lshortfile)
    // LoggerDebug implements debug logger, it's disabled by default.
    LoggerDebug = log.New(ioutil.Discard, "DEBUG [logchecker]: ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
)

// Backender is an interface to handle data storage operations.
type Backender interface {
    String() string
}

// File is a type of settings for a watched file.
type File struct {
    Log string            `json:"file"`
    Delay uint            `json:"delay"`
    Pattern string        `json:"pattern"`
    Boundary uint         `json:"boundary"`
    Increase bool         `json:"increase"`
    Emails []string       `json:"emails"`
    Limits [3]uint64      `json:"limits"`
    States [3]uint64      // counter of sent emails
    Counters [3]uint64    // cases counter for every periond
    RealLimits [3]uint64  // real conter after possible increasing
    Hours uint64          // hours after start
    Pos uint64            // file posision after last check
    ModTime time.Time     // file modify date during last check
    LogStart time.Time    // time of logger start
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

// LogChecker is a main object for logging.
type LogChecker struct {
    Name string
    Cfg Config
    Backend Backender
    Running time.Time
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

// String of MemoryBackend returns a name of the logger back-end.
func (bk *MemoryBackend) String() string {
    return fmt.Sprintf("Backend: %v", bk.Name)
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
    return res
}

// String returns main info about LogChecker.
func (logger *LogChecker) String() string {
    data := fmt.Sprintf("%v [%v]", logger.Name, logger.Backend)
    if (logger.Running != time.Time{}) {
        data += fmt.Sprintf(", starting from %v [%v]", logger.Running, time.Since(logger.Running))
    }
    return data
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
    if (logger.Running != time.Time{}) {
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

// Start runs LogChecker processes.
func (logger *LogChecker) Start() (chan bool, *sync.WaitGroup, chan taskqueue.Tasker) {
    logger.Running = time.Now()
    var group sync.WaitGroup
    finish := make(chan bool)

    tasks := make([]taskqueue.Tasker, 0)
    for i, serv := range logger.Cfg.Observed {
        for j, f := range serv.Files {
            if err := f.Validate(); err != nil {
                LoggerError.Printf("incorrect file was skipped [%v / %v]\n", serv.Name, f.Base())
            } else {
                serv.Files[j].RealLimits = serv.Files[j].Limits
                serv.Files[j].LogStart = time.Now()
                tasks = append(tasks, &Task{logger, &logger.Cfg.Observed[i], &serv.Files[j]})
           }
       }
    }
    complete := taskqueue.Start(tasks, &group, finish)
    return finish, &group, complete
}

func (logger *LogChecker) Stop(finish chan bool, group *sync.WaitGroup, complete chan taskqueue.Tasker) {
    defer func() {
        logger.Running = time.Time{}
    }()
    taskqueue.Stop(finish, group, complete)
}

// String returns main text info about the task.
func (task *Task) String() string {
    return fmt.Sprintf("%v-%v-%v", task.QLogChecker.Name, task.QService.Name, task.QFile.Base())
}

// Run starts a process to check a task.
func (task *Task) Run() {
    if count, err := task.Poll(); err != nil {
        LoggerError.Printf("poll is incorrect [%v]", task)
    } else {
        if err := task.Check(count); err != nil {
            LoggerError.Printf("task is not checked [%v]: %v", task, err)
        }
    }
}

// Sleep is a delay between runs of a task.
func (task *Task) Sleep() {
    time.Sleep(time.Duration(task.QFile.Delay) * time.Second)
}

// Poll reads file lines and counts needed from them.
// It skips "pos" lines.
func (task *Task) Poll() (uint64, error) {
    var counter, clines uint64
    info, err := os.Stat(task.QFile.Log)
    if err != nil {
        return counter, err
    }
    if task.QFile.ModTime.Equal(info.ModTime()) {
        // file is not chnaged
        return counter, nil
    }
    file, err := os.Open(task.QFile.Log)
    if err != nil {
        LoggerError.Printf("can't open file: %v\n", task.QFile.Log)
        return counter, err
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
    task.QFile.Pos = clines
    task.QFile.ModTime = info.ModTime()
    return counter, nil
}

// Check calculates currnet found abnormal records for time periods
func (task *Task) Check(count uint64) error {
    // var needsend bool
    // for i := range task.QFile.Counters {
    //     task.QFile.Counters[i] += uint64(count)
    // }
    // hours := uint64(time.Since(task.QFile.LogStart).Hours())
    // if task.QFile.Hours != hours {
    //     days := hours % 24
    //     weeks := days % 7
    //     switch {
    //         case (task.QFile.Hours % 168) != weeks:
    //              task.QFile.Counters = [3]uint64{0,0,0}
    //         case (task.QFile.Hours % 24) != days:
    //             task.QFile.Counters[0:1] = [2]uint64{0, 0}
    //         default:
    //             task.QFile.Counters[0] = 0
    //     }
    // }
    // for i := range task.QFile.Periods {
    //     if (task.QFile.Counters[i] >= task.QFile.Boundary) && (task.QFile.States[i] <= task.QFile.RealLimits[i]) {
    //         needsend = true
    //     }
    // }

    // task.QFile.RealLimits

    return nil
}

// DebugMode is a initialization of Logger handlers.
func DebugMode(debugmode bool) {
    debugHandle := ioutil.Discard
    if debugmode {
        debugHandle = os.Stdout
    }
    LoggerDebug = log.New(debugHandle, "DEBUG [logchecker]: ",
        log.Ldate|log.Lmicroseconds|log.Lshortfile)
    taskqueue.Debug(debugmode)
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
    if (logger.Running != time.Time{}) {
        return fmt.Errorf("logchecker is already running")
    }
    path, err := FilePath(name)
    if err != nil {
        LoggerError.Printf("Can't check config file [%v]", name)
        return err
    }
    logger.Cfg.Path = path
    jsondata, err := ioutil.ReadFile(path)
    if err != nil {
        LoggerError.Printf("Can't read config file [%v]", name)
        return err
    }
    err = json.Unmarshal(jsondata, &logger.Cfg)
    if err != nil {
        LoggerError.Printf("Can't parse config file [%v]", name)
        return err
    }
    return logger.Validate()
}

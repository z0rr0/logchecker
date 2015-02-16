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
    "strings"
    "io/ioutil"
    "encoding/json"
    "path/filepath"
)

// const (

// )

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
    Limits []uint   `json:"limits"`
    Counter [3]uint
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

// LogChecker is a main object for logging.
type LogChecker struct {
    Name string
    Cfg Config
    Backend Backender
    mutex sync.RWMutex
}

// MemoryBackend is a type for the implementation of memory storage methods.
type MemoryBackend struct {
    Name string
}

// GetName of MemoryBackend returns a name of the logger back-end.
func (bk *MemoryBackend) GetName() string {
    return bk.Name
}

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

// Watch starts a logger observation.
func (logger *LogChecker) Watch() {
    pending, complete := make(chan *File), make(chan *File)

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
            backend = &MemoryBackend{"Memory"}
    }
    if backend == nil {
        return fmt.Errorf("unknown backend")
    }
    logger.Backend = backend
    return nil
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

// New created new LogChecker object and returns its reference.
func New() *LogChecker {
    return &LogChecker{}
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

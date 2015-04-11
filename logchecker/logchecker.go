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
package logchecker

import (
    "bufio"
    "encoding/json"
    "fmt"
    "golang.org/x/exp/inotify"
    "io/ioutil"
    "log"
    "net/smtp"
    "os"
    "path/filepath"
    "regexp"
    "runtime"
    "strings"
    "sync"
    "time"
)

const (
    watcherMask uint32 = inotify.IN_MODIFY | inotify.IN_ATTRIB
    maxMsgLines uint64 = 10
    emailMsg string = "LogChecker notification.\n"
)

var (
    // LoggerError implements error logger.
    LoggerError = log.New(os.Stderr, "ERROR [logchecker]: ", log.Ldate|log.Ltime|log.Lshortfile)
    // LoggerInfo implements info logger.
    LoggerInfo = log.New(os.Stderr, "INFO [logchecker]: ", log.Ldate|log.Ltime|log.Lshortfile)
    // LoggerDebug implements debug logger, it's disabled by default.
    LoggerDebug = log.New(ioutil.Discard, "DEBUG [logchecker]: ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
    // MoveWait is waiting period before a check that a file was again created.
    MoveWait = 2 * time.Second
    // EmailSimulator is a file path to verify sent emails during debug mode.
    EmailSimulator string

    debug = false
    initTime = time.Time{}
)

// Backender is an interface to handle data storage operations.
type Backender interface {
    String() string
}

// Notifier is an interface to notify users about file changes.
type Notifier interface {
    String() string
    Notify(string, []string)
}

type debugSender struct {
    Name string
}
func (ds *debugSender) String() string {
    return ds.Name
}
func (ds *debugSender) Notify(msg string, to []string) {
    LoggerDebug.Printf("call EmailSimulator (%v)", EmailSimulator)
    writeLine := fmt.Sprintf("%v: get message (%v symbols) for [%v]\n", time.Now(), len(msg), strings.Join(to, ", "))
    if len(EmailSimulator) == 0 {
        LoggerDebug.Println("call Notify simulator with empty file path")
        LoggerDebug.Printf(writeLine)
    } else {
        if !filepath.IsAbs(EmailSimulator) {
            LoggerError.Println("path should be absolute")
            return
        }
        _, err := os.Stat(EmailSimulator);
        if err != nil {
            LoggerError.Printf("unknown file: %v", err)
            return
        }
        file, err := os.OpenFile(EmailSimulator, os.O_APPEND|os.O_WRONLY, 0660)
        if err != nil {
            LoggerError.Println(err)
            return
        }
        defer file.Close()
        writer := bufio.NewWriter(file)
        _, err = writer.WriteString(writeLine)
        if err != nil {
            LoggerError.Println(err)
            return
        }
        writer.Flush()
    }
}

// File is a type of settings for a watched file.
type File struct {
    Log string                `json:"file"`
    Pattern string            `json:"pattern"`
    Boundary uint64           `json:"boundary"`
    Increase bool             `json:"increase"`
    Emails []string           `json:"emails"`
    Limit uint64              `json:"limit"`
    Period uint64             `json:"period"`
    RgPattern *regexp.Regexp  // regexp expression from the pattern
    Pos uint64                // file posision after last check
    LogStart time.Time        // time of logger start
    Granularity uint64        // number of a period after last check
    Found uint64              // found lines by the Pattern
    Counter uint64            // cases counter for time period
    ExtBoundary uint64        // extended boundary value if Increase is set
    service *Service          // backward reference to service name
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

// String service name.
func (s *Service) String() string {
    return s.Name
}

// Base returns the last element of log file path.
func (f *File) Base() string {
    return filepath.Base(f.Log)
}

// String returns absolute file's path.
func (f *File) String() string {
    return f.Log
}

// Validate checks that File is correct: has absolute path and exists.
func (f *File) Validate() error {
    var err error
    if !filepath.IsAbs(f.Log) {
        return fmt.Errorf("path should be absolute")
    }
    _, err = os.Stat(f.Log);
    if err != nil {
        return err
    }
    if len(f.Pattern) == 0 {
        return fmt.Errorf("pattern should not be empty")
    }
    f.RgPattern, err = regexp.Compile(f.Pattern)
    if err != nil {
        return err
    }
    return nil
}

// Watch implements a file watcher.
func (f *File) Watch(group *sync.WaitGroup, finish chan bool, logger *LogChecker) {
    watcher, err := inotify.NewWatcher()
    if err != nil {
        LoggerError.Printf("can't create new watcher: %v - %v\n", f.Base(), err)
        return
    }
    if err = watcher.AddWatch(f.Log, watcherMask); err != nil {
        LoggerError.Printf("can't add new watcher: %v - %v\n", f.Base(), err)
        return
    }
    for {
        select {
            case <-finish:
                return
            case event := <-watcher.Event:
                if (event.Mask & inotify.IN_ATTRIB) != 0 {
                    LoggerInfo.Printf("file was deleted or moved[%v]: %v\n", event, f.Base())
                    watcher, err = IsMoved(f.Log, watcher)
                    if err != nil {
                        LoggerError.Printf("re-creation watcher error: %v\n", err)
                        return
                    }
                    f.Pos = 0
                }
                if err := f.Check(group, logger); err != nil {
                    LoggerError.Printf("[%v]: %v", f.String(), err)
                }
            case err := <-watcher.Error:
                LoggerError.Printf("file watcher error: %v\n", err)
                return
        }
    }
}

// Duration identifies user's time period after watcher start.
func (f *File) Duration() uint64 {
    return uint64(time.Since(f.LogStart).Seconds()) / f.Period
}

// Check validates conditions before sending email notifications.
func (f *File) Check(group *sync.WaitGroup, logger *LogChecker) error {
    var (
        counter, clines uint64
        msgLines []string
        notifier Notifier
    )
    group.Add(1)
    LoggerDebug.Printf("check: %v\n", f.Base())
    defer func() {
        LoggerDebug.Printf("check done: %v\n", f.Base())
        group.Done()
    }()

    file, err := os.Open(f.Log)
    if err != nil {
        return err
    }
    defer file.Close()

    // read the file line by line
    scanner := bufio.NewScanner(file)
    counter = 0
    for scanner.Scan() {
        clines++
        if f.Pos < clines {
            if line := scanner.Text(); len(line) > 0 {
                if f.RgPattern.MatchString(line) {
                    switch {
                        case counter < (maxMsgLines + 1):
                            msgLines = append(msgLines, fmt.Sprintf("%v: %v", clines, line))
                        case counter == (maxMsgLines + 1):
                            msgLines = append(msgLines, "...")
                    }
                    counter++
                }
            }
        }
    }
    err = scanner.Err()
    if err != nil {
        return err
    }
    curPeriod, sent := f.Duration(), false
    if curPeriod != f.Granularity {
        f.Granularity = curPeriod
        f.Found = 0
        f.Counter = 0
        LoggerDebug.Printf("period was reset [%v]: %v", f.Base(), f.Granularity)
    }
    f.Pos = clines
    f.Found += counter

    if (f.Found >= f.ExtBoundary) && (f.Counter <= f.Limit) {
        if f.Increase {
            f.ExtBoundary = f.ExtBoundary * 2
        }
        if debug {
            notifier = &debugSender{"debugSender"}
        } else {
            notifier = logger
        }
        message := fmt.Sprintf("%v\n\nReport for \"%v\" service (%v new items): %v\n%v\n\n--\nBR, LogChecker", emailMsg, f.service, f.Found, f.Log, strings.Join(msgLines, "\n"))
        go notifier.Notify(message, f.Emails)
        f.Counter++
        sent = true
    } else {
        f.ExtBoundary = f.Boundary
    }
    LoggerDebug.Printf("check [%v], sent=%v, found=%v, boundary=%v, counter=%v, limit=%v", f.Base(), sent, f.Found, f.ExtBoundary, f.Counter, f.Limit)
    return nil
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
            files[j] = file.Base()
        }
        services[i] = fmt.Sprintf("%v: %v", service.Name, strings.Join(files, ", "))
    }
    return fmt.Sprintf("Config [%v]: %v\n\t%v\n", cfg.Path, cfg.Storage, strings.Join(services, "\n\t"))
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
    if logger.IsWorking() {
        data += fmt.Sprintf(" (%v [%v])", logger.Running, time.Since(logger.Running))
    }
    return data
}

// HasService checks that the Service is included to the LogChecker.
// It can use locked mode to guarantee that service array will be
// immutable during reading.
func (logger *LogChecker) HasService(serv *Service, lock bool) int {
    if lock {
        logger.mutex.RLock()
        defer func() {
            logger.mutex.RUnlock()
        }()
    }
    for i, s := range logger.Cfg.Observed {
        if s.Name == serv.Name {
            return i
        }
    }
    return -1
}

// AddService includes a new Service to the LogChecker.
func (logger *LogChecker) AddService(serv *Service) error {
    if logger.IsWorking() {
        return fmt.Errorf("logchecker is already running")
    }
    logger.mutex.Lock()
    defer func() {
        logger.mutex.Unlock()
    }()
    if len(serv.Name) == 0 {
        return fmt.Errorf("service name should not be empty")
    }
    if logger.HasService(serv, false) > -1 {
        return fmt.Errorf("service [%v] is already used", serv.Name)
    }
    logger.Cfg.Observed = append(logger.Cfg.Observed, *serv)
    LoggerDebug.Printf("new service is added: %v\n", serv.Name)
    return nil
}

// RemoveService includes a new Service to the LogChecker.
func (logger *LogChecker) RemoveService(serv *Service) error {
    if logger.IsWorking() {
        return fmt.Errorf("logchecker is already running")
    }
    logger.mutex.Lock()
    defer func() {
        logger.mutex.Unlock()
    }()
    index := logger.HasService(serv, false)
    if index == -1 {
        return fmt.Errorf("service not found: %v", serv.Name)
    }
    logger.Cfg.Observed = append(logger.Cfg.Observed[0:index], logger.Cfg.Observed[index+1:]...)
    LoggerDebug.Printf("service is removed: %v\n", serv.Name)
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
                return fmt.Errorf("file error [%v] %v", f.Log, err)
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
func (logger *LogChecker) Notify(msg string, to []string) {
    const mime string = "MIME-version: 1.0;\nContent-Type: text/plain; charset=\"UTF-8\";\n\n";
    content := []byte("From: LogChecker\nSubject: LogChecker notification\n" + mime + msg)
    auth := smtp.PlainAuth(
        "",
        logger.Cfg.Sender["user"],
        logger.Cfg.Sender["password"],
        logger.Cfg.Sender["host"],
    )
    LoggerDebug.Println("send email")
    err := smtp.SendMail(logger.Cfg.Sender["addr"], auth, logger.Cfg.Sender["user"], to, content)
    if err != nil {
        LoggerError.Printf("send email error: %v", err)
    }
}

// IsWorking return "true" if LogChecker process is already running.
func (logger *LogChecker) IsWorking() bool {
    return logger.Running != initTime
}

// Start runs LogChecker processes.
func (logger *LogChecker) Start(group *sync.WaitGroup) (chan bool, error) {
    var watched int
    finish := make(chan bool)
    if logger.IsWorking() {
        return finish, fmt.Errorf("process is already running")
    }
    logger.Running = time.Now()
    defer LoggerInfo.Printf("%v is started.\n", logger)

    for i, serv := range logger.Cfg.Observed {
        info := make([]string, len(serv.Files))
        for j := range serv.Files {
            if err := serv.Files[j].Validate(); err != nil {
                LoggerError.Printf("incorrect file was skipped [%v / %v]\n", serv.Name, serv.Files[j].Base())
                info[j] = fmt.Sprintf("FAILED: %s", serv.Files[j].String())
            } else {
                serv.Files[j].service = &logger.Cfg.Observed[i]
                serv.Files[j].LogStart = time.Now()
                serv.Files[j].ExtBoundary = serv.Files[j].Boundary
                go serv.Files[j].Watch(group, finish, logger)
                info[j] = fmt.Sprintf("OK: %s \"%s\"", serv.Files[j].String(), serv.Files[j].Pattern)
                watched++
           }
       }
       LoggerInfo.Printf("%v prepared\n\t%v\n", serv, strings.Join(info, "\n\t"))
    }
    if watched == 0 {
        return finish, fmt.Errorf("empty task queue")
    }
    return finish, nil
}

// Stop terminated running process.
func (logger *LogChecker) Stop(finish chan bool, group *sync.WaitGroup) error {
    if !logger.IsWorking() {
        return fmt.Errorf("process is already stopped")
    }
    close(finish)
    group.Wait()
    logger.Running = initTime
    LoggerInfo.Printf("%v is stopped\n", logger)
    return nil
}

// DebugMode is a initialization of Logger handlers.
func DebugMode(debugmode bool) {
    debug = debugmode
    debugHandle := ioutil.Discard
    if debugmode {
        debugHandle = os.Stdout
    }
    LoggerDebug = log.New(debugHandle, "DEBUG [logchecker]: ",
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
    if runtime.GOOS != "linux" {
        LoggerError.Printf("unsupported platform: %v\n", runtime.GOOS)
        return fmt.Errorf("only Linux is now supported")
    }
    if logger.IsWorking() {
        return fmt.Errorf("logchecker is already running")
    }
    path, err := FilePath(name)
    if err != nil {
        LoggerError.Printf("can't check config file [%v]", name)
        return err
    }
    logger.Cfg.Path = path
    jsondata, err := ioutil.ReadFile(path)
    if err != nil {
        LoggerError.Printf("can't read config file [%v]", name)
        return err
    }
    err = json.Unmarshal(jsondata, &logger.Cfg)
    if err != nil {
        LoggerError.Printf("can't parse config file [%v]", name)
        return err
    }
    return logger.Validate()
}

// IsMoved creates new inotify watcher if a file was moved, instead returns an error.
func IsMoved(filename string, oldw *inotify.Watcher) (*inotify.Watcher, error) {
    var neww *inotify.Watcher
    time.Sleep(MoveWait)
    if _, err := os.Stat(filename); err != nil {
        oldw.RemoveWatch(filename)
        return neww, err
    }
    neww, err := inotify.NewWatcher()
    if err != nil {
        return neww, err
    }
    err = neww.AddWatch(filename, watcherMask)
    if err != nil {
        return neww, err
    }
    return neww, nil
}

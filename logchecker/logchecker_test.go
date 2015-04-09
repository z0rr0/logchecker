// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// LogChecker testing methods
//
package logchecker

import (
    "bufio"
    "golang.org/x/exp/inotify"
    "io/ioutil"
    "os"
    "os/signal"
    "path/filepath"
    "strings"
    "sync"
    "syscall"
    "testing"
    "time"
)

func buildDir() string {
    path := os.Getenv("TRAVIS_BUILD_DIR")
    if len(path) > 0 {
        return path
    }
    return filepath.Join(os.Getenv("GOPATH"), "src/github.com/z0rr0/logchecker")
}

func createFile(name string, mode int) error {
    file, err := os.Create(name)
    if err != nil {
        return err
    }
    file.Close()
    return os.Chmod(name, os.FileMode(mode))
}

func updateFile(name string, lines ...string) error {
    file, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil {
        return err
    }
    defer file.Close()
    writer := bufio.NewWriter(file)
    for _, v := range lines {
        _, err := writer.WriteString(v + "\n")
        if err != nil {
            return err
        }
    }
    return writer.Flush()
}

func moveFile(name, first string) error {
    tmpfile := filepath.Join(buildDir(), "test_tmp")
    err := createFile(tmpfile, 0666)
    if err != nil {
        return err
    }
    // defer os.Remove(tmpfile)
    return os.Rename(tmpfile, name)
}

func prepareConfig(from, to string, replace map[string]string) error {
    data, err := ioutil.ReadFile(from)
    if err != nil {
        return err
    }
    strinfo := string(data)
    for k, v := range replace {
        strinfo = strings.Replace(strinfo, k, v, 1)
    }
    return ioutil.WriteFile(to, []byte(strinfo), os.FileMode(0666))
}

// Tests

func TestDebugMode(t *testing.T) {
    if (LoggerError == nil) || (LoggerDebug == nil) {
        t.Errorf("incorrect references")
    }
    DebugMode(false)
    if (LoggerError.Prefix() != "ERROR [logchecker]: ") || (LoggerDebug.Prefix() != "DEBUG [logchecker]: ") {
        t.Errorf("incorrect loggers settings")
    }
    DebugMode(true)
    if (LoggerError.Flags() != 19) || (LoggerDebug.Flags() != 21) {
        t.Errorf("incorrect loggers settings")
    }
}

func TestFilePath(t *testing.T) {
    if _, err := FilePath("invalid_name"); err == nil {
        t.Errorf("incorrect response")
    }
    if _, err := FilePath(""); err == nil {
        t.Errorf("incorrect response")
    }
    realfile := filepath.Join(buildDir(), "config.example.json")
    if path, err := FilePath(realfile); err != nil {
        t.Errorf("incorrect response, the file should exist")
    } else {
        if path != realfile {
            t.Errorf("ivalid paths")
        }
    }
}

func TestInitConfig(t *testing.T) {
    rm := func(name string) {
        if err := os.Remove(name); err != nil {
            t.Errorf("can't remove file [%v]: %v", name, err)
        }
    }
    testdir := buildDir()
    newvalues := map[string]string{
        "/var/log/nginx/error.log": filepath.Join(testdir, "test_error.log"),
        "/var/log/nginx/access.log": filepath.Join(testdir, "test_access.log"),
        "/var/log/syslog": filepath.Join(testdir, "test_syslog"),
    }
    oldexample := filepath.Join(testdir, "config.example.json")
    example := filepath.Join(testdir, "config.new.json")
    if err := prepareConfig(oldexample, example, newvalues); err != nil {
        t.Errorf("can't prepare test config file [%v]", err)
    }
    defer rm(example)
    for _, v := range newvalues {
        if err := createFile(v, 0666); err != nil {
            t.Errorf("test file preparation error [%v]: %v", v, err)
        }
        defer rm(v)
    }
    logger := New()
    if err := InitConfig(logger, example); err != nil {
        t.Errorf("error during InitConfig [%v]: %v", example, err)
    }
    if l := len(logger.Cfg.String()); l == 0 {
        t.Errorf("config should be initiated [%v]", l)
    }

    // checks of incorrect configurations
    if len(logger.Cfg.Observed) > 1 {
        logger.Cfg.Observed[0].Name = logger.Cfg.Observed[1].Name
        if err := logger.Validate(); err == nil {
            t.Errorf("wrong validation [%v]", err)
        }
    }
    if err := InitConfig(logger, "invalid_name"); err == nil {
        t.Errorf("need error during InitConfig")
    }
    testfile := filepath.Join(testdir, "testfile.json")
    if err := createFile(testfile, 0200); err != nil {
        t.Errorf("%v", err)
    }
    defer rm(testfile)
    if err := InitConfig(logger, testfile); err == nil {
        t.Errorf("need permissions error during InitConfig")
    }
    os.Chmod(testfile, 0666)
    if err := InitConfig(logger, testfile); err == nil {
        t.Errorf("need json error during InitConfig")
    }
}

func TestNew(t *testing.T) {
    logger := New()
    if logger == nil {
        t.Errorf("incorrect reference")
    }
    serv := Service{}
    if err := logger.AddService(&serv); err == nil {
        t.Errorf("incorrect response for empty Service: %v\n", err)
    }
    if err := logger.RemoveService(&serv); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    serv.Name = "TestSrv"
    if serv.String() != "TestSrv" {
        t.Errorf("invalid service name")
    }
    if logger.HasService(&serv, true) > -1 {
        t.Errorf("incorrect response")
    }
    if err := logger.AddService(&serv); err != nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    if logger.HasService(&serv, true) == -1 {
        t.Errorf("incorrect response")
    }
    if err := logger.AddService(&serv); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    if err := logger.RemoveService(&serv); err != nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    //
    if err := logger.AddService(&serv); err != nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    logger.Cfg.Sender = map[string]string{
        "user": "user@host.com",
        "password": "password",
        "host": "smtp.host.com",
        // "addr": "smtp.host.com:25",
    }
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    logger.Cfg.Sender = map[string]string{
        "user": "user@host.com",
        "password": "password",
        "host": "smtp.host.com",
        "addr": "",
    }
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    logger.Cfg.Sender = map[string]string{
        "user": "user@host.com",
        "password": "password",
        "host": "smtp.host.com",
        "addr": "smtp.host.com:25",
    }
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    fileExample := File{}
    logger.Cfg.Observed[0].Files = append(logger.Cfg.Observed[0].Files, fileExample)
    // unknown backend
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    logger.Cfg.Storage = "memory"
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    logger.Cfg.Observed[0].Files[0].Log = "/tmp/wrong"
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }

    rm := func(name string) {
        if err := os.Remove(name); err != nil {
            t.Errorf("can't remove file [%v]: %v", name, err)
        }
    }
    testdir := buildDir()
    filename := filepath.Join(testdir, "test_config.log")
    if err := createFile(filename, 0666); err != nil {
        t.Errorf("test file preparation error [%v]", err)
    }
    defer rm(filename)

    logger.Cfg.Observed[0].Files[0].Log = filename
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    logger.Cfg.Observed[0].Files[0].Pattern = "**"
    if err := logger.Validate(); err == nil {
        t.Errorf("incorrect response: %v\n", err)
    }
    if len(logger.String()) == 0 {
        t.Errorf("invalid logger")
    }
}

func TestIsMoved(t *testing.T) {
    MoveWait = 1 * time.Second
    // rm := func(name string) {
    //     if err := os.Remove(name); err != nil {
    //         t.Errorf("can't remove file [%v]: %v", name, err)
    //     }
    // }
    testfile := filepath.Join(buildDir(), "test_error.log")
    err := createFile(testfile, 0666)
    if err != nil {
        t.Errorf("test file preparation error [%v]: %v", testfile, err)
    }
    // defer rm(testfile)

    watcher, err := inotify.NewWatcher()
    if err != nil {
        t.Errorf("cant create inotify watcher")
    }
    if err = watcher.AddWatch(testfile, inotify.IN_CLOSE_WRITE | inotify.IN_ATTRIB); err != nil {
        t.Errorf("cant add inotify watcher")
    }

    go func() {
        time.Sleep(100 * time.Millisecond)
        if err := updateFile(testfile, "new line"); err != nil {
            t.Errorf("cant update file %v", err)
        }
        time.Sleep(100 * time.Millisecond)
        if err := moveFile(testfile, "init line"); err != nil {
            t.Errorf("cant move file %v", err)
        }
        time.Sleep(1100 * time.Millisecond)
        if err := os.Remove(testfile); err != nil {
            t.Errorf("cant remove file")
        }
    }()

    func() {
        for {
            select {
                case event := <-watcher.Event:
                    t.Log("file update detected", event.String())
                    if (event.Mask & inotify.IN_ATTRIB) != 0 {
                        watcher, err = IsMoved(testfile, watcher)
                        if err != nil {
                            t.Log("file was removed")
                            return
                        }
                    }
                case err := <-watcher.Error:
                    t.Errorf("watcher error: %v", err)
                    return
            }
        }
    }()
}

func TestStart(t *testing.T) {
    var (
        group sync.WaitGroup
        Period time.Duration = 30 * time.Second
    )
    rm := func(name string) {
        if err := os.Remove(name); err != nil {
            t.Errorf("can't remove file [%v]: %v", name, err)
        }
    }
    DebugMode(true)
    testdir := buildDir()
    newvalues := map[string]string{
        "/var/log/nginx/error.log": filepath.Join(testdir, "test_error.log"),
        "/var/log/nginx/access.log": filepath.Join(testdir, "test_access.log"),
        "/var/log/syslog": filepath.Join(testdir, "test_syslog"),
    }
    oldexample := filepath.Join(testdir, "config.example.json")
    example := filepath.Join(testdir, "config.new.json")
    if err := prepareConfig(oldexample, example, newvalues); err != nil {
        t.Errorf("can't prepare test config file [%v]", err)
    }
    defer rm(example)
    for _, v := range newvalues {
        if err := createFile(v, 0666); err != nil {
            t.Errorf("test file preparation error [%v]: %v", v, err)
        }
        defer rm(v)
    }

    logger := New()
    if err := InitConfig(logger, example); err != nil {
        t.Error(err)
    }
    logger.Name = "Test-LogChecker"
    // process start
    finish, err := logger.Start(&group)
    if err != nil {
        t.Error(err)
    }
     // config monitoring
    watcher, err := inotify.NewWatcher()
    if err != nil {
        t.Error(err)
    }
    if err = watcher.AddWatch(logger.Cfg.Path, watcherMask); err != nil {
        close(finish)
        t.Errorf("can't activate config watcher: %v\n", err)
    }
    timestat := time.Tick(Period)
    sigchan := make(chan os.Signal, 2)
    signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)

    // notificationCounter := 2

    for {
        select {
            case <-sigchan:
                t.Log("process will be stopped")
                close(finish)
                group.Wait()
                return
            case event := <-watcher.Event:
                t.Log("process will be resarted due to reconfiguration")
                if (event.Mask & inotify.IN_DELETE_SELF) != 0 {
                    watcher, err = IsMoved(logger.Cfg.Path, watcher)
                    if err != nil {
                        t.Errorf("re-creation watcher error: %v\n", err)
                    }
                }
                if err = logger.Stop(finish, &group); err != nil {
                    t.Error(err)
                }
                err = InitConfig(logger, logger.Cfg.Path)
                if err != nil {
                    t.Error(err)
                }
                finish, err = logger.Start(&group)
                if err != nil {
                    t.Errorf("can't start the process: %v\n", err)
                    t.Error(err)
                }
            case werr := <-watcher.Error:
                t.Errorf("config watcher error: %v\n", werr)
                if err = logger.Stop(finish, &group); err != nil {
                    t.Error(err)
                }
                t.Error(werr)
            case <- timestat:
                t.Log("statictics: %v", logger)
        }
    }
}

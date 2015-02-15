// Copyright (c) 2015, Alexander Zaytsev. All rights reserved.
// Use of this source code is governed by a LGPL-style
// license that can be found in the LICENSE file.

// LogChecker testing methods
//
package logchecker

import (
	"testing"
)

func TestDebugMode(t *testing.T) {
    if (LoggerError == nil) || (LoggerDebug == nil) {
        t.Errorf("Incorrect references")
    }
    DebugMode(false)
    if (LoggerError.Prefix() != "ERROR: ") || (LoggerDebug.Prefix() != "DEBUG: ") {
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

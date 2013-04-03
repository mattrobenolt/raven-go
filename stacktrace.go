// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Some code from the runtime/debug package of the Go standard library.

package raven

import (
	"bytes"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"sync"
)

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Stacktrace
type Stacktrace struct {
	// Required
	Frames []StacktraceFrame `json:"frames"`
}

func (s *Stacktrace) Class() string { return "sentry.interfaces.Stacktrace" }

type StacktraceFrame struct {
	// At least one required
	Filename string `json:"filename,omitempty"`
	Function string `json:"function,omitempty"`
	Module   string `json:"module,omitempty"`

	// Optional
	Lineno       int      `json:"lineno,omitempty"`
	Colno        int      `json:"colno,omitempty"`
	AbsolutePath string   `json:"abs_path,omitempty"`
	ContextLine  string   `json:"context_line,omitempty"`
	PreContext   []string `json:"pre_context,omitempty"`
	PostContext  []string `json:"post_context,omitempty"`
	InApp        *bool    `json:"in_app,omitempty"`
}

// Intialize and populate a new stacktrace, skipping skip frames.
//
// context is the number of surrounding lines that should be included for context.
// Setting context to 3 would try to get seven lines. Setting context to -1 returns
// one line with no surrounding context, and 0 returns no context.
//
// appPackagePrefixes is a list of prefixes used to check whether a package should
// be considered "in app".
func NewStacktrace(skip int, context int, appPackagePrefixes []string) *Stacktrace {
	var frames []StacktraceFrame
	for i := 1 + skip; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		frame := StacktraceFrame{AbsolutePath: file, Filename: trimPath(file), Lineno: line, InApp: new(bool)}
		frame.Module, frame.Function = functionName(pc)
		if frame.Module == "main" {
			*frame.InApp = true
		} else {
			for _, prefix := range appPackagePrefixes {
				if strings.HasPrefix(frame.Module, prefix) {
					*frame.InApp = true
				}
			}
			if frame.InApp == nil {
				*frame.InApp = false
			}
		}

		if context > 0 {
			contextLines := fileContext(file, line-context, (context*2)+1)
			if len(contextLines) > 0 {
				for i, line := range contextLines {
					switch {
					case i < context:
						frame.PreContext = append(frame.PreContext, string(line))
					case i == context:
						frame.ContextLine = string(line)
					default:
						frame.PostContext = append(frame.PostContext, string(line))
					}
				}
			}
		} else if context == -1 {
			contextLine := fileContext(file, line, 1)
			if len(contextLine) > 0 {
				frame.ContextLine = string(contextLine[0])
			}
		}

		frames = append(frames, frame)
	}
	return &Stacktrace{frames}
}

// Retrieve the name of the package and function containing the PC.
func functionName(pc uintptr) (pack string, name string) {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return
	}
	name = fn.Name()
	// We get this:
	//	runtime/debug.*T·ptrmethod
	// and want this:
	//  pack = runtime/debug
	//	name = *T.ptrmethod
	if idx := strings.LastIndex(name, "."); idx != -1 {
		pack = name[:idx]
		name = name[idx+1:]
	}
	name = strings.Replace(name, "·", ".", -1)
	return
}

var fileCacheLock sync.Mutex
var fileCache = make(map[string][][]byte)

func fileContext(filename string, line int, count int) [][]byte {
	fileCacheLock.Lock()
	defer fileCacheLock.Unlock()
	lines, ok := fileCache[filename]
	if !ok {
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil
		}
		lines = bytes.Split(data, []byte{'\n'})
		fileCache[filename] = lines
	}
	line-- // stack trace lines are 1-indexed
	end := line + count
	if line >= len(lines) {
		return nil
	}
	if end > len(lines) {
		end = len(lines)
	}
	return lines[line:end]
}

var trimPaths []string

// Try to trim the GOROOT or GOPATH prefix off of a filename
func trimPath(filename string) string {
	for _, prefix := range trimPaths {
		if prefix[len(prefix)-1] != '/' {
			prefix += "/"
		}
		trimmed := filename
		// TODO: Use strings.TrimPrefix when Go 1.1 has been released
		if strings.HasPrefix(filename, prefix) {
			trimmed = filename[len(prefix):]
		}
		if len(trimmed) < len(filename) {
			return trimmed
		}
	}
	return filename
}

func init() {
	trimPaths = []string{runtime.GOROOT()}
	if path := os.Getenv("GOPATH"); path != "" {
		trimPaths = append(trimPaths, strings.Split(path, ":")...)
	}
}
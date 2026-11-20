/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logger

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"

	logrus "github.com/sirupsen/logrus"
)

var componentName string

const LoggerTimeStampFormat = "2006-01-02T15:04:05.999Z07:00"

type Fields map[string]interface{}

// ANSI color escape codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

type MeepFormatter struct {
	component string
}

func (f *MeepFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var levelColor string
	switch entry.Level {
	case logrus.InfoLevel:
		levelColor = ColorGreen
	case logrus.WarnLevel:
		levelColor = ColorYellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		levelColor = ColorBold + ColorRed
	case logrus.DebugLevel:
		levelColor = ColorCyan
	case logrus.TraceLevel:
		levelColor = ColorWhite
	default:
		levelColor = ColorReset
	}

	timestamp := entry.Time.Format(LoggerTimeStampFormat)
	levelStr := strings.ToUpper(entry.Level.String())

	var fieldsStr string
	if len(entry.Data) > 0 {
		fields := make([]string, 0, len(entry.Data))
		for k, v := range entry.Data {
			if k == "meep.component" || k == "meep.from" {
				continue
			}
			fields = append(fields, fmt.Sprintf("%s=%v", k, v))
		}
		if len(fields) > 0 {
			sort.Strings(fields)
			fieldsStr = fmt.Sprintf(" %s%s%s", ColorPurple, strings.Join(fields, " "), ColorReset)
		}
	}

	fromStr := ""
	if from, ok := entry.Data["meep.from"]; ok {
		fromStr = fmt.Sprintf(" (%s%s%s)", ColorBlue, from, ColorReset)
	}

	logLine := fmt.Sprintf("%s %s[%s]%s %s%s%s%s - %s%s\n",
		timestamp,
		ColorBold+ColorBlue, f.component, ColorReset,
		levelColor, levelStr, ColorReset,
		fromStr,
		entry.Message,
		fieldsStr,
	)

	return []byte(logLine), nil
}

func MeepTextLogInit(name string) {
	componentName = name
	if os.Getenv("MEEP_LOG_FORMAT") == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logrus.SetFormatter(&MeepFormatter{component: name})
	}
	logrus.SetLevel(logrus.DebugLevel)
}

func MeepJSONLogInit(name string) {
	componentName = name
	if os.Getenv("MEEP_LOG_FORMAT") == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logrus.SetFormatter(&MeepFormatter{component: name})
	}
	logrus.SetLevel(logrus.DebugLevel)
}

// getLogCaller
// setReportCaller cannot ignore caller levels, so cannot work in a wrapper. Fix not merged in github, so doing it manually
func getLogCaller() string {
	_, file, line, _ := runtime.Caller(2)

	location := fmt.Sprintf("%v:%v", path.Base(file), line)
	return location
}

func Info(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
	}).Info(args...)
}

func Debug(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
		"meep.from":      getLogCaller(),
	}).Debug(args...)
}

func Trace(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
		"meep.from":      getLogCaller(),
	}).Trace(args...)
}

func Warn(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
		"meep.from":      getLogCaller(),
	}).Warn(args...)
}

func Error(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
		"meep.from":      getLogCaller(),
	}).Error(args...)
}

func Panic(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
		"meep.from":      getLogCaller(),
	}).Panic(args...)
}

func Fatal(args ...interface{}) {
	logrus.WithFields(logrus.Fields{
		"meep.component": componentName,
		"meep.from":      getLogCaller(),
	}).Fatal(args...)
}

func WithFields(fields Fields) *logrus.Entry {
	return logrus.WithFields(logrus.Fields(fields))
}

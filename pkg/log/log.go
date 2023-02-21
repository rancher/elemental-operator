/*
Copyright © 2022 - 2023 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"flag"
	"strconv"

	"k8s.io/klog/v2"
)

const (
	DebugDepth = 7
)

func EnableDebugLogging() {
	var fs flag.FlagSet
	klog.InitFlags(&fs)
	err := fs.Set("v", strconv.Itoa(DebugDepth))
	if err != nil {
		klog.Warningf("Error enabling debug logging: %s", err.Error())
		return
	}
	klog.V(DebugDepth).Infoln("Debug logs enabled")
}

func Infof(format string, args ...interface{}) {
	klog.Infof(format, args)
}

func Info(args ...interface{}) {
	klog.Info(args)
}

func Debugf(format string, args ...interface{}) {
	klog.V(DebugDepth).Infof(format, args)
}

func Debug(args ...interface{}) {
	klog.V(DebugDepth).Info(args)
}

func Debugln(args ...interface{}) {
	klog.V(DebugDepth).Infoln(args)
}

func Warningf(format string, args ...interface{}) {
	klog.Warningf(format, args)
}

func Warning(args ...interface{}) {
	klog.Warning(args)
}

func Errorf(format string, args ...interface{}) {
	klog.Errorf(format, args)
}

func Error(args ...interface{}) {
	klog.Error(args)
}

func Fatalln(args ...interface{}) {
	klog.Fatalln(args)
}

func Fatalf(format string, args ...interface{}) {
	klog.Fatalf(format, args)
}

func Fatal(args ...interface{}) {
	klog.Fatal(args)
}

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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	var port, timeout int
	var dir string
	var srv http.Server

	flag.IntVar(&port, "p", 80, "listening port")
	flag.IntVar(&timeout, "t", 0, "timeout in seconds")
	flag.StringVar(&dir, "d", "", "serving dir"+" (default current dir)")

	flag.Parse()
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
	}

	fileinfo, err := os.Stat(dir)
	if err != nil {
		log.Fatal(err)
	}
	if !fileinfo.IsDir() {
		log.Fatalf("%s is not a directory", dir)
	}

	shutdownCompleted := make(chan struct{})
	if timeout > 0 {
		go func() {
			log.Printf("Deadline set to %d seconds", timeout)
			time.Sleep(time.Duration(timeout) * time.Second)
			log.Printf("Deadline reached, closing idle connections")
			// Shutdown makes ListenAndServe to return immediately, than waits for
			// active connections to complete before unblocking
			if err := srv.Shutdown(context.Background()); err != nil {
				log.Printf("Got unexpected error on shutdown: %s", err.Error())
			}
			close(shutdownCompleted)
		}()
	}

	log.Printf("Serving '%s' on port %d", dir, port)
	srv.Addr = fmt.Sprintf(":%d", port)
	srv.Handler = http.FileServer(http.Dir(dir))
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-shutdownCompleted
}

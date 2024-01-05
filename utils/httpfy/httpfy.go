/*
Copyright © 2022 - 2024 SUSE LLC

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
	"path"
	"time"
)

type singleFileHandler struct {
	name string
}

func (sf *singleFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// since the URL here will likely not specify a file name, pass the file name in the header
	w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", path.Base(sf.name)))
	http.ServeFile(w, r, sf.name)
}

func serveSingleFile(filePath string) http.Handler {
	return &singleFileHandler{filePath}
}

func selectHTTPHandler(srvPath string) (http.Handler, error) {
	srvPathInfo, err := os.Stat(srvPath)
	if err != nil {
		return nil, err
	}

	if srvPathInfo.IsDir() {
		return http.FileServer(http.Dir(srvPath)), nil
	}

	return serveSingleFile(srvPath), nil
}

func main() {
	var port, timeout int
	var path string
	var srv http.Server
	var err error

	flag.IntVar(&port, "p", 80, "listening port")
	flag.IntVar(&timeout, "t", 0, "timeout in seconds")
	flag.StringVar(&path, "d", "", "serving dir or file"+" (default current dir)")

	flag.Parse()
	if path == "" {
		path, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
	}

	srv.Handler, err = selectHTTPHandler(path)
	if err != nil {
		log.Fatal(err)
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

	log.Printf("Serving '%s' on port %d", path, port)
	srv.Addr = fmt.Sprintf(":%d", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-shutdownCompleted
}

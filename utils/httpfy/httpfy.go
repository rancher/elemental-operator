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
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	var port int
	var dir string

	flag.IntVar(&port, "p", 80, "listening port")
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

	log.Printf("Serving '%s' on port %d", dir, port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), http.FileServer(http.Dir(dir))))
}

/*
 * Copyright 2019 InfAI (CC SES)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"flag"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type VoidWriter struct{}

func (v VoidWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	time.Sleep(5 * time.Second) //wait for routing tables in cluster

	configLocation := flag.String("config", "config.json", "configuration file")
	flag.Parse()

	config, err := configuration.LoadConfig(*configLocation)
	if err != nil {
		log.Fatal("ERROR: unable to load config ", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	switch config.Log {
	case "void":
		log.SetOutput(VoidWriter{})
	case "":
		break
	case "stdout":
		log.SetOutput(os.Stdout)
	case "stderr":
		log.SetOutput(os.Stderr)
	default:
		f, err := os.OpenFile(config.Log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	err = lib.Start(ctx, config)
	if err != nil {
		log.Fatal(err)
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	sig := <-shutdown
	log.Println("received shutdown signal", sig)
	cancel()
	log.Println("shutdown in 1s")
	time.Sleep(1 * time.Second)
}

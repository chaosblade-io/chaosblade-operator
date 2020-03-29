/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
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

package main

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"os/exec"
	"time"

	chaosbladehook "github.com/chaosblade-io/chaosblade-operator/pkg/hookfs"
	"github.com/ethercflow/hookfs/hookfs"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/sirupsen/logrus"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var (
	addr       string
	pidFile    string
	original   string
	mountpoint string
)

func main() {
	flag.StringVar(&addr, "addr", ":65534", "The address to bind to")
	flag.StringVar(&original, "original", "", "ORIGINAL")
	flag.StringVar(&mountpoint, "mountpoint", "", "MOUNTPOINT")

	rand.Seed(time.Now().UnixNano())
	flag.Parse()
	logf.SetLogger(zap.Logger())
	stopCh := signals.SetupSignalHandler()
	//go startFuseServer(stopCh)
	chaosbladeHookServer := chaosbladehook.NewChaosbladeHookServer(addr)

	go chaosbladeHookServer.Start(stopCh)
	if err := startFuseServer(stopCh); err != nil {
		logrus.Fatal("start fuse server failed", err)
	}

}

func startFuseServer(stop <-chan struct{}) error {
	if !IsExist(original) {
		if err := os.MkdirAll(original, os.FileMode(755)); err != nil {
			return err
		}
	}
	if !IsExist(mountpoint) {
		if err := os.MkdirAll(mountpoint, os.FileMode(755)); err != nil {
			return err
		}
	}

	logrus.Info("Init hookfs")
	fs, err := hookfs.NewHookFs(original, mountpoint, &chaosbladehook.ChaosbladeHook{})
	if err != nil {
		return err
	}
	errCh := make(chan error)
	go func() {
		errCh <- fs.Serve()
	}()
	for {
		select {
		case <-stop:
			logrus.Infof("start unmount fuse volume %s", mountpoint)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "fusermount", "-zu", mountpoint)
			if err := cmd.Run(); err != nil {
				logrus.Errorf("failed to fusermount: %v", cmd)
			}
			return err
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

func IsExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

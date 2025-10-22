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
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/ethercflow/hookfs/hookfs"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	chaosbladehook "github.com/chaosblade-io/chaosblade-operator/pkg/hookfs"
)

var (
	address    string
	pidFile    string
	original   string
	mountpoint string
)

func main() {
	flag.StringVar(&address, "address", ":65534", "The address to bind")
	flag.StringVar(&original, "original", "", "Mapping of the original disk, not affected by the drill")
	flag.StringVar(&mountpoint, "mountpoint", "", "The disk of the drill. The affected directories are controlled by the path flag.")
	rand.Seed(time.Now().UnixNano())
	flag.Parse()

	logFields := logrus.WithFields(logrus.Fields{
		"address":    address,
		"original":   original,
		"mountpoint": mountpoint,
	})
	stopCtx := signals.SetupSignalHandler()
	chaosbladeHookServer := chaosbladehook.NewChaosbladeHookServer(address)
	logFields.Infoln("Start chaosblade hook server.")
	go chaosbladeHookServer.Start(stopCtx)

	logFields.Infoln("Start fuse server.")
	if err := startFuseServer(stopCtx); err != nil {
		logFields.WithError(err).Fatalln("Start fuse server failed")
	}
}

// startFuseServer starts hookfs server
func startFuseServer(stop context.Context) error {
	if !util.IsExist(original) {
		if err := os.MkdirAll(original, os.FileMode(755)); err != nil {
			return fmt.Errorf("create original directory error, %v", err)
		}
	}
	if !util.IsExist(mountpoint) {
		if err := os.MkdirAll(mountpoint, os.FileMode(755)); err != nil {
			return fmt.Errorf("create mountpoint directory error, %v", err)
		}
	}
	fs, err := hookfs.NewHookFs(original, mountpoint, &chaosbladehook.ChaosbladeHook{MountPoint: mountpoint})
	if err != nil {
		return fmt.Errorf("create hookfs error, %v", err)
	}
	errCh := make(chan error)
	go func() {
		errCh <- fs.Serve()
	}()
	for {
		select {
		case <-stop.Done():
			logFields := logrus.WithFields(logrus.Fields{
				"address":    address,
				"original":   original,
				"mountpoint": mountpoint,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "fusermount", "-zu", mountpoint)
			logFields.Infof("Start unmount fuse volume, cmd: %v", cmd)
			if err := cmd.Run(); err != nil {
				logFields.WithError(err).Errorln("Failed to execute fusermount")
			}
			return err
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

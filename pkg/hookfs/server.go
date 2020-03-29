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

package hookfs

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"regexp"
	"sync"
	"syscall"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

//go:generate protoc -I pb pb/injure.proto --go_out=plugins=grpc:pb

var (
	log              = ctrl.Log.WithName("fuse-chaosblade")
	injectFaultCache sync.Map
)

func init() {
	injectFaultCache = sync.Map{}
}

type InjectMessage struct {
	Methods []string `json:"methods"`
	Path    string   `json:"path"`
	Delay   uint32   `json:"delay"`
	Percent uint32   `json:"percent"`
	Random  bool     `json:"random"`
	Errno   uint32   `json:"errno"`
}

type ChaosbladeHookServer struct {
	addr string
}

func NewChaosbladeHookServer(addr string) *ChaosbladeHookServer {
	return &ChaosbladeHookServer{
		addr: addr,
	}
}

func (s *ChaosbladeHookServer) Start(stop <-chan struct{}) error {
	mux := http.NewServeMux()
	mux.HandleFunc(InjectPath, s.InjectHandler)
	mux.HandleFunc(RecoverPath, s.RecoverHandler)
	errCh := make(chan error)
	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}
	log.Info("start chaosblade server", "addr", s.addr)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	for {
		select {
		case <-stop:
			return server.Shutdown(context.Background())
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

func (s *ChaosbladeHookServer) InjectHandler(w http.ResponseWriter, r *http.Request) {
	var injectMsg InjectMessage
	if err := json.NewDecoder(r.Body).Decode(&injectMsg); err != nil {
		log.Error(err, "Cannot Decode Request Message")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	log.Info("Inject Fault", "inject message", injectMsg)
	for _, method := range injectMsg.Methods {
		injectFaultCache.Store(method, &injectMsg)
	}
}
func (s *ChaosbladeHookServer) RecoverHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("recover all fault")
	for _, method := range defaultHookPoints {
		injectFaultCache.Delete(method)
	}
}

func randomErrno() error {
	// from E2BIG to EXFULL, notice linux only
	return syscall.Errno(rand.Intn(0x36-0x7) + 0x7)
}

func probab(percentage uint32) bool {
	return rand.Intn(99) < int(percentage)
}

func doInjectFault(path, method string) error {
	log.Info("do Inject fault", "method", method, "path", path)
	val, ok := injectFaultCache.Load(method)
	if !ok {
		return nil
	}
	faultMsg := val.(*InjectMessage)
	log.Info("do Inject fault", "fault message", faultMsg)

	if faultMsg.Percent > 0 && !probab(faultMsg.Percent) {
		return nil
	}

	if len(faultMsg.Path) > 0 {
		re, err := regexp.Compile(faultMsg.Path)
		if err != nil {
			log.Error(err, "failed to parse path", "path: ", faultMsg.Path)
			return nil
		}
		if !re.MatchString(path) {
			return nil
		}
	}

	var err error = nil
	if faultMsg.Errno != 0 {
		err = syscall.Errno(faultMsg.Errno)
	} else if faultMsg.Random {
		err = randomErrno()
	}

	if faultMsg.Delay > 0 {
		time.Sleep(time.Duration(faultMsg.Delay) * time.Millisecond)
	}
	return err

}

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
	"fmt"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

var injectFaultCache sync.Map

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

func (s *ChaosbladeHookServer) Start(stop context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(InjectPath, s.InjectHandler)
	mux.HandleFunc(RecoverPath, s.RecoverHandler)
	errCh := make(chan error)
	server := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}
	go func() {
		errCh <- server.ListenAndServe()
	}()
	for {
		select {
		case <-stop.Done():
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
		logrus.WithError(err).Errorf("Cannot Decode Request Message, %+v", r)
		http.Error(w, "Cannot Decode Request Message", http.StatusBadRequest)
		return
	}
	logrus.WithField("injectMsg", injectMsg).Infoln("Inject Fault")
	for _, method := range injectMsg.Methods {
		injectFaultCache.Store(method, &injectMsg)
	}
	fmt.Fprintf(w, "success")
}

func (s *ChaosbladeHookServer) RecoverHandler(w http.ResponseWriter, r *http.Request) {
	logrus.Infoln("recover all fault")
	for _, method := range defaultHookPoints {
		injectFaultCache.Delete(method)
	}
	fmt.Fprintf(w, "success")
}

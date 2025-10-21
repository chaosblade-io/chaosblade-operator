/*
 * Copyright 2025 The ChaosBlade Authors
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
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
)

type ChaosBladeHookClient struct {
	client *http.Client
	addr   string
}

func NewChabladeHookClient(addr string) *ChaosBladeHookClient {
	return &ChaosBladeHookClient{
		addr: addr,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
				DisableKeepAlives: true,
			},
		},
	}
}

func (c *ChaosBladeHookClient) InjectFault(ctx context.Context, injectMsg *InjectMessage) error {
	url := "http://" + c.addr + InjectPath
	body, err := json.Marshal(injectMsg)
	if err != nil {
		return err
	}
	logrus.WithField("injectMsg", injectMsg).Infoln("Inject fault")
	result, err, code := util.PostCurl(url, body, "application/json")
	if err != nil {
		return err
	}
	logrus.WithField("injectMsg", injectMsg).Infof("Response is %s", result)
	if code != http.StatusOK {
		return errors.New(result)
	}
	return nil
}

func (c *ChaosBladeHookClient) Revoke(ctx context.Context) error {
	url := "http://" + c.addr + RecoverPath
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	result := string(bytes)
	logrus.Infof("Revoke fault, response is %s", result)
	if resp.StatusCode != http.StatusOK {
		return errors.New(result)
	}
	return nil
}

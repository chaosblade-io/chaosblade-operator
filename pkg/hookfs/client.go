package hookfs

import (
	"context"
	"encoding/json"
	"fmt"
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
		return fmt.Errorf(result)
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
		return fmt.Errorf(result)
	}
	return nil
}

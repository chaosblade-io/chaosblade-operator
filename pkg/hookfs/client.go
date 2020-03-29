package hookfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
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
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("")
	}
	return nil
}

func (c *ChaosBladeHookClient) Revocer(ctx context.Context) error {
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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("")
	}
	return nil
}

package main

import(
	"os"
	"fmt"
	"log"
	"time"
	"bytes"
	"syscall"
	"context"
	"net/http"
	"io/ioutil"
	"crypto/tls"
	"encoding/json"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
)
type RetryHttpClient struct {
	Client *retryablehttp.Client

	Username, Password string
}


func NewRetryHttpClient(userName, password string) (*RetryHttpClient, error) {
  rc := retryablehttp.NewClient()

  conf := &tls.Config{
	InsecureSkipVerify: true,
	MinVersion:         tls.VersionTLS12,
  }

  rc.HTTPClient.Transport = &http.Transport{TLSClientConfig: conf}
  rc.RetryMax = 5
  rc.Logger = nil 
  c := &RetryHttpClient{
	Client:   rc,
	Username: userName,
	Password: password,
  }

  return c, nil
}

func (c *RetryHttpClient) Do(ctx context.Context, reqBytes interface{}, endpoint string, customHeader map[string]string, method string) ([]byte, error) {
	var req *http.Request
	var err error
	if reqBytes != nil {
		_reqBytes, err := json.Marshal(reqBytes)

		if err != nil {
	  		return nil, err
		}

		req, err = http.NewRequest(method, endpoint, bytes.NewReader(_reqBytes))
		if err != nil {
	  		return nil, err
		}
	} else {
		req, err = http.NewRequest(method, endpoint, nil)
		if err != nil {
		  return nil, err
		}
	}

	

	retryableReq, err := retryablehttp.NewRequest(req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}

	for k, v := range customHeader {
		retryableReq.Header.Add(k, v)
	}

	retryableReq.Header.Add("Content-Type", "application/json")
	
	if c.Username != "" && c.Password != "" {
		retryableReq.SetBasicAuth(c.Username, c.Password)	
	}
	
	// log.Println(retryableReq.Header)
	
	// log.Println(retryableReq.Header)
	resp, err := c.Client.Do(retryableReq.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

  	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusServiceUnavailable:
		return nil, fmt.Errorf("service unavailable: %s", resp.Body)
	case http.StatusGatewayTimeout:
		return nil, fmt.Errorf("gateway timeout: %s", resp.Body)
	case http.StatusBadGateway:
		return nil, fmt.Errorf("bad gateway: %s", resp.Body)
	case http.StatusRequestTimeout:
		return nil, fmt.Errorf("timeout: %s", resp.Body)
	case http.StatusForbidden:
		return nil, fmt.Errorf("forbidden: %s", resp.Body)
	case http.StatusNotFound:
		return nil, fmt.Errorf("not found: %s", resp.Body)
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized: %s", resp.Body)
	}


	return body, nil
}

func (r *resources) eventVMWatching() {
	httpClient, _ := NewRetryHttpClient("", "")
	pid := os.Getpid()

	myProcess, _ := os.FindProcess(pid)

	for {
		if resp, err := httpClient.Do(context.Background(), nil, "http://metadata.google.internal/computeMetadata/v1/instance/maintenance-event", map[string]string{"Metadata-Flavor": "Google"}, "GET"); err == nil {
			if string(resp) != "NONE" {
				if err := myProcess.Signal(syscall.SIGTERM); err != nil {
					fmt.Println(err)
				}
				break
			}
			time.Sleep(time.Duration(2) * time.Second)
		}
	}
}

func (r *resources) eventPrempVMWatching() {
	httpClient, _ := NewRetryHttpClient("", "")
	pid := os.Getpid()

	myProcess, _ := os.FindProcess(pid)

	for {
		if resp, err := httpClient.Do(context.Background(), nil, "http://metadata.google.internal/computeMetadata/v1/instance/preempted", map[string]string{"Metadata-Flavor": "Google"}, "GET"); err == nil {
			if string(resp) != "FALSE" {
				if err := myProcess.Signal(syscall.SIGTERM); err != nil {
					fmt.Println(err)
				}
				break
			}
			time.Sleep(time.Duration(2) * time.Second)
		}
	}
}


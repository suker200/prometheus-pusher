package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
	"strconv"
	"regexp"
	"math/rand"
	"github.com/pelletier/go-toml"
)

// generate random string
func randomString() string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	    "abcdefghijklmnopqrstuvwxyz" +
	    "0123456789")
	length := 4
	var b strings.Builder
	for i := 0; i < length; i++ {
	    b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// reads all config files into []byte
//
func concatConfigFiles(path string) ([]byte, error) {
	var config []byte

	if runtime != "local" {
		return config, nil
	}

	pathCheck, err := os.Open(path)
	if err != nil {
		return []byte{}, err
	}

	pathInfo, err := pathCheck.Stat()
	if err != nil {
		return []byte{}, err
	}

	if pathInfo.IsDir() {
		dir, _ := pathCheck.Readdir(-1)
		buf := make([][]byte, len(dir))
		for _, file := range dir {
			if strings.HasSuffix(file.Name(), ".toml") && (file.Mode().IsRegular()) {
				fileName := path + "/" + file.Name()
				data, err2 := ioutil.ReadFile(fileName)
				if err2 != nil {
					logger.Errorf("Failed to read config file %s - %s", fileName, err2.Error())
					continue
				}
				buf = append(buf, data)
			}
		}
		config = bytes.Join(buf, []byte("\n"))
		return config, nil
	}

	config, err = ioutil.ReadFile(path)
	if err != nil {
		return []byte{}, err
	}
	return config, nil
}

// resource config type
//
type resourceConfig struct {
	pushGatewayURL string
	defaultRoute   string
	resURL         string
	port           int
	host           string
	ssl            bool
	path           string
	routeMap       string
}

// global pusher config type
// it contains instances of resourceConfig
//
type pusherConfig struct {
	envLabels      map[string]string
	pushGatewayURL string
	defaultRoute   string
	pushInterval   time.Duration
	routeMap       string
	resources      map[string]*resourceConfig
}

// parses []byte with TOML config data into pusherConfig
// instance
//
func parseConfig(data []byte) (*pusherConfig, error) {
	p := &pusherConfig{
		pushInterval: time.Duration(60) * time.Second,
		resources:    make(map[string]*resourceConfig),
	}

	if runtime != "local" {
		p.pushGatewayURL = os.Getenv("PUSH_GATEWAY_URL")
		interval, _ := strconv.Atoi(os.Getenv("PUSH_GATEWAY_INTERVAL"))
		p.pushInterval = time.Duration(interval) * time.Second

		port, _ := strconv.Atoi(os.Getenv("PUSH_GATEWAY_RESOURCE_PORT"))

		res := &resourceConfig{
			pushGatewayURL: p.pushGatewayURL,
			defaultRoute:   p.defaultRoute,
			resURL:         "",
			host:           "localhost",
			port:           port,
			ssl:            false,
			path:           "metrics",
			routeMap:       p.routeMap,
		}
		res.resURL = fmt.Sprintf("%s://%s:%d/%s", "http", res.host, res.port, res.path)
		p.resources[runtime] = res

		envs := os.Environ()
		envLabelsMap := make(map[string]string)
		reReplace := regexp.MustCompile("PROMETHEUS_LABEL_")

		for _, e := range envs {
			if match, _ := regexp.Match("^PROMETHEUS_LABEL_.*", []byte(e)); match {
				key := fmt.Sprintf("%s", reReplace.ReplaceAll([]byte(strings.Split(e, "=")[0]), []byte("")))
				val := strings.Split(e, "=")[1]
				envLabelsMap[strings.ToLower(key)] = val
			}
		}

		// envLabelsMap["container"] = randomString()

		p.envLabels = envLabelsMap
	} else {
		rd := bytes.NewReader(data)
		t, err := toml.LoadReader(rd)
		if err != nil {
			return nil, err
		}

		envLabelLabels := make([]interface{}, 0)
		envLabelsSet := false
		if t.Has("default_env_labels") && t.Has("default_env_labels.env_labels") {
			envLabelLabels = append(envLabelLabels, t.Get("default_env_labels.env_labels").([]interface{})...)
			envLabelsSet = true
		}
		if t.Has("service_env_labels") && t.Has("service_env_labels.env_labels") {
			envLabelLabels = append(envLabelLabels, t.Get("service_env_labels.env_labels").([]interface{})...)
			envLabelsSet = true
		}

		if envLabelsSet {
			envLabelsMap := make(map[string]string)
			for _, label := range envLabelLabels {
				strLabel := label.(string)
				val := os.Getenv(strLabel)
				if len(val) != 0 {
					envLabelsMap[strings.ToLower(strLabel)] = val
					logger.Debugf("Got additional ENV label %s with value %s", strings.ToLower(strLabel), val)
				}
			}
			p.envLabels = envLabelsMap
		}

		if t.Has("config.pushgateway_url") {
			p.pushGatewayURL = t.Get("config.pushgateway_url").(string)
		} else {
			p.pushGatewayURL = "http://localhost:9091/metrics"
		}

		if t.Has("config.push_interval") {
			p.pushInterval = time.Duration(t.Get("config.push_interval").(int64)) * time.Second
		}

		if t.Has("config.route_map") {
			p.routeMap = t.Get("config.route_map").(string)
		}

		if t.Has("config.default_route") {
			p.defaultRoute = t.Get("config.default_route").(string)
		}


		for _, resName := range t.Keys() {
			if resName == "config" || resName == "default_env_labels" || resName == "service_env_labels" {
				continue
			}

			res := &resourceConfig{
				pushGatewayURL: p.pushGatewayURL,
				defaultRoute:   p.defaultRoute,
				resURL:         "",
				host:           "localhost",
				port:           0,
				ssl:            false,
				path:           "metrics",
				routeMap:       p.routeMap,
			}

			if t.Has(resName + ".port") {
				res.port = int(t.Get(resName + ".port").(int64))
			} else {
				logger.Fatalf("missing port for resource '%s', exiting", resName)
				continue
			}

			if t.Has(resName + ".pushgateway_url") {
				res.pushGatewayURL = t.Get(resName + ".pushgateway_url").(string)
			}

			if t.Has(resName + ".default_route") {
				res.defaultRoute = t.Get(resName + ".default_route").(string)
			}

			if t.Has(resName + ".host") {
				res.host = t.Get(resName + ".host").(string)
			}

			if t.Has(resName + ".ssl") {
				res.ssl = t.Get(resName + ".ssl").(bool)
			}

			if t.Has(resName + ".path") {
				res.path = t.Get(resName + ".path").(string)
				res.path = strings.TrimPrefix(res.path, "/")
			}

			if t.Has(resName + ".route_map") {
				res.routeMap = t.Get(resName + ".path").(string)
			}
			var scheme string
			if res.ssl {
				scheme = "https"
			} else {
				scheme = "http"
			}

			res.resURL = fmt.Sprintf("%s://%s:%d/%s", scheme, res.host, res.port, res.path)
			p.resources[resName] = res
		}
	}
	return p, nil
}

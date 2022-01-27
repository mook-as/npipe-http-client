package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/Microsoft/go-winio"
	"github.com/sirupsen/logrus"
)

var verbosity = flag.Int("v", 0, "set verbosity")
var method = flag.String("X", "GET", "HTTP method")

func main() {
	flag.Parse()
	logrus.SetLevel(logrus.InfoLevel + logrus.Level(*verbosity))

	pipePath := "//./pipe/docker_engine"
	requestPath := "/info"
	switch len(flag.Args()) {
	case 0:
		// use defaults
	case 1:
		requestPath = flag.Arg(0)
	case 2:
		pipePath = flag.Arg(0)
		requestPath = flag.Arg(1)
	default:
		logrus.WithField("args", flag.Args()).Fatal("Incorrect usage")
	}

	err := request(context.Background(), pipePath, requestPath, *method)
	if err != nil {
		logrus.WithError(err).WithField("pipe", pipePath).WithField("request", requestPath).Fatal("could not make request")
	}
}

func makeClient(pipePath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := winio.DialPipeContext(ctx, pipePath)
				logrus.WithField("path", pipePath).WithContext(ctx).WithError(err).WithField("conn", conn).Trace("dialing pipe")
				if err != nil {
					return conn, err
				}
				return conn, nil
			},
		},
	}
}

func request(ctx context.Context, pipePath, requestPath, method string) error {
	if strings.Contains(pipePath, ":") {
		parts := strings.SplitN(pipePath, ":", -1)
		pipePath = parts[len(parts)-1]
	}
	if strings.Contains(requestPath, ":") {
		parts := strings.SplitN(requestPath, ":", -1)
		requestPath = parts[len(parts)-1]
	}
	requestURL := "http://host" + requestPath
	logEntry := logrus.WithField("pipe", pipePath).WithField("request", requestURL)
	logEntry.Trace("making request")

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		requestURL,
		&bytes.Buffer{},
	)
	if err != nil {
		logEntry.WithError(err).Fatal("could not create request")
	}

	resp, err := makeClient(pipePath).Do(req)
	if err != nil {
		logEntry.WithError(err).Fatal("could not execute request")
	}

	logEntry.WithField("response", resp).Trace("got response")
	
	if contentType := resp.Header.Get("Content-Type"); contentType != "application/json" {
		logEntry.WithField("content-type", contentType).Trace("unexpected content type")
		return nil
	}

	var data interface{}
	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		logEntry.WithError(err).Fatal("could not read response body")
	}
	if err = json.Unmarshal(blob, &data); err != nil {
		logEntry.WithError(err).Fatal("could not unmarshal response body")
	}
	logEntry.WithField("body", fmt.Sprintf("%+v", data)).Trace("got response")

	return nil
}
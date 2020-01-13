package http

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/github/orchestrator-agent/go/helper/config"
	"github.com/github/orchestrator-agent/go/helper/ssl"
	"github.com/go-martini/martini"
	log "github.com/sirupsen/logrus"
)

// InitHTTPClient initialize new http client
func InitHTTPClient(httpTimeout *config.Duration, sslSkipVerify bool, sslCAFile string, useMutualTLS bool, sslCertFile string, sslPrivateKeyFile string, logger *log.Entry) *http.Client {
	timeout := httpTimeout.Duration
	dialTimeout := func(network, addr string) (net.Conn, error) {
		return net.DialTimeout(network, addr, timeout)
	}
	tlsConfig, err := buildTLS(sslSkipVerify, sslCAFile, useMutualTLS, sslCertFile, sslPrivateKeyFile)
	if err != nil {
		logger.Error(err)
	}
	httpTransport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		Dial:                  dialTimeout,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Transport: httpTransport}
}

func buildTLS(sslSkipVerify bool, sslCAFile string, useMutualTLS bool, sslCertFile string, sslPrivateKeyFile string) (*tls.Config, error) {
	tlsConfig, err := ssl.NewTLSConfig(sslCAFile, useMutualTLS)
	if err != nil {
		return tlsConfig, err
	}
	_ = ssl.AppendKeyPair(tlsConfig, sslCertFile, sslPrivateKeyFile)
	tlsConfig.InsecureSkipVerify = sslSkipVerify
	return tlsConfig, nil
}

// NewMartini creates new instance of martini with configured logger
func NewMartini() *martini.ClassicMartini {
	martini.Env = martini.Prod
	apiLogger := log.WithFields(log.Fields{"prefix": "API"})
	r := martini.NewRouter()
	m := martini.New()
	m.Use(martiniLogger())
	m.Use(martini.Recovery())
	m.Use(martini.Static("public"))
	m.Map(apiLogger)
	m.MapTo(r, (*martini.Routes)(nil))
	m.Action(r.Handle)
	return &martini.ClassicMartini{m, r}
}

func martiniLogger() martini.Handler {
	return func(res http.ResponseWriter, req *http.Request, c martini.Context, logger *log.Entry) {
		start := time.Now()

		addr := req.Header.Get("X-Real-IP")
		if addr == "" {
			addr = req.Header.Get("X-Forwarded-For")
			if addr == "" {
				addr = req.RemoteAddr
			}
		}

		logger.WithFields(log.Fields{"method": req.Method, "URL": req.URL.Path, "address": addr}).Debug("Processing request")

		rw := res.(martini.ResponseWriter)
		c.Next()

		logger.WithFields(log.Fields{"URL": req.URL.Path, "status": rw.Status(), "statusText": http.StatusText(rw.Status()), "time": time.Since(start)}).Debug("Request completed")

	}
}

package nsqlookupd

import (
	"log"
	"os"
	"time"
)

type nsqlookupdOptions struct {
	Verbose bool `flag:"verbose"`

	TCPAddress           string `flag:"tcp-address"`
	HTTPAddress          string `flag:"http-address"`
	TCPBroadcastAddress  string `flag:"tcp-broadcast-address"`
	HTTPBroadcastAddress string `flag:"http-broadcast-address"`

	InactiveProducerTimeout time.Duration `flag:"inactive-producer-timeout"`
	TombstoneLifetime       time.Duration `flag:"tombstone-lifetime"`

	Logger logger
}

func NewNSQLookupdOptions() *nsqlookupdOptions {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}

	return &nsqlookupdOptions{
		TCPAddress:           "0.0.0.0:4160",
		HTTPAddress:          "0.0.0.0:4161",
		TCPBroadcastAddress:  hostname,
		HTTPBroadcastAddress: hostname,

		InactiveProducerTimeout: 300 * time.Second,
		TombstoneLifetime:       45 * time.Second,

		Logger: log.New(os.Stderr, "[nsqlookupd] ", log.Ldate|log.Ltime|log.Lmicroseconds),
	}
}

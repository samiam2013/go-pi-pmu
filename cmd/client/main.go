package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
	"github.com/spf13/pflag"
)

func main() {
	// flag allowing testing mode where it creates it's own data
	test := pflag.BoolP("test", "t", false, "testing mode - send random measurements")
	pflag.Parse()

	switch *test {
	case true:
		runTestClient()
	case false:
		panic("normal mode not yet implemented; use -t to test")
	}
}

func runTestClient() {
	// start an http client
	// make the request
	measurement := &protobuf.Measurement{
		Voltage: 1,
		Current: 1,
	}

	client := http.Client{}
	req, err := http.NewRequest(http.MethodPost, "http://localhost:8080", nil)
	if err != nil {
		panic(err)
	}
	// set the content type to application/x-protobuf
	req.Header.Set("Content-Type", "application/x-protobuf")

	for {
		reqB, err := proto.Marshal(measurement)
		if err != nil {
			panic(err)
		}
		measurement.Epochnano = time.Now().UnixNano()
		req.Body = io.NopCloser(bytes.NewBuffer(reqB))

		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}

		respB, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		resp.Body.Close()
		log.Printf("response: %s\n", string(respB))
	}

}

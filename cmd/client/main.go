package main

import (
	"bytes"
	"io"
	"log"
	"math"
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

	start := time.Now()
	for i := 0; true; i++ {
		measurement.Epochnano = time.Now().UnixNano()
		// generate a point on the sine wave of 60 hz
		secF := float64(time.Now().Nanosecond()) / 1_000_000_000.00
		hzWavelength := time.Second / 60.0
		remainder := math.Mod(secF, float64(hzWavelength))
		angle := (remainder / hzWavelength.Seconds()) * (math.Pi * 2)
		measurement.Voltage = int32(math.Sin(angle/(math.Pi*2)) * 120)
		measurement.Current = int32(math.Abs(float64(measurement.Voltage / 10)))
		// log.Printf("Remainder: %f Angle %f Voltage: %d", remainder, angle, measurement.Voltage)
		reqB, err := proto.Marshal(measurement)
		if err != nil {
			panic(err)
		}
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
		_ = respB
		if i%1_000 == 0 {
			log.Printf("response: %s\n", string(respB))
		}
		if i%100_000 == 0 && i != 0 {
			d := time.Since(start)
			avgPerMeas := d / time.Duration(i)
			perSec := int64(time.Second / avgPerMeas)
			log.Printf("Sent %d measurments in %v ; avg %d per sec", i, d, perSec)
		}
	}

}

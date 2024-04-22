package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
	"github.com/spf13/pflag"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
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
	series := &protobuf.Series{
		Measurements: []*protobuf.Series_Measurement{},
	}

	const batchSize = 100

	start := time.Now()
	limiter := rate.NewLimiter(rate.Limit(1200), 1)
	for i := 0; true; i++ {
		if err := limiter.Wait(context.Background()); err != nil {
			panic(err)
		}
		measurement := &protobuf.Series_Measurement{}
		measurement.Epochnano = time.Now().UnixNano()
		// generate a point on the sine wave of 60 hz
		secF := float64(time.Now().Nanosecond()) / 1_000_000_000.00
		hzWavelength := time.Second / 60.0
		remainder := math.Mod(secF, float64(hzWavelength))
		angle := (remainder / hzWavelength.Seconds()) * (math.Pi * 2)
		measurement.Voltage = int32(math.Sin(angle/(math.Pi*2)) * 120)
		measurement.Current = int32(math.Abs(float64(measurement.Voltage / 10)))
		// log.Printf("Remainder: %f Angle %f Voltage: %d", remainder, angle, measurement.Voltage)
		series.Measurements = append(series.Measurements, measurement)

		if i%batchSize == 0 {
			reqB, err := proto.Marshal(series)
			if err != nil {
				panic(err)
			}

			go func(reqB []byte) {
				client := http.Client{}
				req, err := http.NewRequest(http.MethodPost, "http://rpi5:8080",
					io.NopCloser(bytes.NewBuffer(reqB)))
				if err != nil {
					panic(err)
				}
				// set the content type to application/x-protobuf
				req.Header.Set("Content-Type", "application/x-protobuf")

				resp, err := client.Do(req)
				if err != nil {
					panic(err)
				}
				_ = resp
			}(reqB[:])
			series = &protobuf.Series{}
		}

		if i%10_000 == 0 && i != 0 {
			d := time.Since(start)
			avgPerMeas := d / time.Duration(i)
			perSec := int64(time.Second / avgPerMeas)
			log.Printf("Sent %d measurements in %v ; avg %d per sec", i, d, perSec)
		}
	}

}

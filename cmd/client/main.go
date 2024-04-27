package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
	"github.com/spf13/pflag"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
	"periph.io/x/conn/v3/analog"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/devices/v3/ads1x15"
	"periph.io/x/host/v3"
)

func main() {
	// flag allowing testing mode where it creates it's own data
	test := pflag.BoolP("test", "t", false, "testing mode - send random measurements")
	pflag.Parse()

	switch *test {
	case true:
		runTestClient()
	case false:
		runClient()
	}
}

func runClient() {

	// Make sure periph is initialized.
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	// Open default I²C bus.
	bus, err := i2creg.Open("")
	if err != nil {
		log.Fatalf("failed to open I²C: %v", err)
	}
	defer bus.Close()

	// Create a new ADS1115 ADC.
	adc, err := ads1x15.NewADS1115(bus, &ads1x15.DefaultOpts)
	if err != nil {
		log.Fatalln(err)
	}

	// Obtain an analog pin from the ADC.
	// pin, err := adc.PinForChannel(ads1x15.Channel0Minus1, 1*physic.Volt, 121*physic.Hertz, ads1x15.SaveEnergy)
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	// defer pin.Halt()

	pin3, err := adc.PinForChannel(ads1x15.Channel2, physic.Volt*3, 120*physic.Hertz, ads1x15.BestQuality)
	if err != nil {
		log.Fatalln(err)
	}

	// // Read values from ADC.
	// fmt.Println("Single reading")
	// reading, err := pin3.Read()

	// if err != nil {
	// 	log.Fatalln(err)
	// }

	// fmt.Println(reading)

	// Read values continuously from ADC.
	// fmt.Println("Continuous reading")
	c := pin3.ReadContinuous()

	i := 0
	results := map[int64]analog.Sample{}
	for reading := range c {
		results[time.Now().UnixNano()] = reading
		i++
		if i > 1_200 {
			break
		}
	}
	max := "0.000V"
	min := "9.999V"
	for time, result := range results {
		if result.V.String() > max {
			max = result.V.String()
		}
		if result.V.String() < min {
			min = result.V.String()
		}
		fmt.Println(time, result)
	}
	fmt.Println("min", min, "max", max)
	minf, _ := strconv.ParseFloat(strings.TrimRight(min, "V"), 64)
	maxf, _ := strconv.ParseFloat(strings.TrimRight(max, "V"), 64)
	diff := maxf - minf
	scaleV := 340 / diff
	fmt.Print("scale factor", scaleV)
	avg := minf + (diff / 2)
	for time, result := range results {
		voltF, _ := strconv.ParseFloat(strings.TrimRight(result.V.String(), "V"), 64)
		d := -(avg - voltF)
		scaledDiff := d * scaleV
		fmt.Println(time, scaledDiff)
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
		measurement.Voltage = int32(math.Sin(angle/(math.Pi*2)) * 240)
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

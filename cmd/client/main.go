package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"periph.io/x/conn/v3/analog"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/devices/v3/ads1x15"
	"periph.io/x/host/v3"
)

func main() {
	// flag allowing testing mode where it creates it's own data
	// test := pflag.BoolP("test", "t", false, "testing mode - send random measurements")
	// pflag.Parse()

	runClient()
}

func runClient() {
	if _, err := host.Init(); err != nil {
		logrus.Fatal(err)
	}

	// Open default I²C bus.
	bus, err := i2creg.Open("")
	if err != nil {
		logrus.Fatalf("failed to open I²C: %v", err)
	}
	defer func() { _ = bus.Close() }()

	// Create a new ADS1115 ADC.
	adc, err := ads1x15.NewADS1115(bus, &ads1x15.DefaultOpts)
	if err != nil {
		logrus.Fatalln(err)
	}

	// ADC pins 0 & 1 - current reading
	cPin, err := adc.PinForChannel(ads1x15.Channel0Minus1, 1*physic.Volt, 150*physic.Hertz, ads1x15.SaveEnergy)
	if err != nil {
		logrus.Fatalln(err)
	}
	defer func() { _ = cPin.Halt() }()

	// ADC pin 2 - voltage reading
	vPin, err := adc.PinForChannel(ads1x15.Channel2, 3*physic.Volt, 150*physic.Hertz, ads1x15.SaveEnergy)
	if err != nil {
		logrus.Fatalln(err)
	}
	defer func() { _ = vPin.Halt() }()

	vCont := vPin.ReadContinuous()
	cCont := cPin.ReadContinuous()

	funnel := make(chan sample, 64)
	go func(ret chan sample) {
		for reading := range vCont {
			ret <- sample{kind: protobuf.SampleKind_VOLTAGE, data: reading, UnixNano: time.Now().UnixNano()}
		}
	}(funnel)
	go func(ret chan sample) {
		for reading := range cCont {
			ret <- sample{kind: protobuf.SampleKind_CURRENT, data: reading, UnixNano: time.Now().UnixNano()}
		}
	}(funnel)

	sampleBuf := make([]sample, 0, 1024)
	for smpl := range funnel {
		sampleBuf = append(sampleBuf, smpl)
		// logrus.Infof("sample size %d", len(sampleBuf))
		if len(sampleBuf) >= 1024 {
			go func(samples []sample) {
				if err := send(samples); err != nil {
					logrus.WithError(err).Error("Failed to send series")
				}
			}(sampleBuf[:]) // pass a copy on the stack so it can't remove the reference
			sampleBuf = make([]sample, 0, 1024)
		}
	}
}

type sample struct {
	data     analog.Sample
	kind     protobuf.SampleKind
	UnixNano int64
}

func send(seriesData []sample) error {
	series := &protobuf.Series{}
	for _, s := range seriesData {
		series.Measurements = append(series.Measurements,
			&protobuf.Measurement{
				Nanovolts:  int64(s.data.V),
				Rawsample:  int64(s.data.Raw),
				Epochnano:  s.UnixNano,
				Samplekind: s.kind,
			})
	}

	reqB, err := proto.Marshal(series)
	if err != nil {
		return fmt.Errorf("failed marshalling series for send: %w", err)
	}

	client := http.Client{}
	req, err := http.NewRequest(http.MethodPost, "http://rpi5:8080",
		io.NopCloser(bytes.NewBuffer(reqB)))
	if err != nil {
		return fmt.Errorf("failed to make request for send: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("faild to execude request for send: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response code after sending not ok: %w", err)
	}
	return nil
}

func runTestClient() {
	// TODO: maybe re-implement this?
}

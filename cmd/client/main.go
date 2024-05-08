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
	runClient()
}

func runClient() {
	if _, err := host.Init(); err != nil {
		logrus.Fatal(err)
	}

	vBus, err := i2creg.Open("0")
	if err != nil {
		logrus.Fatalf("failed to open I²C: %v", err)
	}
	defer func() { _ = vBus.Close() }()

	cBus, err := i2creg.Open("1")
	if err != nil {
		logrus.Fatalf("failed to open I²C: %v", err)
	}
	defer func() { _ = vBus.Close() }()

	// Create a new ADS1115 ADC, one for voltage, one for current
	cADC, err := ads1x15.NewADS1115(cBus, &ads1x15.DefaultOpts)
	if err != nil {
		logrus.Fatalln(err)
	}

	vADC, err := ads1x15.NewADS1115(vBus, &ads1x15.DefaultOpts)
	if err != nil {
		logrus.Fatalln(err)
	}

	// ADC pins 0 & 1 - current reading
	cPin, err := cADC.PinForChannel(ads1x15.Channel0Minus1, 1*physic.Volt, 512*physic.Hertz, ads1x15.BestQuality)
	if err != nil {
		logrus.Fatalln(err)
	}
	defer func() { _ = cPin.Halt() }()

	// ADC pin 2 - voltage reading
	vPin, err := vADC.PinForChannel(ads1x15.Channel2, 5*physic.Volt, 512*physic.Hertz, ads1x15.BestQuality)
	if err != nil {
		logrus.Fatalln(err)
	}
	defer func() { _ = vPin.Halt() }()

	vCont := vPin.ReadContinuous()
	cCont := cPin.ReadContinuous()

	funnel := make(chan sample, 1024000)
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

	sampleBuf := make([]sample, 0, 10240)
	for s := range funnel {
		sampleBuf = append(sampleBuf, s)
		// logrus.Infof("sample size %d", len(sampleBuf))
		if len(sampleBuf) >= 10240 {
			go func(samples []sample) {
				logrus.Infof("sending series starting with %+v", samples[0])
				logrus.Infof("series ending with %+v", samples[10239])
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
		return fmt.Errorf("failed to execute request for send: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response code after sending not ok: %w", err)
	}
	return nil
}

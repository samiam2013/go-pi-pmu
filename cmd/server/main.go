package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	_ "github.com/lib/pq"
)

func main() {
	connectStr := "postgresql://grafana:grafana@localhost/grafana?sslmode=disable"
	db, err := sql.Open("postgres", connectStr)
	if err != nil {
		log.Fatal(err)
	}
	if err := migrateSchema(db); err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// write the protobuf to the response
			w.Header().Set("Content-Type", "application/x-protobuf")
			w.WriteHeader(http.StatusOK)

			var series protobuf.Series
			reqBody, err := io.ReadAll(r.Body)
			if err != nil {
				logrus.WithError(err).Fatal("failed to read request")
			}
			if err := proto.Unmarshal(reqBody, &series); err != nil {
				logrus.WithError(err).Fatal("failed to unmarshal protobuf data")
			}
			// if _, err := w.Write([]byte(series.String())); err != nil {
			// 	logrus.WithError(err).Fatal("")
			// }
			lenMeasurements := len(series.Measurements)
			if lenMeasurements == 0 {
				logrus.Error("No measurements in decoded series")
				return
			}

			// do analysis of frequency
			start := series.Measurements[0].Epochnano
			i, min, max, vCount, sum := int64(0), int64(math.MaxInt64), int64(math.MinInt64), int64(0), int64(0)
			nsInSecond := 1_000_000_000
			for time := start; time < (start + int64(nsInSecond)); {
				m := series.Measurements[i]
				if m.Samplekind == protobuf.SampleKind_CURRENT {
					i++
					continue
				}
				time = m.Epochnano
				// logrus.Infof("time: ", time)
				rs := m.Rawsample
				if rs > max {
					max = rs
				}
				if rs < min {
					min = rs
				}
				sum += rs
				vCount++
				i++
			}
			// logrus.Infof("end of first sampling loop")
			average := sum / vCount
			// logrus.Infof("min: %d, max %d, avg %d, count: %d", min, max, average, i)

			zeroCrossings := int64(0)
			lastSample := series.Measurements[0].Rawsample
			for i := 1; i < len(series.Measurements); i++ {
				m := series.Measurements[i]
				if m.Samplekind == protobuf.SampleKind_CURRENT {
					continue
				}
				rs := m.Rawsample
				if (lastSample > average && rs <= average) ||
					(lastSample < average && rs >= average) {
					zeroCrossings++
				}
				lastSample = rs
			}
			// logrus.Info("end of second sampling loop")
			d := time.Duration(series.Measurements[len(series.Measurements)-1].Epochnano -
				series.Measurements[0].Epochnano)
			frequency := (float64(zeroCrossings) / 2.0) / d.Seconds()
			logrus.Infof("Frequency: %.2f", frequency)

			// build an insert for this data
			queryPrefix := "INSERT INTO pmu(sample_kind, nano_volts, raw_sample, epoch_nano) VALUES"
			var sb strings.Builder
			for _, measurement := range series.Measurements {
				sb.WriteString(fmt.Sprintf("('%s', %d, %d, %d),",
					strings.ToLower(measurement.Samplekind.String()),
					measurement.Nanovolts,
					measurement.Rawsample,
					measurement.Epochnano))
			}
			query := queryPrefix + strings.TrimRight(sb.String(), ",")
			if _, err := db.Exec(query); err != nil {
				logrus.WithError(err).WithField("query", query).Error("failed to insert data")
			}
			logrus.Infof("Inserted %d measurements.", lenMeasurements)
		}),
	}
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}

}

func migrateSchema(db *sql.DB) error {
	query := "SELECT version FROM migration ORDER BY version DESC LIMIT 1"
	row := db.QueryRow(query)
	var version int64
	if err := row.Scan(&version); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed querying for migration version: %w", err)
	}
	log.Printf("loaded migration: %d", version)
	migrations := map[int64]string{
		1: "CREATE TABLE IF NOT EXISTS pmu ( voltage INT, current INT, epoch_nano BIGINT)",
		2: "CREATE INDEX IF NOT EXISTS idx_epoch_nano ON pmu (epoch_nano)",
		3: "ALTER TABLE pmu DROP COLUMN current",
		4: "CREATE TYPE measurement_sample_kind AS ENUM ('current', 'voltage')",
		5: "ALTER TABLE pmu ADD COLUMN sample_kind measurement_sample_kind",
		6: "ALTER TABLE pmu ADD COLUMN raw_sample BIGINT",
		7: "ALTER TABLE pmu DROP COLUMN voltage",
		8: "ALTER TABLE pmu ADD COLUMN nano_volts BIGINT",
	}
	for i := int64(1); true; i++ {
		v, ok := migrations[i]
		if !ok {
			logrus.Infof("migration %d did not exist, stopping at %d", i, i-1)
			break
		}
		if version >= i {
			// logrus.Infof("migration %d below version %d, skipping", i, version)
			continue
		}
		if _, err := db.Exec(v); err != nil {
			return fmt.Errorf("failed migration %s, %w", v, err)
		}
		upMigVer := "INSERT INTO migration(version) VALUES($1)"
		if _, err := db.Exec(upMigVer, i); err != nil {
			return fmt.Errorf("failed updating migration version: %v", err)
		}
	}
	return nil
}

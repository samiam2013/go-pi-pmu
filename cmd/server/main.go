package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
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
				panic(err)
			}
			if err := proto.Unmarshal(reqBody, &series); err != nil {
				panic(err)
			}
			if _, err := w.Write([]byte(series.String())); err != nil {
				panic(err)
			}

			// build an insert for this data
			queryPrefix := "INSERT INTO pmu(sample_kind, voltage, raw_sample, epoch_nano) VALUES"
			var sb strings.Builder
			sb.WriteString(queryPrefix)
			for _, measurement := range series.Measurements {
				sb.WriteString(fmt.Sprintf("(%d, %f, %d, %d),",
					measurement.Samplekind,
					measurement.Voltage,
					measurement.Rawsample,
					measurement.Epochnano))
			}
			if _, err := db.Exec(strings.TrimRight(sb.String(), ",")); err != nil {
				log.Printf("failed to insert data: %v", err)
			}

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
		4: "CREATE TYPE measurement_sample_kind as ('current', 'voltage')",
		5: "ALTER TABLE pmu ADD COLUMN sample_kind measurement_sample_kind",
		6: "ALTER TABLE pmu ADD COLUMN raw_sample BIGINT",
	}
	for i := int64(1); true; i++ {
		v, ok := migrations[i]
		if !ok {
			break
		}
		if version >= i {
			break
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

package main

import (
	"io"
	"net/http"

	"github.com/samiam2013/go-pi-pmu/measurement/protobuf"
	"google.golang.org/protobuf/proto"
)

func main() {
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
			// log.Print(series.String())
		}),
	}
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}

}

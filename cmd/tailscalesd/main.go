package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cfunkhouser/tailscalesd"
)

var (
	address        string = "0.0.0.0:9242"
	token          string
	tailnet        string
	printVer       bool
	pollLimit      time.Duration = time.Minute * 5
	useLocalAPI    bool
	localAPISocket string = tailscalesd.PublicAPIHost

	// Version of tailscalesd. Set at build time to something meaningful.
	Version = "development"
)

func envVarWithDefault(key, def string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return def
}

func boolEnvVarWithDefault(key string, def bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "true" || val == "yes"
	}
	return def
}

func durationEnvVarWithDefault(key string, def time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		d, err := time.ParseDuration(val)
		if err == nil {
			return d
		}
		log.Printf("Duration parsing failed, using default %q: %v", def, err)
	}
	return def
}

func defineFlags() {
	flag.StringVar(&address, "address", envVarWithDefault("LISTEN", address), "Address on which to serve Tailscale SD")
	flag.StringVar(&token, "token", os.Getenv("TAILSCALE_API_TOKEN"), "Tailscale API Token")
	flag.StringVar(&tailnet, "tailnet", os.Getenv("TAILNET"), "Tailnet name.")
	flag.DurationVar(&pollLimit, "poll", durationEnvVarWithDefault("TAILSCALE_API_POLL_LIMIT", pollLimit), "Max frequency with which to poll the Tailscale API. Cached results are served between intervals.")
	flag.BoolVar(&printVer, "version", false, "Print the version and exit.")
	flag.BoolVar(&useLocalAPI, "localapi", boolEnvVarWithDefault("TAILSCALE_USE_LOCAL_API", false), "Use the Tailscale local API exported by the local node's tailscaled")
	flag.StringVar(&localAPISocket, "localapi_socket", envVarWithDefault("TAILSCALE_LOCAL_API_SOCKET", localAPISocket), "Unix Domain Socket to use for communication with the local tailscaled API.")
}

type logWriter struct {
	TZ     *time.Location
	Format string
}

func (w *logWriter) Write(data []byte) (int, error) {
	return fmt.Printf("%v %v", time.Now().In(w.TZ).Format(w.Format), string(data))
}

func main() {
	log.SetFlags(0)
	log.SetOutput(&logWriter{
		TZ:     time.UTC,
		Format: time.RFC3339,
	})

	defineFlags()
	flag.Parse()

	if printVer {
		fmt.Printf("tailscalesd version %v\n", Version)
		return
	}

	if !useLocalAPI && (token == "" || tailnet == "") {
		if _, err := fmt.Fprintln(os.Stderr, "Both -token and -tailnet are required when using the public API"); err != nil {
			panic(err)
		}
		flag.Usage()
		return
	}

	if useLocalAPI && localAPISocket == "" {
		if _, err := fmt.Fprintln(os.Stderr, "-localapi_socket must not be empty when using the local API."); err != nil {
			panic(err)
		}
		flag.Usage()
		return
	}

	var ts tailscalesd.Discoverer
	if useLocalAPI {
		ts = tailscalesd.LocalAPI(tailscalesd.LocalAPISocket)
	} else {
		ts = tailscalesd.PublicAPI(tailnet, token)
	}
	ts = tailscalesd.RateLimit(ts, pollLimit)
	http.Handle("/", tailscalesd.Export(ts))
	log.Printf("Serving Tailscale service discovery on %q", address)
	log.Print(http.ListenAndServe(address, nil))
	log.Print("Done")
}

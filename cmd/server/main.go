package main

import (
	"flag"
	"log"

	_ "net/http/pprof"

	appserver "github.com/chengsn/kite-desktop/internal/server"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	// Opt into the new klog behavior so that -stderrthreshold is honored even
	// when -logtostderr=true (the default).
	// Ref: kubernetes/klog#212, kubernetes/klog#432
	flag.Set("legacy_stderr_threshold_behavior", "false") //nolint:errcheck
	flag.Set("stderrthreshold", "INFO")                   //nolint:errcheck
	flag.Parse()

	if err := appserver.RunUntilSignal("localhost:6060"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

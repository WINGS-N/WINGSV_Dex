// Command xrayhelper is the bin/xray child binary. It wraps the WINGS-N libXray fork so
// the main GUI process never has to import xray-core. Two classes of subcommand:
//
//	run    - brings up the TUN + routes and runs the xray engine (privileged; the
//	         net-helper spawns it, because xray-core owns the TUN via its gVisor inbound).
//	convert/ping/test/version - unprivileged utilities the GUI shells out to for building
//	         configs and probing nodes.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"github.com/xtls/libxray/share"
	"github.com/xtls/libxray/xray"
	"github.com/xtls/xray-core/common/platform"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: xray <run|convert|ping|test|version> [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = cmdRun(os.Args[2:])
	case "convert":
		err = cmdConvert()
	case "ping":
		err = cmdPing(os.Args[2:])
	case "test":
		err = cmdTest(os.Args[2:])
	case "version":
		fmt.Println(xray.XrayVersion())
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runConfig is the wrapper the GUI writes next to the real xray config; tunName must match
// the tun inbound name in that config so initIpRoute can find the device xray created.
type runConfig struct {
	TunName     string `json:"tunName,omitempty"`
	TunPriority int    `json:"tunPriority,omitempty"`
	EnableIPv6  bool   `json:"enableIPv6,omitempty"`
	DatDir      string `json:"datDir,omitempty"`
	ConfigPath  string `json:"configPath,omitempty"`
}

func cmdRun(argv []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := fs.String("config", "run.json", "path to the run wrapper json")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	raw, err := os.ReadFile(*configPath)
	if err != nil {
		return err
	}
	var cfg runConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	initGeoEnv(cfg.DatDir)
	if err := xray.RunXray(cfg.ConfigPath); err != nil {
		return err
	}
	// Proxy-only runs (no tun inbound) leave TunName empty: there is no device to address
	// or route, and the process can stay unprivileged.
	if cfg.TunName != "" {
		if err := initIpRoute(cfg.TunName, cfg.TunPriority, cfg.EnableIPv6); err != nil {
			_ = xray.StopXray()
			return err
		}
	}
	// The config load allocates a lot of transient garbage; hand it back to the OS.
	runtime.GC()
	debug.FreeOSMemory()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	return xray.StopXray()
}

// cmdConvert reads share links (vless://..., v2rayN plain/base64, Clash.Meta yaml) from
// stdin and writes the resulting xray config json to stdout.
func cmdConvert() error {
	links, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	conf, err := share.ConvertShareLinksToXrayJson(string(links))
	if err != nil {
		return err
	}
	if conf == nil {
		return errors.New("no outbound produced from input")
	}
	out, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

// initGeoEnv points xray at the directory holding geosite.dat / geoip.dat and the cert
// bundle. libXray used to do this itself inside RunXray; since v26.7.11 it sets no
// environment at all, so an unset asset dir would silently resolve next to the binary and
// every geosite/geoip routing rule would fail to load.
func initGeoEnv(datDir string) {
	if datDir == "" {
		return
	}
	os.Setenv(platform.AssetLocation, datDir)
	os.Setenv(platform.CertLocation, datDir)
}

func cmdPing(argv []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	datDir := fs.String("datdir", "", "geo asset directory")
	configPath := fs.String("config", "", "xray config json path")
	timeout := fs.Int("timeout", 10, "timeout seconds")
	url := fs.String("url", "https://www.gstatic.com/generate_204", "probe url")
	proxy := fs.String("proxy", "", "local proxy to route the probe through")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	initGeoEnv(*datDir)
	delay, err := xray.Ping(*configPath, *timeout, *url, *proxy)
	if err != nil {
		return err
	}
	fmt.Println(delay)
	return nil
}

func cmdTest(argv []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	datDir := fs.String("datdir", "", "geo asset directory")
	configPath := fs.String("config", "", "xray config json path")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	initGeoEnv(*datDir)
	return xray.TestXray(*configPath)
}

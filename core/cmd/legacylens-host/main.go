package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"legacylens/core/internal/adapters/localapi"
	"legacylens/core/internal/adapters/native"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	// Chrome supplies the extension origin as a process argument. The parent window handle is never an identity.
	origin := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "chrome-extension://") {
			origin = arg
			break
		}
	}
	if origin == "" {
		return native.ErrUnknownOrigin
	}
	path, err := localapi.DiscoveryPath()
	if err != nil {
		return errors.New("local core discovery is unavailable")
	}
	discovery, err := localapi.ReadDiscovery(path)
	if err != nil {
		return errors.New("local core discovery is unavailable")
	}
	return (native.Bridge{Origin: origin, AllowedOrigins: discovery.ExtensionOrigins, APIAddress: "http://" + discovery.Address, HostToken: discovery.HostToken}).Run(context.Background(), in, out)
}

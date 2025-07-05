package relay

import (
	"fmt"
)

var (
	version string
	build   string
)

func Version() string {
	return fmt.Sprintf("%s (%s)", version, build)
}

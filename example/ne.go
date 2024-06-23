package main

import (
	"context"

	"github.com/redpanda-data/benthos/v4/public/service"

	// Import all standard Benthos components

	// "github.com/redpanda-data/connect/v4/public/components/all"
	_ "github.com/redpanda-data/benthos/v4/public/bloblang"
	_ "github.com/redpanda-data/benthos/v4/public/components/io"
	_ "github.com/redpanda-data/connect/v4/public/components/mqtt"

	_ "github.com/RuneRoven/benthosAlarm"
)

func main() {
	service.RunCLI(context.Background())
}

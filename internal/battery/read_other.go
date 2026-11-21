//go:build !darwin && !windows

package battery

import "context"

func Read(context.Context) Snapshot { return Snapshot{} }

//go:build !darwin && !windows

package battery

import "context"

func Read(context.Context) Snapshot { return Snapshot{} }

func ReadIncremental(context.Context, func(Snapshot)) Snapshot { return Snapshot{} }

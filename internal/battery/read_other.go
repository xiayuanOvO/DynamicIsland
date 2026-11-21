//go:build !darwin && !windows

package battery

import "context"

func Read(context.Context) []Reading { return nil }

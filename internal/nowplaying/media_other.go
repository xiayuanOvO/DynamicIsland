//go:build !darwin && !windows

package nowplaying

import (
	"context"
	"errors"
)

func Adapter() string { return "" }

func Read(context.Context, string, bool) (Track, error) {
	return Track{}, errors.ErrUnsupported
}

func Control(context.Context, string, Command) error {
	return errors.ErrUnsupported
}

func Watch(context.Context, string, func(Track)) {}

func Close() {}
